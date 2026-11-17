//go:build unix

package gotool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A cancelled build is killed with the compilers it started, at once, and
// leaves its work directory in TmpDir, not in the system's.
func TestBuildCancelKillsTheTree(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module m\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport (\n\t_ \"go/types\"\n\t_ \"net/http\"\n)\n\nfunc main() {}\n"), 0o644)
	// An empty cache, so that the build has the standard library to
	// compile and is still running when it is cancelled.
	cache := filepath.Join(dir, "cache")
	t.Setenv("GOCACHE", cache)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	work := filepath.Join(dir, "work")
	os.Mkdir(work, 0o755)

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- Build(ctx, BuildOptions{Dir: dir, Package: ".", Output: filepath.Join(dir, "out"), TmpDir: work})
	}()
	// Until a compiler runs.
	deadline := time.Now().Add(20 * time.Second)
	for !running(t, work) {
		if time.Now().After(deadline) {
			t.Skip("no compiler seen running")
		}
		time.Sleep(20 * time.Millisecond)
	}
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled build succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Build did not return after cancel")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("Build returned %s after cancel", d)
	}
	time.Sleep(100 * time.Millisecond)
	if running(t, work) {
		t.Error("compilers of the cancelled build still run")
	}
	// The work directory, left behind, is in TmpDir for the caller to
	// remove.
	if entries, _ := os.ReadDir(work); len(entries) == 0 {
		t.Error("the go command's work directory is not in TmpDir")
	}
}

// running reports whether a process mentions the work directory, as the
// compilers the go command starts do in their arguments.
func running(t *testing.T, work string) bool {
	out, err := exec.Command("ps", "-axo", "command=").Output()
	if err != nil {
		t.Skipf("ps: %v", err)
	}
	for _, l := range strings.Split(string(out), "\n") {
		if strings.Contains(l, work) && !strings.HasPrefix(l, "ps ") {
			return true
		}
	}
	return false
}
