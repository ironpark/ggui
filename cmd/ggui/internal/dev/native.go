// Package dev is ggui dev's loop: watch the sources, rebuild on a change
// and replace the running app with the new build.
//
// Three pieces do the work and know nothing of each other: the watcher
// (package watch) says that something changed, the builder (package
// gotool, or package web for the browser) turns the sources into a
// program, and the runner (package proc) starts and stops it. A fourth,
// the Display, shows the app's window in a viewer process that outlives
// the app's restarts (package viewer). This package only sequences them,
// and Native.restart is the one place where a running instance and its
// replacement meet.
package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/proc"
	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
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
	// Display, when set, shows the app in a viewer's window rather than
	// in its own: remote display. When there is no viewer to be had, or
	// it goes away, the app opens its own window for the rest of the
	// session, and ggui says why.
	Display Display

	Log    io.Writer // ggui's own messages
	Stdout io.Writer // the app's
	Stderr io.Writer

	tmp  string
	seq  int
	app  *proc.Process
	exe  string // the build app is running, or the last that built
	w    *watch.Watcher
	list gotool.ListOptions

	viewer  Viewer
	tail    *tail     // the end of what the running app wrote to stderr
	changed time.Time // when the change being built was seen
	cause   string    // "the change", or "ggui dev started"
	built   time.Duration
	started time.Time
	shown   bool // the running app's first frame is up
}

func (n *Native) logf(format string, args ...any) {
	fmt.Fprintf(n.Log, "ggui: "+format+"\n", args...)
}

// startResult is a viewer that started, or why none did.
type startResult struct {
	v   Viewer
	err error
}

// Run builds and runs the app, and with Watch rebuilds and restarts it on
// every change until ctx is done. A failed build leaves the app that is
// running alone; an app that exits by itself is started again by the next
// change that builds. Without Watch it returns when the app exits. With a
// Display, it also returns when the viewer's window is closed.
func (n *Native) Run(ctx context.Context) error {
	tmp, err := os.MkdirTemp("", "ggui-dev-")
	if err != nil {
		return err
	}
	n.tmp = tmp
	defer os.RemoveAll(tmp)
	// The app stops first, then its viewer.
	defer n.dropViewer()
	defer n.stop()
	n.changed, n.cause = time.Now(), "ggui dev started"

	// The viewer starts, which may take a build of it, while the app
	// builds.
	var viewerReady chan startResult
	if n.Display != nil {
		n.Tags = append(slices.Clone(n.Tags), RemoteTag)
		viewerReady = make(chan startResult, 1)
		go func() {
			v, err := n.Display.Start(ctx)
			viewerReady <- startResult{v, err}
		}()
	}
	n.list = gotool.ListOptions{Dir: n.Dir, Tags: n.Tags}

	var changes chan []string
	if n.Watch {
		set, err := WatchSet(ctx, n.list, n.Package, n.Extra)
		if err != nil {
			if viewerReady != nil {
				n.useViewer(<-viewerReady)
			}
			return err
		}
		n.w = watch.New(set)
		changes = make(chan []string)
		go n.w.Run(ctx, changes)
		n.logf("watching %d files", n.w.Len())
	}

	exe, err := n.build(ctx)
	if viewerReady != nil {
		n.useViewer(<-viewerReady)
	}
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		n.buildFailed(err)
		if !n.Watch {
			return err
		}
	} else {
		n.restart(exe)
	}
	for {
		var exited <-chan struct{}
		if n.app != nil {
			exited = n.app.Done()
		}
		var events <-chan viewer.Event
		if n.viewer != nil {
			events = n.viewer.Events()
		}
		select {
		case <-ctx.Done():
			return nil
		case batch := <-changes:
			n.changed, n.cause = time.Now(), "the change"
			n.logf("%s changed, rebuilding", describe(n.Dir, batch))
			n.rebuild(ctx, batch)
			n.refresh(ctx)
		case <-exited:
			msg := n.app.Exited()
			err := n.app.Err()
			n.app.Stop(0) // releases the group
			n.app = nil
			if err == nil && n.windowClosing() {
				n.logf("the window was closed")
				return nil
			}
			if !n.Watch {
				if err != nil {
					return fmt.Errorf("the app %s", msg)
				}
				return nil
			}
			n.logf("the app %s; waiting for a change to start it again", msg)
			n.send(viewer.Command{Type: viewer.AppExited, Status: msg, Output: n.tail.String()})
		case ev, ok := <-events:
			if !ok {
				n.viewerLost()
				continue
			}
			switch ev.Type {
			case viewer.FirstFrame:
				if n.app != nil && ev.PID == n.app.Pid() && !n.shown {
					n.shown = true
					now := time.Now()
					n.logf("on screen %s after %s (build %s, start to first frame %s)",
						now.Sub(n.changed).Round(time.Millisecond), n.cause, n.built.Round(time.Millisecond),
						now.Sub(n.started).Round(time.Millisecond))
				}
			case viewer.WindowClosed:
				n.logf("the window was closed")
				return nil
			}
		}
	}
}

// closeGrace is how long an app that exited cleanly is given for its
// viewer to say that its window closed: an app exits when its window is
// closed, a moment before the viewer says so.
const closeGrace = 300 * time.Millisecond

