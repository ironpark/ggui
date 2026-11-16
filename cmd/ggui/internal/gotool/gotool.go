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
	"os"
	"os/exec"
	"strings"
	"time"
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
		cmd.Env = append(os.Environ(), env...)
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
			return out, errors.New("the go command is not on PATH: install Go from https://go.dev/dl/")
		}
		return out, fmt.Errorf("go %s: %w", args[0], err)
	}
	return out, nil
}

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
	return env
}

// String is the build as a person would type it, for logs.
func (o BuildOptions) String() string {
	var b strings.Builder
	for _, e := range o.env() {
		b.WriteString(e + " ")
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
// file:line:col positions an editor can follow.
func Build(ctx context.Context, o BuildOptions) error {
	cmd := command(ctx, o.Dir, o.env(), o.Args()...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return errors.New("the go command is not on PATH: install Go from https://go.dev/dl/")
		}
		return &BuildError{Output: out, Err: err}
	}
	if len(out) > 0 {
		// A successful build can still warn, as a linker does.
		os.Stderr.Write(out)
	}
	return nil
}
