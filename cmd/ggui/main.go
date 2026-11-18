// Command ggui creates, runs, checks and bundles ggui applications.
//
//	ggui init [dir]        create a new application
//	ggui dev [package]     run it, rebuilding and restarting it on every change
//	ggui build [package]   build a macOS .app, a Windows .exe, a web bundle or a program
//	ggui doctor            check the machine and the project
//	ggui i18n check|extract  check the translation catalogs, or fill them in
//
// It drives the go command on PATH rather than linking a toolchain in, so
// a build is the one go build would make, with the developer's own
// toolchain, workspace and module cache. Settings come from ggui.toml in
// the project, and flags override them. See docs/cli.md.
//
// Install it with
//
//	go install github.com/ironpark/ggui/cmd/ggui@latest
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// command is one subcommand.
type command struct {
	name    string
	args    string // the positional arguments, for the usage line
	summary string
	// help is printed above the flags by ggui help name.
	help  string
	flags func(fs *flag.FlagSet) func(ctx context.Context, pos, rest []string) error
}

var commands = []*command{initCmd, devCmd, buildCmd, doctorCmd, i18nCmd}

// stopSignals are the signals that stop a command: Ctrl-C, a polite kill,
// and the terminal closing.
var stopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

const docsURL = "https://github.com/ironpark/ggui/blob/main/docs/cli.md"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// errUsage reports a mistake in the command line, already explained; it
// exits with status 2, as the flag package's errors do.
var errUsage = errors.New("usage")

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		usage(stderr)
		return 2
	}
	name := args[0]
	switch name {
	case "-h", "-help", "--help", "help":
		if len(args) > 1 {
			if c := find(args[1]); c != nil {
				c.usage(stdout, nil)
				return 0
			}
			fmt.Fprintf(stderr, "ggui help: unknown command %q\n", args[1])
			return 2
		}
		usage(stdout)
		return 0
	case "version", "-version", "--version":
		fmt.Fprintln(stdout, "ggui", version())
		return 0
	}
	c := find(name)
	if c == nil {
		fmt.Fprintf(stderr, "ggui: unknown command %q\n\n", name)
		usage(stderr)
		return 2
	}

	fs := flag.NewFlagSet("ggui "+c.name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // errors are printed below, once
	runFn := c.flags(fs)
	pos, rest, err := parseInterspersed(fs, args[1:])
	if errors.Is(err, flag.ErrHelp) {
		c.usage(stdout, fs)
		return 0
	}
	if err != nil {
		fmt.Fprintf(stderr, "ggui %s: %v\n", c.name, err)
		fmt.Fprintf(stderr, "run 'ggui help %s' for usage\n", c.name)
		return 2
	}

	// The first of these ends the command cleanly: a build is interrupted,
	// a running app asked to exit. ggui dev also counts a second.
	ctx, stop := signal.NotifyContext(context.Background(), stopSignals...)
	defer stop()
	// A step a command left running, as one that returned early may,
	// leaves no spinner behind on the terminal.
	defer ui.Reset()
	if err := runFn(ctx, pos, rest); err != nil {
		if errors.Is(err, errUsage) {
			fmt.Fprintf(stderr, "run 'ggui help %s' for usage\n", c.name)
			return 2
		}
		if errors.Is(err, errFailed) {
			return 1 // the report said why
		}
		if ctx.Err() == nil || !errors.Is(err, context.Canceled) {
			e := ui.For(stderr)
			ui.Reset()
			fmt.Fprintf(e, "%s %v\n", e.Red("ggui "+c.name+":"), err)
		}
		return 1
	}
	return 0
}

func find(name string) *command {
	for _, c := range commands {
		if c.name == name {
			return c
		}
	}
	return nil
}

func usage(w io.Writer) {
	o := ui.For(w)
	fmt.Fprintf(w, `ggui creates, runs, checks and bundles ggui applications.

%s

	ggui <command> [arguments]

%s

`, o.Bold("Usage:"), o.Bold("Commands:"))
	for _, c := range commands {
		fmt.Fprintf(w, "\t%s %s\n", o.Cyan(fmt.Sprintf("%-8s", c.name)), c.summary)
	}
	fmt.Fprintf(w, `
Settings come from ggui.toml in the project (ggui init writes one), and
flags override them. Run 'ggui help <command>' for a command's flags.

Documentation: %s
`, docsURL)
}

// usage prints a command's help and flags.
func (c *command) usage(w io.Writer, fs *flag.FlagSet) {
	if fs == nil {
		fs = flag.NewFlagSet("ggui "+c.name, flag.ContinueOnError)
		c.flags(fs)
	}
	o := ui.For(w)
	fmt.Fprintf(w, "%s ggui %s [flags] %s\n\n%s\n", o.Bold("Usage:"), c.name, c.args, c.help)
	first := true
	fs.VisitAll(func(f *flag.Flag) {
		if first {
			fmt.Fprintf(w, "\n%s\n\n", o.Bold("Flags:"))
			first = false
		}
		name, help := flag.UnquoteUsage(f)
		line := "  --" + f.Name
		if name != "" {
			line += " " + name
		}
		fmt.Fprintln(w, o.Cyan(line))
		fmt.Fprintf(w, "\t%s", help)
		if f.DefValue != "" && f.DefValue != "false" {
			fmt.Fprintf(w, " (default %q)", f.DefValue)
		}
		fmt.Fprintln(w)
	})
	fmt.Fprintf(w, "\nDocumentation: %s\n", docsURL)
}

// releaseVersion is the version a release build stamps in with -X, as the
// one built for Homebrew is: a build from a checkout records no module
// version of its own.
var releaseVersion string

// version reports the version this program was released as, or the module
// version it was built from, which go install records.
func version() string {
	if releaseVersion != "" {
		return releaseVersion
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "(unknown version)"
}
