package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// touch writes a file and moves its modification time on, so a change is
// seen on file systems whose times are coarse.
func touch(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Duration(len(data)+1) * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
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
	if err := shared.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := shared.Remove(path); err != nil {
		t.Fatal(err)
	}
}

// pkg is a package directory under test: its Go files, an embedded file
// in a directory git would ignore, and what the go command would list.
type pkg struct {
	t   *testing.T
	dir string

	mu    sync.Mutex
	extra []string // more files List returns
	lists int
}

const mainGo = `package main

import _ "embed"

//go:embed gen/data.txt
var data string

// main prints.
func main() { println(data) }
`

func newPkg(t *testing.T) *pkg {
	dir := t.TempDir()
	mkfiles(t, dir, map[string]string{
		"main.go":                 mainGo,
		"gen/data.txt":            "one",
		"node_modules/x/index.js": "x",
	})
	return &pkg{t: t, dir: dir}
}

func (p *pkg) path(name string) string { return filepath.Join(p.dir, filepath.FromSlash(name)) }

func (p *pkg) list(context.Context) ([]string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lists++
	return append([]string{p.dir, p.path("main.go"), p.path("gen/data.txt")}, p.extra...), nil
}

// run starts a watcher on the package, polling or not.
func (p *pkg) run(poll bool) <-chan Change {
	t := p.t
	if poll {
		t.Setenv(PollEnv, "poll")
	} else {
		t.Setenv(PollEnv, "")
	}
	set, _ := p.list(context.Background())
	o := Options{
		List:   p.list,
		Ignore: NewIgnore(p.path("build")),
		Settle: 15 * time.Millisecond,
		Poll:   50 * time.Millisecond,
		// Long enough that, with notifications, only they find a change
		// in time.
		SafetyNet: time.Minute,
		Logf:      t.Logf,
		Tracef:    t.Logf,
	}
	w := New(set, o)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan Change)
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
	time.Sleep(200 * time.Millisecond)
	return out
}

func next(t *testing.T, out <-chan Change, what string) Change {
	t.Helper()
	select {
	case c := <-out:
		return c
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: no change reported", what)
		return Change{}
	}
}

func none(t *testing.T, out <-chan Change, what string) {
	t.Helper()
	select {
	case c := <-out:
		t.Errorf("%s: reported %+v", what, c)
	case <-time.After(500 * time.Millisecond):
	}
}

// forBoth runs f with the system's notifications and with polling.
func forBoth(t *testing.T, f func(t *testing.T, poll bool)) {
	t.Run("notified", func(t *testing.T) { f(t, false) })
	t.Run("polling", func(t *testing.T) { f(t, true) })
}

func TestRun(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		p := newPkg(t)
		out := p.run(poll)
		main := p.path("main.go")

		write(t, main, mainGo+"\nfunc f() {}\n")
		c := next(t, out, "a real edit")
		if !slices.Equal(c.Real, []string{main}) || len(c.Cosmetic) > 0 || c.At.IsZero() {
			t.Errorf("a real edit: %+v", c)
		}

		write(t, main, mainGo+"\n// f does nothing.\nfunc f() {}\n")
		c = next(t, out, "a comment")
		if len(c.Real) > 0 || !slices.Equal(c.Cosmetic, []string{main}) {
			t.Errorf("a comment: %+v", c)
		}

		write(t, main, mainGo+"\n// f does nothing.\nfunc f() {\n}\n")
		c = next(t, out, "layout")
		if len(c.Real) > 0 || !slices.Equal(c.Cosmetic, []string{main}) {
			t.Errorf("layout: %+v", c)
		}

		// Embedded, below a directory with a generated name: still real.
		data := p.path("gen/data.txt")
		write(t, data, "two")
		c = next(t, out, "an embedded file")
		if !slices.Equal(c.Real, []string{data}) {
			t.Errorf("an embedded file: %+v", c)
		}

		// A test file is not in the build: the directory changed, the
		// listing says nothing is new, and nothing is reported.
		write(t, p.path("main_test.go"), "package main\n")
		none(t, out, "a new test file")
		write(t, p.path("main_test.go"), "package main\n\nimport \"testing\"\n")
		none(t, out, "an edited test file")

		// Ignored: below node_modules, and the output directory.
		write(t, p.path("node_modules/x/index.js"), "y")
		none(t, out, "node_modules")
		if err := os.MkdirAll(p.path("build"), 0o755); err != nil {
			t.Fatal(err)
		}
		none(t, out, "the output directory made")
		write(t, p.path("build/app"), "binary")
		none(t, out, "a build written")

		// A new Go file the listing now has.
		extra := p.path("extra.go")
		p.mu.Lock()
		p.extra = []string{extra}
		p.mu.Unlock()
		write(t, extra, "package main\n")
		c = next(t, out, "a new Go file")
		if !slices.Contains(c.Real, extra) {
			t.Errorf("a new Go file: %+v", c)
		}

		// Deleted.
		remove(t, extra)
		c = next(t, out, "a deleted Go file")
		if !slices.Contains(c.Real, extra) {
			t.Errorf("a deleted Go file: %+v", c)
		}
	})
}

