package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/dev"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

var devCmd = &command{
	name:    "dev",
	args:    "[package] [-- app arguments]",
	summary: "run an application, rebuilding and restarting it on every change",
	help: `Dev builds the application with the development tags (dev.tags in
ggui.toml, ggui_inspector and ggui_debug by default) and runs it. It
watches the Go files the app is built from in the modules being edited,
the files it embeds, ggui.toml and the paths dev.watch names; on a
change it rebuilds and, when the build succeeds, stops the app and
starts the new one. A change to comments or layout alone, a test file
and what git ignores rebuild nothing. A failed build, or a Go file that
does not parse, prints the errors and leaves the running app alone. An app that exits is started again by
the next change. Ctrl-C stops the app and ggui; a second Ctrl-C kills
the app at once.

State the app marks with StateValue.Keep, and a router's location with
Router.Keep, is asked of the app before it is stopped and handed to the
new build as it starts, so that a save does not lose the screen being
worked on; with --web the page carries it across its reload.

On macOS the app's window is shown by a viewer, a process of its own
that keeps the window open, where it was and as large as it was, while
the app is rebuilt and restarts: the window shows the build's progress
and a failed build's errors, and says when the app exits. The app is
built with the ggwd_remote tag for it and draws through the viewer. With
--display=window, or dev.display = "window", each run of the app opens
a window of its own, as a release build does. When there is no viewer
to be had, because the project's ggui is too old for one or it does not
build or start, the app opens its own window and ggui says why; if the
viewer goes away, the app is started again in its own window. Closing
the viewer's window ends ggui dev.

The package is a directory such as ./cmd/app, app.package in ggui.toml
by default. Arguments after -- go to the app, after dev.args.

With --web the app is built for the browser and served, and an open page
reloads itself after each build. A failed build is shown over the page,
which keeps running the last build that succeeded.`,
	flags: func(fs *flag.FlagSet) func(context.Context, []string, []string) error {
		cfgPath := fs.String("config", "", "read `file` instead of the ggui.toml found from the current directory")
		tags := fs.String("tags", "", "build `tags`, comma-separated, replacing dev.tags")
		noWatch := fs.Bool("no-watch", false, "build and run once; do not watch for changes")
		stopTimeout := fs.Duration("stop-timeout", 3*time.Second, "how long the app has to exit when asked before it is killed")
		display := fs.String("display", "", "where the app's window shows: `remote`, in a viewer that stays while the app restarts (the default on macOS, Linux and Windows), or window, its own; replaces dev.display")
		webMode := fs.Bool("web", false, "build for the browser and serve the app instead of running it")
		port := fs.Int("port", 8080, "with --web, the `port` to serve on")
		host := fs.String("host", "localhost", "with --web, the `address` to listen on; 0.0.0.0 reaches the server from other devices, such as a phone")
		wasmOpt := fs.String("wasm-opt", "", "with --web, wasm-opt `flags` such as -O2; empty skips the pass, which takes seconds")
		base := fs.String("base", "", "with --web, the `path` to serve the app below, replacing web.base")
		verbose := fs.Bool("verbose", false, "print each decision of the watcher and the rebuild loop, timed: what changed, what was passed over and why, builds started and cancelled; GGUI_VERBOSE=1 does the same")
		return func(ctx context.Context, pos, rest []string) error {
			if len(pos) > 1 {
				fmt.Fprintln(os.Stderr, "ggui dev: takes one package")
				return errUsage
			}
			arg := ""
			if len(pos) == 1 {
				arg = pos[0]
			}
			proj, err := project.Open(".", arg, *cfgPath)
			if err != nil {
				return err
			}
			cfg := proj.Config
			// settings are what p's file gives the session that it can
			// take up again while it runs, the flags replacing what
			// they replace.
			settings := func(p *project.Project) dev.Settings {
				s := dev.Settings{
					Tags: p.Config.Dev.Tags,
					Args: append(slices.Clone(p.Config.Dev.Args), rest...),
					Env:  p.Config.Dev.Env,
				}
				if isSet(fs, "tags") {
					s.Tags = splitTags(*tags)
				}
				if p.File != "" {
					s.Extra = append(s.Extra, p.File)
				}
				for _, w := range p.Config.Dev.Watch {
					s.Extra = append(s.Extra, p.Path(w))
				}
				return s
			}
			first := settings(proj)
			devTags, extra := first.Tags, first.Extra
			outDir := proj.Path(cfg.Build.Output)
			say := ui.Stderr.Prefixed("ggui")
			var tracef func(string, ...any)
			if *verbose || os.Getenv("GGUI_VERBOSE") != "" {
				tracef = func(format string, args ...any) {
					say.Info("%s", say.Dim(fmt.Sprintf("%s "+format, append([]any{time.Now().Format("15:04:05.000")}, args...)...)))
				}
			}
			lo := gotool.ListOptions{Tags: devTags}
			if *webMode {
				lo.GOOS, lo.GOARCH = "js", "wasm"
			}
			main, err := resolveMain(ctx, say, proj, arg, lo)
			if err != nil {
				return err
			}

			if *webMode {
				if len(rest) > 0 || len(cfg.Dev.Args) > 0 || len(cfg.Dev.Env) > 0 {
					say.Warn("a page in a browser takes no arguments or environment; dev.args, dev.env and those after -- are not used with --web")
				}
				b := cfg.Web.Base
				if isSet(fs, "base") {
					b = *base
				}
				title := cfg.Web.Title
				if title == "" {
					title = main.Name
				}
				w := &dev.Web{
					Dir: proj.Root, Package: main.Package, PkgDir: main.Dir,
					Target: target(main), Tags: devTags,
					LDFlags: "-s -w", WasmOpt: *wasmOpt,
					Title: title, Base: b,
					Addr:  net.JoinHostPort(*host, strconv.Itoa(*port)),
					Extra: extra, OutDir: outDir, Watch: !*noWatch, Log: os.Stderr,
					Config: proj.File,
					Tracef: tracef,
				}
				return w.Run(ctx)
			}

			// A second signal skips the graceful stop.
			force := make(chan struct{})
			sigs := make(chan os.Signal, 2)
			signal.Notify(sigs, stopSignals...)
			defer signal.Stop(sigs)
			go func() {
				<-sigs
				<-sigs
				close(force)
			}()

			n := &dev.Native{
				Dir: proj.Root, Package: main.Package, Tags: devTags,
				Args: first.Args, Env: first.Env,
				Extra: extra, OutDir: outDir, Watch: !*noWatch,
				StopTimeout: *stopTimeout, Force: force,
				Config: proj.File,
				// ggui.toml read again when it changes. What the session
				// took from it once and holds for good is named, so that
				// an edit that seems ignored is not a puzzle.
				Reload: func() (dev.Settings, []string, error) {
					p, err := project.Open(".", arg, *cfgPath)
					if err != nil {
						return dev.Settings{}, nil, err
					}
					var notes []string
					held := func(changed bool, key string) {
						if changed {
							notes = append(notes, key+" changed: start ggui dev again for it to take effect")
						}
					}
					held(p.Config.Dev.Display != cfg.Dev.Display && !isSet(fs, "display"), "dev.display")
					held(p.Path(p.Config.Build.Output) != outDir, "build.output")
					held(p.Config.App.Package != cfg.App.Package && arg == "", "app.package")
					return settings(p), notes, nil
				},
				// The app's stdout is the terminal itself; its stderr
				// goes through a pipe for the tail a viewer shows, and on
				// through ui, above any spinner of ggui's.
				Log: os.Stderr, Stdout: os.Stdout, Stderr: ui.Stderr,
				Tracef: tracef,
			}
			mode := cfg.Dev.Display
			if isSet(fs, "display") {
				mode = *display
			}
			d, err := remoteDisplay(mode, proj.Root)
			if err != nil {
				return err
			}
			if d != nil {
				n.Display = d
			}
			return n.Run(ctx)
		}
	},
}

