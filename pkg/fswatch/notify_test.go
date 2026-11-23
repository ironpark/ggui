package fswatch

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// openTestSource returns the platform's recursive source, skipping where
// there is none.
func openTestSource(t *testing.T) source {
	t.Helper()
	s, err := newSource(true, nil)
	if err != nil {
		t.Skipf("no source: %v", err)
	}
	t.Cleanup(s.close)
	return s
}

// reported waits until the source has reported each of want, or said
// that events were lost, and drains it.
func reported(t *testing.T, s source, what string, want ...string) {
	t.Helper()
	missing := slices.Clone(want)
	deadline := time.After(5 * time.Second)
	for len(missing) > 0 {
		select {
		case <-s.wake():
			paths, all, err := s.take()
			if err != nil {
				t.Fatalf("%s: the source failed: %v", what, err)
			}
			if all {
				missing = nil
				break
			}
			missing = slices.DeleteFunc(missing, func(m string) bool { return slices.Contains(paths, m) })
		case <-deadline:
			t.Fatalf("%s: never reported %v", what, missing)
		}
	}
	quiet(s, 100*time.Millisecond)
}

// quiet drains what the source reports until it has said nothing for d.
func quiet(s source, d time.Duration) {
	idle := time.NewTimer(d)
	defer idle.Stop()
	for {
		idle.Reset(d)
		select {
		case <-s.wake():
			s.take()
		case <-idle.C:
			return
		}
	}
}

// TestSource puts the platform's source through what editors and tools do
// to files, each reported by the changed path.
func TestSource(t *testing.T) {
	s := openTestSource(t)
	dir := t.TempDir()
	a := filepath.Join(dir, "a.go")
	write(t, a, "package a\n")
	if err := s.watch([]string{dir}); err != nil {
		t.Fatal(err)
	}
	// Some sources start a moment after they are asked to.
	time.Sleep(100 * time.Millisecond)
	quiet(s, 50*time.Millisecond)

	write(t, a, "package a // in place\n")
	reported(t, s, "write in place", a)

	tmp := filepath.Join(dir, ".a.go.swp")
	write(t, tmp, "package a // renamed over\n")
	rename(t, tmp, a)
	reported(t, s, "write a temporary file and rename it over", a)

	b := filepath.Join(dir, "b.go")
	write(t, b, "package a\n")
	reported(t, s, "create", b)

	remove(t, b)
	reported(t, s, "delete", b)

	sub := filepath.Join(dir, "sub")
	mkdir(t, sub)
	c := filepath.Join(sub, "c.go")
	write(t, c, "package sub\n")
	reported(t, s, "create a directory with a file in it", sub)
	// The file in the new directory is reported too, once the source
	// watches the directory, which it may start to a moment late.
	write(t, c, "package sub // again\n")
	reported(t, s, "write in a new directory", c)

	moved := filepath.Join(dir, "moved")
	rename(t, sub, moved)
	reported(t, s, "rename a directory", sub, moved)
	write(t, filepath.Join(moved, "c.go"), "package sub // after the rename\n")
	reported(t, s, "write in a renamed directory", filepath.Join(moved, "c.go"))
}

