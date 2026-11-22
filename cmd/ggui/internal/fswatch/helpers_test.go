package fswatch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func mkfiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// touch writes a file and moves its modification time on, so a change is
// seen on file systems whose times are coarse.
func touch(t *testing.T, path, data string) {
	t.Helper()
	write(t, path, data)
	future := time.Now().Add(time.Duration(len(data)+1) * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

// rename saves as an editor does, by renaming a file over another, and
// remove deletes one. Both try again when Windows refuses because the
// file is open, as it is for a moment while the watcher under test reads
// it or an antivirus scans it: an editor does the same. An operation that
// still fails is the test's failure, said as what it is, rather than a
// scan that then finds nothing changed.
func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := retry(func() error { return os.Rename(from, to) }); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := retry(func() error { return os.RemoveAll(path) }); err != nil {
		t.Fatal(err)
	}
}

func retry(op func() error) error {
	err := op()
	for deadline := time.Now().Add(time.Second); err != nil && transient(err) && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
		err = op()
	}
	return err
}

// testOptions are quick to notice, for tests: with notifications only
// they find a change in time, as the safety net is a minute away.
func testOptions(t *testing.T, poll bool) Options {
	return Options{
		NoNotify: poll,
		Poll:     40 * time.Millisecond,
		Rescan:   time.Minute,
		Logf:     t.Logf,
		Tracef:   t.Logf,
	}
}

// start runs w until the test ends.
func start(t *testing.T, w *Watcher) <-chan Batch {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Batch)
	done := make(chan struct{})
	go func() {
		w.Run(ctx, out)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	// The source starts, and the fingerprints are taken.
	time.Sleep(150 * time.Millisecond)
	return out
}

// forBoth runs f with the system's notifications and with polling.
func forBoth(t *testing.T, f func(t *testing.T, poll bool)) {
	t.Run("notified", func(t *testing.T) { f(t, false) })
	t.Run("polling", func(t *testing.T) { f(t, true) })
}

// expect gathers batches until every one of want has been reported, as
// the batches merged say, and then until nothing more comes for a while,
// and fails if the outcome is not exactly want.
func expect(t *testing.T, out <-chan Batch, what string, want ...Event) {
	t.Helper()
	var got Batch
	has := func() bool {
		for _, w := range want {
			if !slices.Contains(got.Changes, w) {
				return false
			}
		}
		return true
	}
	deadline := time.After(5 * time.Second)
	for !has() {
		select {
		case b := <-out:
			got.merge(b)
		case <-deadline:
			t.Fatalf("%s: got %v, want %v", what, got.Changes, want)
		}
	}
	quiet := time.After(300 * time.Millisecond)
	for {
		select {
		case b := <-out:
			got.merge(b)
		case <-quiet:
			sortEvents(want)
			if !slices.Equal(got.Changes, want) {
				t.Errorf("%s: got %v, want %v", what, got.Changes, want)
			}
			return
		}
	}
}

// none checks that nothing is reported for a while.
func none(t *testing.T, out <-chan Batch, what string) {
	t.Helper()
	select {
	case b := <-out:
		t.Errorf("%s: reported %v", what, b.Changes)
	case <-time.After(400 * time.Millisecond):
	}
}

func ev(op Op, path string) Event { return Event{Path: path, Op: op} }

func dirEv(op Op, path string) Event { return Event{Path: path, Op: op, Dir: true} }

// fakeSource is a source the test drives.
type fakeSource struct {
	queue
	dirs   chan []string
	closed chan struct{}
}

func newFakeSource() *fakeSource {
	return &fakeSource{queue: newQueue(), dirs: make(chan []string, 100), closed: make(chan struct{})}
}

func (f *fakeSource) watch(dirs []string) error { f.dirs <- dirs; return nil }
func (f *fakeSource) close()                    { close(f.closed) }

// withFake has w use src for its notifications.
func withFake(w *Watcher, src *fakeSource) {
	w.newSource = func(bool, func(string) bool) (source, error) { return src, nil }
}

func watched(w *Watcher) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.files)
}
