package dev

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// fakeBundler simulates a macOS .dev.app bundler for testing Native and
// RemoteDisplay behavior on all platforms.
type fakeBundler struct {
	dir string
	exe string

	mu          sync.Mutex
	stageCalls  []string
	signCalls   []string
	refresh     bool
	stagedPaths []string
}

func (b *fakeBundler) Dir() string { return b.dir }

func (b *fakeBundler) Stage(_ context.Context, src string) (string, []string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stageCalls = append(b.stageCalls, src)
	appDir := filepath.Join(b.dir, "App.dev.app", "Contents", "MacOS")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", nil, err
	}
	name := b.exe
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	dst := filepath.Join(appDir, name)
	in, err := os.Open(src)
	if err != nil {
		return "", nil, err
	}
	defer in.Close()
	tmp, err := os.CreateTemp(appDir, ".stage-*")
	if err != nil {
		return "", nil, err
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", nil, err
	}
	tmp.Chmod(0o755)
	tmp.Close()
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return "", nil, err
	}
	b.stagedPaths = append(b.stagedPaths, dst)
	b.refresh = false
	return dst, nil, nil
}

func (b *fakeBundler) SignBinary(_ context.Context, exe string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.signCalls = append(b.signCalls, exe)
	return nil
}

func (b *fakeBundler) NeedsRefresh([]string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.refresh
}

func (b *fakeBundler) counts() (stages, signs int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.stageCalls), len(b.signCalls)
}

