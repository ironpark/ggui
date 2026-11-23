package viewer

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// With GGUI_FAKE_VIEWER set, the test binary is a viewer that says it is
// ready on the socket it was given and answers each command with an event
// named after it, until its stdin closes; "crash" makes it exit with 2.
func TestMain(m *testing.M) {
	if os.Getenv("GGUI_FAKE_VIEWER") != "" {
		fakeViewer()
		return
	}
	os.Exit(m.Run())
}

func fakeViewer() {
	fs := flag.NewFlagSet("ggviewer", flag.ExitOnError)
	socket := fs.String("socket", "", "")
	fs.String("project", "", "")
	fs.Parse(os.Args[1:])
	enc := json.NewEncoder(os.Stdout)
	enc.Encode(Event{Type: Ready, Socket: *socket})
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		var c Command
		json.Unmarshal(sc.Bytes(), &c)
		if c.Type == "crash" {
			os.Exit(2)
		}
		enc.Encode(Event{Type: "got-" + c.Type, PID: len(c.Output)})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "fake viewer:", err)
		os.Exit(1)
	}
}

func startFake(t *testing.T) *Process {
	t.Helper()
	t.Setenv("GGUI_FAKE_VIEWER", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	v, err := Start(t.Context(), exe, Options{Project: t.TempDir(), Stderr: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func next(t *testing.T, v *Process) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-v.Events():
		return ev, ok
	case <-time.After(10 * time.Second):
		t.Fatal("no event")
		return Event{}, false
	}
}

// A viewer starts on a socket in a directory of its own, short enough for
// a unix socket, hears commands, and once stopped is gone with that
// directory.
func TestProcess(t *testing.T) {
	v := startFake(t)
	if len(v.Socket()) >= 104 {
		t.Errorf("socket path %q is too long for a unix socket", v.Socket())
	}
	dir := filepath.Dir(v.Socket())
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("the socket's directory %s: %v", dir, err)
	}
	if err := v.Send(Command{Type: BuildFailed, Output: "abc"}); err != nil {
		t.Fatal(err)
	}
	if ev, _ := next(t, v); ev.Type != "got-"+BuildFailed || ev.PID != 3 {
		t.Errorf("event = %+v, want the command echoed", ev)
	}
	v.Stop(5 * time.Second)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the socket's directory is left after Stop: %v", err)
	}
	if err := v.Send(Command{Type: Clear}); err == nil {
		t.Error("Send to a stopped viewer succeeded")
	}
}

// A viewer that dies closes its events, and says how it ended.
func TestProcessCrash(t *testing.T) {
	v := startFake(t)
	defer v.Stop(time.Second)
	v.Send(Command{Type: "crash"})
	if ev, ok := next(t, v); ok {
		t.Fatalf("event %+v from a viewer that exited", ev)
	}
	if got := v.Exited(); got != "exit status 2" {
		t.Errorf("Exited = %q", got)
	}
}

// writeModule writes a module at path with the files given, relative.
func writeModule(t *testing.T, path string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	files["go.mod"] = "module " + path + "\n\ngo 1.24\n"
	for name, src := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func offline(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs the go command")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")
}

// The viewer is found in the module graph, keyed by its sources, built
// into the cache once and found there after; an edit to its sources
// makes another key, and a graph without it is Unavailable.
func TestResolve(t *testing.T) {
	offline(t)
	main := "package main\n\nfunc main() { println(%q) }\n"
	dir := writeModule(t, "github.com/ironpark/ggui", map[string]string{
		"cmd/ggviewer/main.go": fmt.Sprintf(main, "one"),
	})
	cache := t.TempDir()
	b, err := Resolve(t.Context(), dir, cache)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(b.Path, cache) {
		t.Errorf("binary %s is not in the cache %s", b.Path, cache)
	}
	built, err := b.Ensure(t.Context())
	if err != nil || !built {
		t.Fatalf("Ensure = %v, %v; want a build", built, err)
	}
	again, err := Resolve(t.Context(), dir, cache)
	if err != nil {
		t.Fatal(err)
	}
	if again.Key != b.Key {
		t.Errorf("the same sources give keys %s and %s", b.Key, again.Key)
	}
	if built, err := again.Ensure(t.Context()); err != nil || built {
		t.Errorf("Ensure of a cached viewer = %v, %v; want it found", built, err)
	}

	src := filepath.Join(dir, "cmd", "ggviewer", "main.go")
	if !b.Affects([]string{src}) || !b.Affects([]string{filepath.Join(dir, "go.mod")}) || b.Affects([]string{filepath.Join(dir, "README")}) {
		t.Error("Affects does not follow the viewer's files and the module files")
	}
	os.WriteFile(src, []byte(fmt.Sprintf(main, "two")), 0o644)
	changed, err := Resolve(t.Context(), dir, cache)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Key == b.Key {
		t.Error("an edit to the viewer's sources keeps its key")
	}

	old := writeModule(t, "github.com/ironpark/ggui", map[string]string{"ggui.go": "package ggui\n"})
	_, err = Resolve(t.Context(), old, cache)
	var u *Unavailable
	if !errors.As(err, &u) || !strings.Contains(u.Reason, "has no "+Package) {
		t.Errorf("Resolve without a viewer = %v, want Unavailable naming it", err)
	}
}

func TestResolveWithTags(t *testing.T) {
	offline(t)
	dir := writeModule(t, "github.com/ironpark/ggui", map[string]string{
		"cmd/ggviewer/main.go":     "package main\n\nfunc main() { println(tagMode) }\n",
		"cmd/ggviewer/mode_go.go":  "//go:build !ggfxzig\n\npackage main\n\nconst tagMode = \"go\"\n",
		"cmd/ggviewer/mode_zig.go": "//go:build ggfxzig\n\npackage main\n\nconst tagMode = \"zig\"\n",
	})
	cache := t.TempDir()
	bGo, err := Resolve(t.Context(), dir, cache)
	if err != nil {
		t.Fatal(err)
	}
	bZig, err := Resolve(t.Context(), dir, cache, "ggfxzig")
	if err != nil {
		t.Fatal(err)
	}
	if bGo.Key == bZig.Key {
		t.Fatalf("Resolve with ggfxzig tag produced same key %s as default", bGo.Key)
	}
	if built, err := bZig.Ensure(t.Context()); err != nil || !built {
		t.Fatalf("Ensure(ggfxzig) = %v, %v", built, err)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	for i := range 6 {
		d := filepath.Join(dir, fmt.Sprint(i))
		os.Mkdir(d, 0o755)
		at := time.Now().Add(time.Duration(i) * time.Minute)
		os.Chtimes(d, at, at)
	}
	prune(dir, 4)
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if got := strings.Join(names, ","); got != "2,3,4,5" {
		t.Errorf("after pruning: %s, want the four used last", got)
	}
}
