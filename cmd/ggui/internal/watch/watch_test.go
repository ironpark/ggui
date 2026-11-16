package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// touch writes a file and moves its modification time on, so a change is
// seen on file systems whose times are coarse.
func touch(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Duration(len(data)+1) * time.Second)
	os.Chtimes(path, future, future)
}

func TestScan(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a.go"), filepath.Join(dir, "b.go"), filepath.Join(dir, "c.go")
	touch(t, a, "a")
	touch(t, b, "b")
	w := New([]string{dir, a, b, c})
	if got := w.Scan(); len(got) != 0 {
		t.Fatalf("first scan reported %v", got)
	}

	touch(t, a, "a2")
	if got := w.Scan(); !slices.Equal(got, []string{a}) {
		t.Errorf("after modifying a: %v", got)
	}
	if got := w.Scan(); len(got) != 0 {
		t.Errorf("a change was reported twice: %v", got)
	}

	// Removing a file is a change to it and to its directory.
	os.Remove(b)
	dirTime := time.Now().Add(time.Hour)
	os.Chtimes(dir, dirTime, dirTime)
	if got := w.Scan(); !slices.Equal(got, []string{dir, b}) {
		t.Errorf("after removing b: %v", got)
	}

	// A watched path that did not exist and now does.
	touch(t, c, "c")
	if got := w.Scan(); !slices.Contains(got, c) {
		t.Errorf("after creating c: %v", got)
	}

	// An editor's save by rename: same name, new file.
	tmp := filepath.Join(t.TempDir(), "a.tmp")
	touch(t, tmp, "a3-renamed")
	os.Rename(tmp, a)
	if got := w.Scan(); !slices.Contains(got, a) {
		t.Errorf("after replacing a by rename: %v", got)
	}
}

func TestSetKeepsStamps(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	touch(t, a, "a")
	touch(t, b, "b")
	w := New([]string{a})
	touch(t, a, "a2")
	// A change made before the set is replaced is still reported; a path
	// new to the set is not reported for existing.
	w.Set([]string{a, b})
	if got := w.Scan(); !slices.Equal(got, []string{a}) {
		t.Errorf("after Set: %v, want [a]", got)
	}
	w.Set([]string{b})
	touch(t, a, "a3")
	if got := w.Scan(); len(got) != 0 {
		t.Errorf("a path dropped from the set was reported: %v", got)
	}
}

// Run waits for the set to go quiet and sends the changes as one batch,
// however many scans saw them.
func TestRunDebounces(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for _, n := range []string{"a", "b", "c"} {
		p := filepath.Join(dir, n)
		touch(t, p, n)
		paths = append(paths, p)
	}
	w := New(paths)
	w.Interval = 10 * time.Millisecond
	w.Quiet = 150 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan []string)
	go w.Run(ctx, out)

	for i, p := range paths {
		touch(t, p, p+string(rune('0'+i)))
		time.Sleep(30 * time.Millisecond) // within the quiet period
	}
	select {
	case got := <-out:
		if !slices.Equal(got, paths) {
			t.Errorf("batch = %v, want all three", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no batch")
	}
	select {
	case got := <-out:
		t.Errorf("a second batch: %v", got)
	case <-time.After(300 * time.Millisecond):
	}
}

// Changes made while the receiver is busy wait as one batch.
func TestRunCoalescesWhileBusy(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	touch(t, a, "a")
	touch(t, b, "b")
	w := New([]string{a, b})
	w.Interval, w.Quiet = 10*time.Millisecond, 20*time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan []string)
	go w.Run(ctx, out)

	touch(t, a, "a1")
	time.Sleep(100 * time.Millisecond) // a's batch is ready; nobody receives
	touch(t, b, "b1")
	time.Sleep(100 * time.Millisecond)
	select {
	case got := <-out:
		if !slices.Equal(got, []string{a, b}) {
			t.Errorf("batch = %v, want [a b]", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no batch")
	}
}