// TestSourceWatchReplaced watches a second directory instead of the first.
func TestSourceWatchReplaced(t *testing.T) {
	s := openTestSource(t)
	one, two := t.TempDir(), t.TempDir()
	if err := s.watch([]string{one}); err != nil {
		t.Fatal(err)
	}
	if err := s.watch([]string{two}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	quiet(s, 50*time.Millisecond)
	f := filepath.Join(two, "f")
	write(t, f, "x")
	reported(t, s, "write in the directory watched now", f)
}

// TestSourceLoad makes thousands of changes at once. Every one is
// reported, or the source says that events were lost; nothing breaks.
func TestSourceLoad(t *testing.T) {
	s := openTestSource(t)
	dir := t.TempDir()
	if err := s.watch([]string{dir}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	quiet(s, 50*time.Millisecond)

	const n = 3000
	want := make(map[string]bool, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("f%04d", i))
		write(t, p, "x")
		want[p] = true
	}
	deadline := time.After(20 * time.Second)
	lost := false
	for len(want) > 0 && !lost {
		select {
		case <-s.wake():
			paths, all, _ := s.take()
			lost = all
			for _, p := range paths {
				delete(want, p)
			}
		case <-deadline:
			t.Fatalf("%d of %d changes never reported", len(want), n)
		}
	}
	t.Logf("lost events: %v (%s)", lost, runtime.GOOS)
	// It keeps working after the burst.
	quiet(s, 200*time.Millisecond)
	f := filepath.Join(dir, "after")
	write(t, f, "x")
	reported(t, s, "write after the burst", f)
}

// TestSourceStress, with FSWATCH_STRESS set to a duration, writes files as
// fast as it can in two directories while the source is switched between
// them and the garbage collector runs, then checks that the source still
// reports a change. It is how the FSEvents callback, which enters Go on a
// thread Go did not start, was shown to hold up with and without cgo; too
// long for every run.
func TestSourceStress(t *testing.T) {
	d, err := time.ParseDuration(os.Getenv("FSWATCH_STRESS"))
	if err != nil {
		t.Skip("set FSWATCH_STRESS to a duration to run")
	}
	s := openTestSource(t)
	one, two := t.TempDir(), t.TempDir()
	stop := make(chan struct{})
	done := make(chan int)
	for _, dir := range []string{one, two} {
		go func() {
			n := 0
			for {
				select {
				case <-stop:
					done <- n
					return
				default:
				}
				p := filepath.Join(dir, fmt.Sprintf("f%d", n%500))
				os.WriteFile(p, []byte("x"), 0o644)
				n++
			}
		}()
	}
	go func() {
		for {
			select {
			case <-stop:
				done <- 0
				return
			default:
				runtime.GC()
				time.Sleep(time.Millisecond)
			}
		}
	}()
	end := time.Now().Add(d)
	taken, rounds := 0, 0
	window := time.NewTimer(50 * time.Millisecond)
	defer window.Stop()
	for time.Now().Before(end) {
		dirs := []string{one}
		if rounds%2 == 1 {
			dirs = []string{two}
		}
		if rounds%3 == 2 {
			dirs = []string{one, two}
		}
		if err := s.watch(dirs); err != nil {
			t.Fatal(err)
		}
		rounds++
		window.Reset(50 * time.Millisecond)
	drain:
		for {
			select {
			case <-s.wake():
				p, _, _ := s.take()
				taken += len(p)
			case <-window.C:
				break drain
			}
		}
	}
	close(stop)
	writes := <-done + <-done + <-done
	t.Logf("%d writes, %d paths reported, %d streams", writes, taken, rounds)
	if err := s.watch([]string{one}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	quiet(s, 100*time.Millisecond)
	f := filepath.Join(one, "after")
	write(t, f, "x")
	reported(t, s, "write after the stress", f)
}

// A root watched that is deleted has the source say that events were
// lost, or name the root: either way the watcher looks.
func TestSourceRootRemoved(t *testing.T) {
	s := openTestSource(t)
	dir := filepath.Join(t.TempDir(), "root")
	mkdir(t, dir)
	if err := s.watch([]string{dir}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	quiet(s, 50*time.Millisecond)
	remove(t, dir)
	reported(t, s, "the root removed", dir)
}

func TestRoots(t *testing.T) {
	sep := string(filepath.Separator)
	p := func(s string) string { return filepath.FromSlash(s) }
	got := roots([]string{p("/a/b/c"), p("/a/b-c"), p("/a/b"), p("/x"), p("/a/b/c/d"), p("/x/")})
	want := []string{p("/a/b"), p("/a/b-c"), p("/x")}
	if !slices.Equal(got, want) {
		t.Errorf("roots = %v, want %v", got, want)
	}
	if !within(sep, p("/a")) || within(p("/a/b"), p("/a/bc")) || !within(p("/a/b"), p("/a/b/c")) {
		t.Error("within is wrong")
	}
}
