// Package gotool runs the go command: builds, package and module listings,
// and the environment. The ggui command never links the toolchain in; it
// drives the go on PATH, so what it builds is exactly what go build would,
// with the toolchain, workspace and module cache the developer already has.
package gotool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/ui"
)

// Go is the program run as the go command.
const Go = "go"

// command returns a go command that stops by interrupt when ctx is done,
// so a build cancelled by Ctrl-C lets the go command stop its compilers
// rather than leaving them to finish on their own.
func command(ctx context.Context, dir string, env []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, Go, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		// Environ, not os.Environ: it sets PWD to dir, as a command
		// without an Env of its own gets. The go command takes its
		// working directory from PWD when PWD names it, and otherwise
		// from the system, which resolves symbolic links; a package
		// named by a path through one, such as macOS's /var/folders
		// for /private/var/folders, would then lie outside the module.
		cmd.Env = append(cmd.Environ(), env...)
	}
	cmd.Cancel = func() error { return interrupt(cmd.Process) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// output runs a go command and returns its standard output, or an error
// carrying what it wrote to standard error.
func output(ctx context.Context, dir string, env []string, args ...string) ([]byte, error) {
	cmd := command(ctx, dir, env, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && stderr.Len() > 0 {
			return out, fmt.Errorf("go %s: %s", args[0], strings.TrimSpace(stderr.String()))
		}
		if errors.Is(err, exec.ErrNotFound) {
			return out, ErrNoGo
		}
		return out, fmt.Errorf("go %s: %w", args[0], err)
	}
	return out, nil
}

// Run runs a go command in dir with env added, writing what it prints,
// both streams, to out. When ctx is done the go command is interrupted,
// as Ctrl-C would, and Run returns once it has stopped, or WaitDelay
// after. A go command that is not on PATH is said so.
func Run(ctx context.Context, dir string, env []string, out io.Writer, args ...string) error {
	cmd := command(ctx, dir, env, args...)
	cmd.Stdout, cmd.Stderr = out, out
	err := cmd.Run()
	if errors.Is(err, exec.ErrNotFound) {
		return ErrNoGo
	}
	return err
}

// ErrNoGo reports that there is no go command to run.
var ErrNoGo = errors.New("the go command is not on PATH: install Go from https://go.dev/dl/")

// Env returns the go command's view of the named variables, which takes
// GOENV, the environment and the toolchain's defaults into account.
func Env(ctx context.Context, dir string, env []string, names ...string) (map[string]string, error) {
	out, err := output(ctx, dir, env, append([]string{"env", "-json"}, names...)...)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	return m, nil
}

// BuildOptions says what to build and how.
type BuildOptions struct {
	Dir     string   // the directory the go command runs in
	Package string   // the main package, a directory or an import path
	Output  string   // the file to write
	Tags    []string // build tags
	LDFlags string   // linker flags
	// Trimpath drops the builder's file system paths from the binary, as
	// a release should.
	Trimpath bool
	GOOS     string // empty for the go command's default
	GOARCH   string
	Env      []string // more environment, KEY=value
	// TmpDir, when set, is where the go command makes its work
	// directory, GOTMPDIR. A build that is cancelled is killed, and
	// leaves its work directory behind for the caller to remove.
	TmpDir string
}

// Args returns the go command line for o, without the program name.
func (o BuildOptions) Args() []string {
	args := []string{"build", "-o", o.Output}
	if len(o.Tags) > 0 {
		args = append(args, "-tags", strings.Join(o.Tags, ","))
	}
	if o.Trimpath {
		args = append(args, "-trimpath")
	}
	if o.LDFlags != "" {
		args = append(args, "-ldflags", o.LDFlags)
	}
	return append(args, o.Package)
}

func (o BuildOptions) env() []string {
	env := append([]string{}, o.Env...)
	if o.GOOS != "" {
		env = append(env, "GOOS="+o.GOOS)
	}
	if o.GOARCH != "" {
		env = append(env, "GOARCH="+o.GOARCH)
	}
	if o.TmpDir != "" {
		env = append(env, "GOTMPDIR="+o.TmpDir)
	}
	return env
}

// String is the build as a person would type it, for logs, without the
// work directory, which only ggui cares where it is.
func (o BuildOptions) String() string {
	var b strings.Builder
	for _, e := range o.env() {
		if !strings.HasPrefix(e, "GOTMPDIR=") {
			b.WriteString(e + " ")
		}
	}
	b.WriteString("go")
	for _, a := range o.Args() {
		if a == "" || strings.ContainsAny(a, " \t\"'") {
			a = fmt.Sprintf("%q", a)
		}
		b.WriteString(" " + a)
	}
	return b.String()
}

// BuildError is a build that failed, with what the compiler said.
type BuildError struct {
	Output []byte
	Err    error
}

func (e *BuildError) Error() string {
	if len(e.Output) > 0 {
		return strings.TrimRight(string(e.Output), "\n")
	}
	return e.Err.Error()
}

func (e *BuildError) Unwrap() error { return e.Err }

// Build runs go build. A failure is a *BuildError holding the compiler's
// output, which the caller shows as it is: the go command already prints
// file:line:col positions an editor can follow. When ctx is done the build
// is killed, with the compilers it started, and Build returns once they
// are gone.
func Build(ctx context.Context, o BuildOptions) error {
	cmd := command(ctx, o.Dir, o.env(), o.Args()...)
	killTree(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return ErrNoGo
		}
		return &BuildError{Output: out, Err: err}
	}
	if len(out) > 0 {
		// A successful build can still warn, as a linker does: above a
		// step that is live on the terminal, not into its line.
		ui.Stderr.Write(out)
	}
	return nil
}
