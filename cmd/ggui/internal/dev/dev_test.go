package dev

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// module writes a module with a main package that imports a package of
// its own, standing in for an app.
func module(t *testing.T, msg string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go command")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module app\n\ngo 1.24\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "msg"), 0o755)
	writeMsg(t, dir, msg)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"app/msg"
)

func main() {
	fmt.Println(msg.Text, os.Getenv("DEV_TEST"), len(os.Args)-1)
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
	fmt.Println("stopped", msg.Text)
}
`), 0o644)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	return dir
}

func writeMsg(t *testing.T, dir, msg string) {
	t.Helper()
	p := filepath.Join(dir, "msg", "msg.go")
	if err := os.WriteFile(p, []byte("package msg\n\nconst Text = "+`"`+msg+`"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Coarse file system clocks must still see a change.
	future := time.Now().Add(time.Duration(len(msg)) * time.Second)
	os.Chtimes(p, future, future)
}

func TestWatchSet(t *testing.T) {
	dir := module(t, "v1")
	extra := filepath.Join(dir, "ggui.toml")
	set, err := WatchSet(t.Context(), gotool.ListOptions{Dir: dir}, ".", []string{extra}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		filepath.Join(dir, "main.go"),
		filepath.Join(dir, "msg", "msg.go"),
		filepath.Join(dir, "msg"), // a file added there is seen
		filepath.Join(dir, "go.mod"),
		extra,
	} {
		if !slices.Contains(set, want) {
			t.Errorf("watch set lacks %s:\n%s", want, strings.Join(set, "\n"))
		}
	}
	for _, p := range set {
		if !strings.HasPrefix(p, dir) {
			t.Errorf("watch set holds %s, outside the module (the standard library?)", p)
		}
	}
}

// syncBuffer is a bytes.Buffer safe to write from the app's output copier
// while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func waitFor(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %q in:\n%s", want, buf.String())
}

// The whole loop: build and run, a change rebuilds and replaces the app,
// a broken change leaves it running, and cancelling stops it.
func TestNativeLoop(t *testing.T) {
	dir := module(t, "v1")
	var out, log syncBuffer
	n := &Native{
		Dir: dir, Package: ".", Watch: true,
		Args: []string{"a", "b"}, Env: map[string]string{"DEV_TEST": "env"},
		StopTimeout: 5 * time.Second,
		Log:         &log, Stdout: &out, Stderr: &out,
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- n.Run(ctx) }()

	// On Windows the request to stop is Ctrl-Break, which a test without
	// a console cannot deliver, and the app is killed instead.
	graceful := runtime.GOOS != "windows"

	waitFor(t, &out, "v1 env 2")
	writeMsg(t, dir, "v2")
	if graceful {
		waitFor(t, &out, "stopped v1") // asked to stop, and did
	}
	waitFor(t, &out, "v2 env 2")

	// A syntax error is found before the go command runs, when ggui's
	// parser is as new as the toolchain, as it is in a test.
	writeMsg(t, dir, "v3 broken\"")
	waitFor(t, &log, "syntax error; the app keeps running the last build that succeeded")
	if strings.Contains(out.String(), "stopped v2") {
		t.Error("a failed build stopped the running app")
	}

	// A comment changes nothing the compiler acts on.
	writeMsg(t, dir, "v2")
	for deadline := time.Now().Add(60 * time.Second); strings.Count(out.String(), "v2 env 2") < 2; {
		if time.Now().After(deadline) {
			t.Fatalf("not started again after the fix:\n%s", out.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	p := filepath.Join(dir, "msg", "msg.go")
	src, _ := os.ReadFile(p)
	os.WriteFile(p, append(src, "\n// A comment.\n"...), 0o644)
	waitFor(t, &log, filepath.Join("msg", "msg.go")+": comments only; not restarted")
	time.Sleep(300 * time.Millisecond)
	if n := strings.Count(out.String(), "v2 env 2"); n != 2 {
		t.Errorf("started %d times on v2, want 2: a comment restarted the app", n)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
	if graceful && !strings.Contains(out.String(), "stopped v2") {
		t.Errorf("the app was not stopped gracefully on exit:\n%s", out.String())
	}
}

func TestDescribe(t *testing.T) {
	paths := []string{"/p/a.go", "/p/b/c.go", "/q/d.go", "/p/e.go", "/p/f.go"}
	for i, p := range paths {
		paths[i] = filepath.FromSlash(p)
	}
	got := describe(filepath.FromSlash("/p"), paths)
	if want := filepath.FromSlash("a.go, b/c.go, /q/d.go and 2 more"); got != want {
		t.Errorf("describe = %q, want %q", got, want)
	}
}
