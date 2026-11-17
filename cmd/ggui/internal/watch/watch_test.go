package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
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
	w := New([]string{dir, a, b, c}, Options{})
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
	w := New([]string{a}, Options{})
	touch(t, a, "a2")
	// A change made before the set is replaced is still reported; a path
	// new to the set is not reported for existing.
	if added := w.Set([]string{a, b}); !slices.Equal(added, []string{b}) {
		t.Errorf("Set added %v, want [b]", added)
	}
	if got := w.Scan(); !slices.Equal(got, []string{a}) {
		t.Errorf("after Set: %v, want [a]", got)
	}
	w.Set([]string{b})
	touch(t, a, "a3")
	if got := w.Scan(); len(got) != 0 {
		t.Errorf("a path dropped from the set was reported: %v", got)
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
		os.MkdirAll(p.path("build"), 0o755)
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
		os.Remove(extra)
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
			if err := os.Rename(tmp, main); err != nil {
				t.Fatal(err)
			}
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