func exePrintingModule(t *testing.T, msg string) string {
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
	exe, _ := os.Executable()
	fmt.Printf("%s exe=%s socket=%q\n", msg.Text, exe, os.Getenv("GGWD_REMOTE_SOCKET"))
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	<-c
}
`), 0o644)
	return dir
}

// In --display=window mode with a Bundler, the app runs directly from inside
// <bundle-dir>/App.dev.app/Contents/MacOS/<exe>, rebuilds stage into the same
// bundle, and writes inside bundle-dir are ignored by the file watcher.
func TestNativeWindowBundle(t *testing.T) {
	dir := exePrintingModule(t, "v1")
	bundleDir := filepath.Join(dir, "custom-bundles")
	b := &fakeBundler{dir: bundleDir, exe: "myapp"}
	var out, log syncBuffer
	n := &Native{
		Dir: dir, Package: ".", OutDir: filepath.Join(dir, "build"),
		Bundle: b, Watch: true,
		StopTimeout: 5 * time.Second,
		Log:         &log, Stdout: &out, Stderr: &out,
	}
	cancel, done := runNative(t, n)
	defer cancel()

	wantSubstr := filepath.Join("App.dev.app", "Contents", "MacOS", "myapp")
	waitFor(t, &out, "v1 exe=")
	if !strings.Contains(out.String(), wantSubstr) {
		t.Fatalf("app did not launch from bundle path %q:\n%s", wantSubstr, out.String())
	}

	// Writing inside bundleDir (which is outside OutDir) must NOT trigger a rebuild.
	os.WriteFile(filepath.Join(bundleDir, "extra.txt"), []byte("ignored"), 0o644)
	time.Sleep(350 * time.Millisecond)
	if stages, _ := b.counts(); stages != 1 {
		t.Errorf("Stage called %d times after writing in bundleDir, want 1", stages)
	}

	// Editing Go source rebuilds and stages v2 into the bundle.
	writeMsg(t, dir, "v2")
	waitFor(t, &out, "v2 exe=")
	if stages, _ := b.counts(); stages != 2 {
		t.Errorf("Stage called %d times after rebuild, want 2", stages)
	}

	cancel()
	if err := wait(t, done); err != nil {
		t.Errorf("Run = %v", err)
	}
}

// RemoteDisplay.Start stages the viewer binary through Bundle.Stage so on macOS
// ggviewer runs inside <Name>.dev.app; Stale triggers a clean viewer restart
// when Bundle.NeedsRefresh reports changed plist/icon metadata.
func TestRemoteDisplayStagesViewerInBundle(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a tiny fake viewer program")
	}
	dir := t.TempDir()
	// Build a minimal fake ggviewer that prints its own os.Executable() to stderr
	// and speaks the ready/quit JSON control channel.
	viewerDir := filepath.Join(dir, "viewer")
	os.MkdirAll(viewerDir, 0o755)
	os.WriteFile(filepath.Join(viewerDir, "go.mod"), []byte("module fakeviewer\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(viewerDir, "main.go"), []byte(`package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	sock := flag.String("socket", "", "")
	flag.String("project", "", "")
	flag.Parse()
	exe, _ := os.Executable()
	fmt.Fprintf(os.Stderr, "viewer-exe=%s\n", exe)
	json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "ready", "socket": *sock})
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
	}
}
`), 0o644)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	viewerBin := filepath.Join(dir, "ggviewer")
	if runtime.GOOS == "windows" {
		viewerBin += ".exe"
	}
	// Build using Native's gotool via go build in the test.
	srcDir := filepath.Join(dir, "bundles")
	b := &fakeBundler{dir: srcDir, exe: "MyApp"}
	var stderr, log syncBuffer
	if err := buildFakeViewer(t, viewerDir, viewerBin); err != nil {
		t.Fatal(err)
	}
	rd := &RemoteDisplay{
		Dir:    dir,
		Exe:    viewerBin,
		Bundle: b,
		Stderr: &stderr,
		Log:    &log,
	}
	v, err := rd.Start(t.Context())
	if err != nil {
		t.Fatalf("RemoteDisplay.Start: %v", err)
	}
	defer v.Stop(5 * time.Second)

	wantSubstr := filepath.Join("App.dev.app", "Contents", "MacOS", "MyApp")
	waitFor(t, &stderr, "viewer-exe=")
	if !strings.Contains(stderr.String(), wantSubstr) {
		t.Fatalf("viewer did not launch from bundle %q:\n%s", wantSubstr, stderr.String())
	}
	if stages, _ := b.counts(); stages != 1 {
		t.Errorf("Bundle.Stage calls = %d, want 1", stages)
	}
	if rd.Stale(t.Context(), []string{filepath.Join(dir, "main.go")}) {
		t.Error("Stale = true when bundle does not need refresh")
	}
	b.mu.Lock()
	b.refresh = true
	b.mu.Unlock()
	if !rd.Stale(t.Context(), []string{filepath.Join(dir, "icon.png")}) {
		t.Error("Stale = false when Bundle.NeedsRefresh is true")
	}
}

func buildFakeViewer(t *testing.T, dir, out string) error {
	t.Helper()
	return gotool.Build(t.Context(), gotool.BuildOptions{Dir: dir, Package: ".", Output: out})
}

// In --display=remote mode, the running viewer owns the bundle; rebuilding the
// app must NOT call Stage on the bundle while the viewer runs (preventing
// live _CodeSignature invalidation), and instead runs the app outside the
// bundle with SignBinary. If the viewer crashes, fallback stages the app into
// the bundle.
func TestNativeRemoteDoesNotMutateViewerBundle(t *testing.T) {
	dir := exePrintingModule(t, "v1")
	bundleDir := filepath.Join(dir, "bundles")
	b := &fakeBundler{dir: bundleDir, exe: "myapp"}
	d := &fakeDisplay{}
	var out, log syncBuffer
	n := &Native{
		Dir: dir, Package: ".", OutDir: filepath.Join(dir, "build"),
		Display: d, Bundle: b, Watch: true,
		StopTimeout: 5 * time.Second,
		Log:         &log, Stdout: &out, Stderr: &out,
	}
	cancel, done := runNative(t, n)
	defer cancel()

	waitFor(t, &out, `v1 exe=`)
	if strings.Contains(out.String(), "App.dev.app") {
		t.Fatalf("remote app ran inside the viewer bundle:\n%s", out.String())
	}
	if stages, signs := b.counts(); stages != 0 || signs != 1 {
		t.Errorf("initial remote run: stages=%d signs=%d, want 0, 1", stages, signs)
	}

	// Rebuilding in remote mode signs the standalone app binary without touching the bundle.
	writeMsg(t, dir, "v2")
	waitFor(t, &out, `v2 exe=`)
	if stages, signs := b.counts(); stages != 0 || signs != 2 {
		t.Errorf("after remote rebuild: stages=%d signs=%d, want 0, 2", stages, signs)
	}

	// When the viewer dies, fallback owns the bundle and stages the app into App.dev.app.
	close(d.viewer(0).events)
	wantSubstr := filepath.Join("App.dev.app", "Contents", "MacOS", "myapp")
	waitUntil(t, "fallback staged into bundle", func() bool {
		return strings.Contains(out.String(), "v2 exe=") && strings.Contains(out.String(), wantSubstr)
	})
	if stages, _ := b.counts(); stages != 1 {
		t.Errorf("after viewerLost fallback: stages=%d, want 1", stages)
	}

	cancel()
	if err := wait(t, done); err != nil {
		t.Errorf("Run = %v", err)
	}
}
