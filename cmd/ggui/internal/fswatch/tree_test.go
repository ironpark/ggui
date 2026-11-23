package fswatch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// runTree runs tree until the test ends, handing on what it reports.
func runTree(t *testing.T, tree *Tree) <-chan []string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan []string, 16)
	done := make(chan error, 1)
	go func() { done <- tree.Run(ctx, out) }()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("Run: %v", err)
		}
	})
	time.Sleep(300 * time.Millisecond) // for the system's watch to be in place
	return out
}

// told waits for a batch naming root, failing past a while.
func told(t *testing.T, out <-chan []string, root string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case b := <-out:
			if slices.Contains(b, root) {
				return
			}
		case <-deadline:
			t.Fatalf("no change told below %s", root)
		}
	}
}

// settled waits until nothing more is told of what was done before: a
// write and a change of its times may be told apart.
func settled(out <-chan []string) {
	for {
		select {
		case <-out:
		case <-time.After(500 * time.Millisecond):
			return
		}
	}
}

// treeQuiet fails if anything is told for a while.
func treeQuiet(t *testing.T, out <-chan []string) {
	t.Helper()
	select {
	case b := <-out:
		t.Fatalf("told of %v", b)
	case <-time.After(time.Second):
	}
}

// A change deep in a tree is told as a change below its root; one below
// an ignored directory is not; and Set moves the watch.
func TestTreeTellsOfChangesBelow(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	mkfiles(t, a, map[string]string{"src/deep/x.go": "x", "node_modules/m/i.js": "i"})
	mkfiles(t, b, map[string]string{"y.go": "y"})
	tree := NewTree(TreeOptions{Ignore: DefaultIgnore().Match, Settle: 50 * time.Millisecond})
	tree.Set([]string{a})
	out := runTree(t, tree)

	touch(t, filepath.Join(a, "src", "deep", "x.go"), "changed")
	told(t, out, a)
	settled(out)

	touch(t, filepath.Join(a, "node_modules", "m", "i.js"), "changed")
	treeQuiet(t, out)

	tree.Set([]string{b})
	time.Sleep(300 * time.Millisecond)
	touch(t, filepath.Join(b, "y.go"), "changed")
	told(t, out, b)
	settled(out)
	touch(t, filepath.Join(a, "src", "deep", "x.go"), "again")
	treeQuiet(t, out)
}

// On Linux a Tree watches each directory, and gives up past MaxDirs:
// the caller is told to look for itself.
func TestTreePastItsDirectoriesIsUnwatched(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("only inotify watches a directory at a time")
	}
	root := t.TempDir()
	files := map[string]string{}
	for i := range 10 {
		files[fmt.Sprintf("d%d/f", i)] = "f"
	}
	mkfiles(t, root, files)
	tree := NewTree(TreeOptions{MaxDirs: 5})
	tree.Set([]string{root})
	if err := tree.Run(context.Background(), make(chan []string)); !errors.Is(err, ErrUnwatched) {
		t.Fatalf("Run: %v, want ErrUnwatched", err)
	}
}

// What a Tree costs does not grow with the files below it: it keeps no
// path, so a tree of many files starts as fast as one of few.
func TestTreeKeepsNoPath(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{}
	for i := range 2000 {
		files[fmt.Sprintf("d%d/f%d", i%50, i)] = "f"
	}
	mkfiles(t, root, files)
	tree := NewTree(TreeOptions{})
	tree.Set([]string{root})
	out := runTree(t, tree)
	touch(t, filepath.Join(root, "d1", "f1"), "changed")
	told(t, out, root)
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if len(tree.roots) != 1 {
		t.Fatalf("roots %v", tree.roots)
	}
}

func TestTreeRootOfNestedAndClean(t *testing.T) {
	p := filepath.FromSlash
	tree := NewTree(TreeOptions{
		Ignore: func(path string, isDir bool) bool {
			return isDir && filepath.Base(path) == "build"
		},
	})
	roots := []string{p("/work"), p("/work/build")}

	// Nested explicitly watched root inside an ignored dir name matches longest root.
	if r, ok := tree.rootOf(roots, p("/work/build//out.txt")); !ok || r != p("/work/build") {
		t.Fatalf("rootOf nested = (%q, %v), want (%q, true)", r, ok, p("/work/build"))
	}
	// Ignored subdir below /work without its own root is still ignored.
	if r, ok := tree.rootOf(roots, p("/work/sub/build/out.txt")); ok {
		t.Fatalf("rootOf ignored = (%q, %v), want false", r, ok)
	}
	if _, ok := tree.rootOf(roots, ""); ok {
		t.Fatal("rootOf empty path matched")
	}
}