// remoteDisplay returns the Display for mode, dev.display's value, or nil
// for the app's own window. Remote display where it is not available is
// the app's own window too, which is said when it was asked for by name.
func remoteDisplay(mode, dir string) (*dev.RemoteDisplay, error) {
	switch mode {
	case "window":
		return nil, nil
	case "", "remote":
	default:
		fmt.Fprintf(os.Stderr, "ggui dev: --display is remote or window, not %q\n", mode)
		return nil, errUsage
	}
	if err := viewer.Supported(); err != nil {
		if mode == "remote" {
			ui.Stderr.Prefixed("ggui").Warn("the app opens its own window: %v", err)
		}
		return nil, nil
	}
	cache, err := viewer.CacheDir()
	if err != nil {
		ui.Stderr.Prefixed("ggui").Warn("the app opens its own window: no cache directory for the viewer: %v", err)
		return nil, nil
	}
	// The viewer's logs go through ui, so that they land above the
	// build's spinner rather than in it.
	return &dev.RemoteDisplay{Dir: dir, CacheDir: cache, Exe: os.Getenv("GGUI_VIEWER"), Stderr: ui.Stderr, Log: os.Stderr}, nil
}

// resolveMain finds the main package, which takes the go command a moment,
// or longer when it has modules to download first: a spinner says so.
func resolveMain(ctx context.Context, log *ui.Output, proj *project.Project, arg string, lo gotool.ListOptions) (*project.Main, error) {
	s := log.Step("loading the package", ui.QuietStart())
	defer s.Stop()
	return proj.ResolveMain(ctx, arg, lo)
}

// target names the package for messages: its directory relative to the
// working directory when it is below it, its import path otherwise.
func target(m *project.Main) string {
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, m.Dir); err == nil && rel == "." {
			return "."
		} else if err == nil && filepath.IsLocal(rel) {
			return "./" + filepath.ToSlash(rel)
		}
	}
	return m.ImportPath
}
