package dev

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/viewer"
)

// remoteModule writes a module whose program stands in for an app shown
// by a viewer: it says which socket it was given and the message it was
// built with, waits to be stopped, and, built with the message "exit",
// writes to stderr and exits with status 3 at once.
func remoteModule(t *testing.T, msg string) string {
	t.Helper()
	dir := module(t, msg)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"app/msg"
)

func main() {
	fmt.Printf("%s socket=%q\n", msg.Text, os.Getenv("GGWD_REMOTE_SOCKET"))
	if msg.Text == "exit" {
		fmt.Fprintln(os.Stderr, "bye from the app")
		os.Exit(3)
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
}
`), 0o644)
	return dir
}

// fakeDisplay starts fakeViewers, or fails with err.
type fakeDisplay struct {
	mu      sync.Mutex
	err     error
	stale   bool
	viewers []*fakeViewer
}

func (d *fakeDisplay) Start(ctx context.Context) (Viewer, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	// The socket is a name the app is given, never a file: the same on every system.
	v := &fakeViewer{socket: "/fake/v" + string(rune('0'+len(d.viewers))) + ".sock", events: make(chan viewer.Event, 4)}
	d.viewers = append(d.viewers, v)
	return v, nil
}

func (d *fakeDisplay) Stale(ctx context.Context, paths []string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.stale
}

func (d *fakeDisplay) viewer(i int) *fakeViewer {
	d.mu.Lock()
	defer d.mu.Unlock()
	if i >= len(d.viewers) {
		return nil
	}
	return d.viewers[i]
}

// fakeViewer records what it is sent; the test sends its events.
type fakeViewer struct {
	socket string
	events chan viewer.Event

	mu      sync.Mutex
	sent    []viewer.Command
	stopped bool
}

func (v *fakeViewer) Socket() string              { return v.socket }
func (v *fakeViewer) Events() <-chan viewer.Event { return v.events }
func (v *fakeViewer) Exited() string              { return "signal: killed" }
func (v *fakeViewer) Stop(time.Duration)          { v.mu.Lock(); v.stopped = true; v.mu.Unlock() }
func (v *fakeViewer) isStopped() bool             { v.mu.Lock(); defer v.mu.Unlock(); return v.stopped }
func (v *fakeViewer) Send(c viewer.Command) error {
	v.mu.Lock()
	v.sent = append(v.sent, c)
	v.mu.Unlock()
	return nil
}
func (v *fakeViewer) commands() []viewer.Command {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]viewer.Command{}, v.sent...)
}
func (v *fakeViewer) lastOf(typ string) *viewer.Command {
	cs := v.commands()
	for i := len(cs) - 1; i >= 0; i-- {
		if cs[i].Type == typ {
			return &cs[i]
		}
	}
	return nil
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func runNative(t *testing.T, n *Native) (cancel func(), done <-chan error) {
	ctx, cancel := context.WithCancel(t.Context())
	ch := make(chan error, 1)
	go func() { ch <- n.Run(ctx) }()
	return cancel, ch
}

func wait(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return")
		return nil
	}
}

// With a viewer, the app is built with the remote tag and started with
// the viewer's socket; the viewer hears of builds, of a failed one with
// the compiler's output, and of an app that exits with the end of its
// stderr; closing the viewer's window ends the session and the viewer.
func TestRemoteLoop(t *testing.T) {
	dir := remoteModule(t, "v1")
	d := &fakeDisplay{}
	var out, log syncBuffer
	n := &Native{
		Dir: dir, Package: ".", Watch: true, Display: d,
		StopTimeout: 5 * time.Second,
		Log:         &log, Stdout: &out, Stderr: &out,
	}
	cancel, done := runNative(t, n)
	defer cancel()

	waitFor(t, &out, `v1 socket="/fake/v0.sock"`)
	if !strings.Contains(strings.Join(n.list.Tags, ","), RemoteTag) {
		t.Errorf("the app is built with tags %v, without %s", n.list.Tags, RemoteTag)
	}
	v := d.viewer(0)

	writeMsg(t, dir, "v2 broken\"")
	waitFor(t, &log, "; the app keeps running the last build that succeeded")
	waitUntil(t, "build-failed", func() bool { return v.lastOf(viewer.BuildFailed) != nil })
	if c := v.lastOf(viewer.BuildFailed); !strings.Contains(c.Output, "msg.go") {
		t.Errorf("build-failed carries %q, not the compiler's output", c.Output)
	}
	if v.lastOf(viewer.Building) == nil {
		t.Error("no building before the build")
	}

	writeMsg(t, dir, "exit")
	waitFor(t, &log, "waiting for a change")
	waitUntil(t, "app-exited", func() bool { return v.lastOf(viewer.AppExited) != nil })
	if c := v.lastOf(viewer.AppExited); c.Status != "exit status 3" || !strings.Contains(c.Output, "bye from the app") {
		t.Errorf("app-exited = %+v, want status 3 and the app's stderr", *c)
	}

	writeMsg(t, dir, "v3")
	waitFor(t, &out, `v3 socket="/fake/v0.sock"`)
	v.events <- viewer.Event{Type: viewer.WindowClosed}
	if err := wait(t, done); err != nil {
		t.Errorf("Run = %v after the window closed, want nil", err)
	}
	if !v.isStopped() {
		t.Error("the viewer was not stopped")
	}
}

// Without a viewer to be had the app opens its own window, and ggui says
// why; a viewer that goes away mid-session leaves the app running in its
// own window, restarted on the last build.
func TestRemoteFallback(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		dir := remoteModule(t, "v1")
		d := &fakeDisplay{err: &viewer.Unavailable{Reason: "the project's ggui (v0.1.0) has no viewer"}}
		var out, log syncBuffer
		n := &Native{Dir: dir, Package: ".", Watch: true, Display: d, StopTimeout: 5 * time.Second, Log: &log, Stdout: &out, Stderr: &out}
		cancel, done := runNative(t, n)
		waitFor(t, &out, `v1 socket=""`)
		waitFor(t, &log, "the app opens its own window: the project's ggui (v0.1.0) has no viewer")
		cancel()
		wait(t, done)
	})
	t.Run("lost", func(t *testing.T) {
		dir := remoteModule(t, "v1")
		d := &fakeDisplay{}
		var out, log syncBuffer
		n := &Native{Dir: dir, Package: ".", Watch: true, Display: d, StopTimeout: 5 * time.Second, Log: &log, Stdout: &out, Stderr: &out}
		cancel, done := runNative(t, n)
		waitFor(t, &out, `v1 socket="/fake/v0.sock"`)
		close(d.viewer(0).events) // it crashed
		waitFor(t, &log, "the app opens its own window for the rest of this session")
		waitFor(t, &out, `v1 socket=""`)
		writeMsg(t, dir, "v2")
		waitFor(t, &out, `v2 socket=""`)
		cancel()
		if err := wait(t, done); err != nil {
			t.Errorf("Run = %v", err)
		}
	})
}

// A change that makes another viewer restarts the viewer, and the app in
// the new one.
func TestRemoteViewerRestart(t *testing.T) {
	dir := remoteModule(t, "v1")
	d := &fakeDisplay{}
	var out, log syncBuffer
	n := &Native{Dir: dir, Package: ".", Watch: true, Display: d, StopTimeout: 5 * time.Second, Log: &log, Stdout: &out, Stderr: &out}
	cancel, done := runNative(t, n)
	defer cancel()
	waitFor(t, &out, `v1 socket="/fake/v0.sock"`)
	d.mu.Lock()
	d.stale = true
	d.mu.Unlock()
	writeMsg(t, dir, "v2")
	waitFor(t, &out, `v2 socket="/fake/v1.sock"`)
	if !d.viewer(0).isStopped() {
		t.Error("the old viewer still runs")
	}
	cancel()
	wait(t, done)
	if !d.viewer(1).isStopped() {
		t.Error("the viewer was not stopped at the end")
	}
}

func TestTail(t *testing.T) {
	var tl tail
	tl.Write([]byte("first line\n"))
	if got := tl.String(); got != "first line\n" {
		t.Errorf("tail = %q", got)
	}
	tl.Write([]byte(strings.Repeat("x", tailSize) + "\nlast\n"))
	if got := tl.String(); got != "last\n" {
		t.Errorf("tail after overflow = %q, want what follows the first cut line", got)
	}
	var nilTail *tail
	if nilTail.String() != "" {
		t.Error("a nil tail is not empty")
	}
}

// An app that exits cleanly as its window closes ends the session when the
// viewer says so in time; what else it says is about the app that went.
func TestWindowClosing(t *testing.T) {
	v := &fakeViewer{events: make(chan viewer.Event, 4)}
	n := &Native{viewer: v}
	v.events <- viewer.Event{Type: viewer.AppDetached, PID: 1}
	v.events <- viewer.Event{Type: viewer.WindowClosed}
	if !n.windowClosing() {
		t.Error("window-closed after the app's exit was missed")
	}
	v.events <- viewer.Event{Type: viewer.AppDetached, PID: 2}
	start := time.Now()
	if n.windowClosing() {
		t.Error("windowClosing without window-closed")
	}
	if d := time.Since(start); d < closeGrace {
		t.Errorf("gave up after %s, before closeGrace", d)
	}
	if (&Native{}).windowClosing() {
		t.Error("windowClosing with no viewer")
	}
}
