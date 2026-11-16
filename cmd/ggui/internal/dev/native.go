// Package dev is ggui dev's loop: watch the sources, rebuild on a change
// and replace the running app with the new build.
//
// Three pieces do the work and know nothing of each other: the watcher
// (package watch) says that something changed, the builder (package
// gotool, or package web for the browser) turns the sources into a
// program, and the runner (package proc) starts and stops it. This
// package only sequences them, and Native.restart is the one place where
// a running instance and its replacement meet.
package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/proc"
	"github.com/ironpark/ggui/cmd/ggui/internal/watch"
)

// Native runs a desktop app under development.
type Native struct {
	Dir     string // where the go command runs
	Package string
	Tags    []string
	Args    []string
	Env     map[string]string
	// Extra are paths to watch besides the Go sources: ggui.toml and
	// dev.watch, absolute.
	Extra []string
	// Watch rebuilds on change; without it the loop builds and runs once.
	Watch bool
	// StopTimeout is how long the app has to exit when asked before it is
	// killed.
	StopTimeout time.Duration
	// Force, when closed, kills the app at once instead of waiting out
	// StopTimeout: a second Ctrl-C.
	Force <-chan struct{}

	Log    io.Writer // ggui's own messages
	Stdout io.Writer // the app's
	Stderr io.Writer

	tmp  string
	seq  int
	app  *proc.Process
	exe  string // the build app is running
	w    *watch.Watcher
	list gotool.ListOptions
}

func (n *Native) logf(format string, args ...any) {
	fmt.Fprintf(n.Log, "ggui: "+format+"\n", args...)
}

// Run builds and runs the app, and with Watch rebuilds and restarts it on
// every change until ctx is done. A failed build leaves the app that is
// running alone; an app that exits by itself is started again by the next
// change that builds. Without Watch it returns when the app exits.
func (n *Native) Run(ctx context.Context) error {
	tmp, err := os.MkdirTemp("", "ggui-dev-")
	if err != nil {
		return err
	}
	n.tmp = tmp
	defer os.RemoveAll(tmp)
	defer n.stop()
	n.list = gotool.ListOptions{Dir: n.Dir, Tags: n.Tags}

	var changes chan []string
	if n.Watch {
		set, err := WatchSet(ctx, n.list, n.Package, n.Extra)
		if err != nil {
			return err
		}
		n.w = watch.New(set)
		changes = make(chan []string)
		go n.w.Run(ctx, changes)
		n.logf("watching %d files", n.w.Len())
	}

	if err := n.rebuild(ctx); err != nil && !n.Watch {
		return err
	}
	for {
		var exited <-chan struct{}
		if n.app != nil {
			exited = n.app.Done()
		}
		select {
		case <-ctx.Done():
			return nil
		case batch := <-changes:
			n.logf("%s changed, rebuilding", describe(n.Dir, batch))
			n.rebuild(ctx)
			n.refresh(ctx)
		case <-exited:
			msg := n.app.Exited()
			err := n.app.Err()
			n.app.Stop(0) // releases the group
			n.app = nil
			if !n.Watch {
				if err != nil {
					return fmt.Errorf("the app %s", msg)
				}
				return nil
			}
			n.logf("the app %s; waiting for a change to start it again", msg)
		}
	}
}

// rebuild builds the code as it is now and, if it built, restarts the app
// on it. The errors of a failed build are printed as the compiler wrote
// them, and the app keeps running the build before.
func (n *Native) rebuild(ctx context.Context) error {
	n.seq++
	exe := filepath.Join(n.tmp, fmt.Sprintf("app-%d", n.seq))
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	start := time.Now()
	err := gotool.Build(ctx, gotool.BuildOptions{
		Dir: n.Dir, Package: n.Package, Output: exe, Tags: n.Tags,
	})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		var be *gotool.BuildError
		if n.app != nil {
			n.logf("build failed; the app keeps running the last build that succeeded:")
		} else {
			n.logf("build failed:")
		}
		if errors.As(err, &be) {
			// As the compiler wrote it, in the file:line:col form
			// terminals and editors make links of; indenting would
			// break them.
			fmt.Fprintln(n.Log, be.Error())
		} else {
			fmt.Fprintln(n.Log, err)
		}
		return err
	}
	n.logf("built in %s", time.Since(start).Round(time.Millisecond))
	return n.restart(exe)
}

// restart replaces the running app, if any, with one running exe.
//
// This is where a development channel to the app would attach, when ggui
// dev learns to keep state across a restart: the old instance would be
// asked for its state here before it is stopped, and the new one handed
// it once started, both over a channel whose address the app finds in its
// environment. For now the old instance is stopped and the new one started
// fresh.
func (n *Native) restart(exe string) error {
	if n.app != nil {
		n.stop()
	}
	if n.exe != "" {
		os.Remove(n.exe) // the previous build, no longer running
	}
	args := append([]string{}, n.Args...)
	p, err := proc.Start(exe, args, proc.Options{
		Dir:    n.Dir,
		Env:    proc.Environ(n.Env),
		Stdout: n.Stdout,
		Stderr: n.Stderr,
	})
	if err != nil {
		n.logf("could not start the app: %v", err)
		return err
	}
	n.app, n.exe = p, exe
	return nil
}

// stop stops the running app, asking first, killing it if it does not exit
// within StopTimeout or when Force closes.
func (n *Native) stop() {
	if n.app == nil {
		return
	}
	app := n.app
	n.app = nil
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-n.Force:
			app.Kill()
		case <-stopped:
		}
	}()
	if err := app.Stop(n.StopTimeout); errors.Is(err, proc.ErrKilled) {
		n.logf("the app %s after %s", err, n.StopTimeout)
	}
}

// refresh lists the sources again, since an edit may have added or
// dropped an import.
func (n *Native) refresh(ctx context.Context) {
	if n.w == nil {
		return
	}
	set, err := WatchSet(ctx, n.list, n.Package, n.Extra)
	if err != nil {
		if ctx.Err() == nil {
			n.logf("could not list the sources to watch: %v", err)
		}
		return
	}
	n.w.Set(set)
}

// describe names the changed files briefly, relative to dir.
func describe(dir string, paths []string) string {
	names := make([]string, 0, 3)
	for _, p := range paths {
		if len(names) == 3 {
			break
		}
		if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		names = append(names, p)
	}
	s := strings.Join(names, ", ")
	if more := len(paths) - len(names); more > 0 {
		s += fmt.Sprintf(" and %d more", more)
	}
	return s
}