// An editor that saves by writing a temporary file and renaming it over
// the old one is one change, real or not.
func TestRunRenameSave(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		p := newPkg(t)
		out := p.run(poll)
		main := p.path("main.go")
		save := func(data string) {
			tmp := main + ".tmp~"
			write(t, tmp, data)
			rename(t, tmp, main)
		}
		save(mainGo + "\nfunc g() {}\n")
		c := next(t, out, "a real save")
		if !slices.Equal(c.Real, []string{main}) {
			t.Errorf("a real save: %+v", c)
		}
		none(t, out, "after a real save")

		save(mainGo + "\nfunc g() {} // g\n")
		c = next(t, out, "a comment saved")
		if len(c.Real) > 0 || !slices.Equal(c.Cosmetic, []string{main}) {
			t.Errorf("a comment saved: %+v", c)
		}
	})
}

// Changes made while the receiver is busy wait as one.
func TestRunCoalescesWhileBusy(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		p := newPkg(t)
		out := p.run(poll)
		main, data := p.path("main.go"), p.path("gen/data.txt")
		write(t, main, mainGo+"\nvar x = 1\n")
		time.Sleep(300 * time.Millisecond) // found; nobody receives
		write(t, data, "three")
		time.Sleep(300 * time.Millisecond)
		c := next(t, out, "two changes")
		if !slices.Equal(c.Real, []string{data, main}) {
			t.Errorf("change = %+v, want both", c)
		}
	})
}

func TestChangeMerge(t *testing.T) {
	t0 := time.Now()
	c := Change{Real: []string{"b"}, Cosmetic: []string{"a", "c"}, At: t0.Add(time.Second)}
	c.merge(Change{Real: []string{"a"}, Cosmetic: []string{"d"}, At: t0})
	if !slices.Equal(c.Real, []string{"a", "b"}) || !slices.Equal(c.Cosmetic, []string{"c", "d"}) || !c.At.Equal(t0) {
		t.Errorf("merged = %+v", c)
	}
}

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

func writeRetry(path string, data []byte) error {
	var err error
	for deadline := time.Now().Add(500 * time.Millisecond); ; time.Sleep(5 * time.Millisecond) {
		if err = os.WriteFile(path, data, 0o644); err == nil || !time.Now().Before(deadline) {
			return err
		}
	}
}

// A scan that catches a save between its truncating the file and writing
// it sees an empty file. The edit is judged by what the save writes: one
// to a comment is not a real change because the file was empty in passing.
func TestClassifyWaitsOutATruncatedSave(t *testing.T) {
	t.Setenv(PollEnv, "poll")
	path := filepath.Join(t.TempDir(), "main.go")
	const src = "package main\n\nfunc main() {}\n"
	touch(t, path, src)
	w := New([]string{path}, Options{Steady: 2 * time.Second})
	// Wait deterministically for New's background fingerprint goroutine:
	// Run with a canceled context returns once background work finishes.
	doneCtx, cancel := context.WithCancel(context.Background())
	cancel()
	w.fs.Run(doneCtx, nil)

	touch(t, path, "")
	done := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		done <- writeRetry(path, []byte(src+"// A comment.\n"))
	}()
	real, cosmetic, _, _ := w.classify(w.fs.Sync(context.Background()))
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(real) > 0 || !slices.Equal(cosmetic, []string{path}) {
		t.Errorf("real %v, cosmetic %v: want the comment's edit cosmetic", real, cosmetic)
	}
	if b := w.fs.Sync(context.Background()); len(b.Changes) > 0 {
		t.Errorf("the next scan judges the file again: %v", b.Changes)
	}
}

// The rules a listing is judged by are git's; the entries of a package
// directory leave out only what is never worth a look.
func TestIgnore(t *testing.T) {
	root := t.TempDir()
	ig := NewIgnore(filepath.Join(root, "build"))
	for _, c := range []struct {
		path       string
		git, entry bool
	}{
		{"main.go", false, false},
		{"build/app", true, true},
		{".git/HEAD", true, true},
		{"main.go~", false, true},
		{"4913", false, true},
		{"node_modules/x.js", true, true}, // outside a work tree, the fallback
	} {
		p := filepath.Join(root, filepath.FromSlash(c.path))
		if got := ig.Ignored(p); got != c.git {
			t.Errorf("%s: ignored = %v, want %v", c.path, got, c.git)
		}
		if got := ig.entry(p, false); got != c.entry {
			t.Errorf("%s: entry ignored = %v, want %v", c.path, got, c.entry)
		}
	}
	var none *Ignore
	if none.Ignored("/x/.git/HEAD") || none.entry("/x/.git", true) {
		t.Error("nil Ignore ignored a path")
	}
	none.Reset()
}