// windowClosing reports whether the viewer says, within closeGrace, that
// its window closed. What else it says meanwhile is about the app that
// exited, and is dropped.
func (n *Native) windowClosing() bool {
	if n.viewer == nil {
		return false
	}
	timer := time.NewTimer(closeGrace)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-n.viewer.Events():
			if !ok {
				return false // gone: the loop hears of it next
			}
			if ev.Type == viewer.WindowClosed {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// useViewer takes the viewer that started, or says why the app opens its
// own window.
func (n *Native) useViewer(r startResult) {
	if r.err != nil {
		if !errors.Is(r.err, context.Canceled) {
			n.logf("the app opens its own window: %v", r.err)
		}
		return
	}
	n.viewer = r.v
}

// viewerLost is the viewer's end when nobody closed its window: the app
// opens its own window from then on, running the last build that
// succeeded, as the one that showed in the viewer exits with it.
func (n *Native) viewerLost() {
	n.logf("the viewer ended (%s); the app opens its own window for the rest of this session", n.viewer.Exited())
	n.dropViewer()
	if n.exe == "" {
		return
	}
	n.stop()
	n.start(n.exe)
}

// dropViewer stops the viewer, if there is one, and forgets it.
func (n *Native) dropViewer() {
	if n.viewer == nil {
		return
	}
	n.viewer.Stop(n.StopTimeout)
	n.viewer = nil
}

// send sends the viewer a command, if there is a viewer. One that went
// away is noticed by its events.
func (n *Native) send(c viewer.Command) {
	if n.viewer != nil {
		_ = n.viewer.Send(c)
	}
}

// rebuild builds the code as it is now and, if it built, restarts the app
// on it. The errors of a failed build are printed as the compiler wrote
// them, and the app keeps running the build before. A change that makes
// another viewer restarts the viewer too.
func (n *Native) rebuild(ctx context.Context, changed []string) {
	n.send(viewer.Command{Type: viewer.Building})
	exe, err := n.build(ctx)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		n.buildFailed(err)
		return
	}
	if n.viewer != nil && n.Display.Stale(ctx, changed) {
		n.logf("the viewer's dependencies changed; restarting it, and its window with it")
		n.stop()
		n.dropViewer()
		v, err := n.Display.Start(ctx)
		n.useViewer(startResult{v, err})
	}
	n.restart(exe)
}

// build builds the app into a new file and returns its path. A failure is
// printed as the compiler wrote it.
func (n *Native) build(ctx context.Context) (string, error) {
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
		return "", ctx.Err()
	}
	if err != nil {
		if n.app != nil {
			n.logf("build failed; the app keeps running the last build that succeeded:")
		} else {
			n.logf("build failed:")
		}
		// As the compiler wrote it, in the file:line:col form terminals
		// and editors make links of; indenting would break them.
		fmt.Fprintln(n.Log, err.Error())
		return "", err
	}
	n.built = time.Since(start)
	n.logf("built in %s", n.built.Round(time.Millisecond))
	return exe, nil
}

// buildFailed shows a failed build in the viewer's window too.
func (n *Native) buildFailed(err error) {
	n.send(viewer.Command{Type: viewer.BuildFailed, Output: err.Error()})
}

// restart replaces the running app, if any, with one running exe.
//
// This is where a development channel to the app would attach, when ggui
// dev learns to keep state across a restart: the old instance would be
// asked for its state here before it is stopped, and the new one handed
// it once started, both over a channel whose address the app finds in its
// environment. For now the old instance is stopped and the new one started
// fresh.
func (n *Native) restart(exe string) {
	if n.app != nil {
		n.stop()
	}
	if n.exe != "" && n.exe != exe {
		os.Remove(n.exe) // the previous build, no longer running
	}
	n.exe = exe
	n.start(exe)
}

// start starts the app on exe, in the viewer when there is one.
func (n *Native) start(exe string) {
	env := proc.Environ(n.Env)
	if n.viewer != nil {
		if env == nil {
			env = os.Environ()
		}
		env = append(env, remoteSocketEnv+"="+n.viewer.Socket())
	}
	n.tail = &tail{}
	p, err := proc.Start(exe, slices.Clone(n.Args), proc.Options{
		Dir:    n.Dir,
		Env:    env,
		Stdout: n.Stdout,
		Stderr: io.MultiWriter(n.Stderr, n.tail),
	})
	if err != nil {
		n.logf("could not start the app: %v", err)
		n.send(viewer.Command{Type: viewer.AppExited, Status: "did not start", Output: err.Error()})
		return
	}
	n.app = p
	n.started, n.shown = time.Now(), false
}

// remoteSocketEnv is ggwd/remote.EnvSocket, which this command, importing
// nothing of ggui's, spells out.
const remoteSocketEnv = "GGWD_REMOTE_SOCKET"

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

// tailSize is how much of the end of the app's stderr a viewer is shown
// when the app exits.
const tailSize = 8 << 10

// tail keeps the last tailSize bytes written to it.
type tail struct {
	mu  sync.Mutex
	b   []byte
	cut bool // the start was dropped
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.b = append(t.b, p...)
	if over := len(t.b) - tailSize; over > 0 {
		t.b = append(t.b[:0], t.b[over:]...)
		t.cut = true
	}
	return len(p), nil
}

// String returns what was kept, from the start of a line.
func (t *tail) String() string {
	if t == nil {
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	s := string(t.b)
	if t.cut {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
	}
	return s
}
