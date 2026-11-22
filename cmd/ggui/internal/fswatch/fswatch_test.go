package fswatch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Sync, with no Run, scans by itself: each way a file changes is seen.
func TestSync(t *testing.T) {
	dir := t.TempDir()
	a, b, c := filepath.Join(dir, "a"), filepath.Join(dir, "b"), filepath.Join(dir, "c")
	touch(t, a, "a")
	touch(t, b, "b")
	w := New(Options{NoNotify: true})
	w.Set([]string{a, b, c})
	ctx := context.Background()
	if got := w.Sync(ctx); len(got.Changes) != 0 {
		t.Fatalf("first scan reported %v", got.Changes)
	}
	if n := watched(w); n != 3 {
		t.Errorf("watched = %d, want 3", n)
	}

	touch(t, a, "a2")
	if got := w.Sync(ctx).Changes; !slices.Equal(got, []Event{ev(Write, a)}) {
		t.Errorf("after modifying a: %v", got)
	}
	if got := w.Sync(ctx).Changes; len(got) != 0 {
		t.Errorf("a change was reported twice: %v", got)
	}

	remove(t, b)
	if got := w.Sync(ctx).Changes; !slices.Equal(got, []Event{ev(Remove, b)}) {
		t.Errorf("after removing b: %v", got)
	}

	// A path given that did not exist and now does.
	touch(t, c, "c")
	if got := w.Sync(ctx).Changes; !slices.Equal(got, []Event{ev(Create, c)}) {
		t.Errorf("after creating c: %v", got)
	}

	// An editor's save by rename: same name, new file.
	tmp := filepath.Join(t.TempDir(), "a.tmp")
	touch(t, tmp, "a3-renamed")
	rename(t, tmp, a)
	if got := w.Sync(ctx).Changes; !slices.Equal(got, []Event{ev(Write, a)}) {
		t.Errorf("after replacing a by rename: %v", got)
	}

	// Deleted and made again between two scans: written.
	remove(t, c)
	touch(t, c, "c again")
	if got := w.Sync(ctx).Changes; !slices.Equal(got, []Event{ev(Write, c)}) {
		t.Errorf("after deleting and making c again: %v", got)
	}
}

func TestSetKeepsStamps(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	touch(t, a, "a")
	touch(t, b, "b")
	w := New(Options{NoNotify: true})
	w.Set([]string{a})
	touch(t, a, "a2")
	// A change made before the set is replaced is still reported; a path
	// new to the set is not reported for existing.
	if added := w.Set([]string{a, b}); !slices.Equal(added, []string{b}) {
		t.Errorf("Set added %v, want [b]", added)
	}
	if got := w.Sync(context.Background()).Changes; !slices.Equal(got, []Event{ev(Write, a)}) {
		t.Errorf("after Set: %v, want a written", got)
	}
	w.Set([]string{b})
	touch(t, a, "a3")
	if got := w.Sync(context.Background()).Changes; len(got) != 0 {
		t.Errorf("a path dropped from the set was reported: %v", got)
	}
}

// A directory given stands for its entries: those added, written and
// removed, and a rename as a remove and a create.
func TestDirectory(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		mkfiles(t, dir, map[string]string{"a": "a"})
		w := New(testOptions(t, poll))
		w.Set([]string{dir})
		out := start(t, w)
		a, x := filepath.Join(dir, "a"), filepath.Join(dir, "x")

		write(t, x, "x")
		expect(t, out, "a file added", ev(Create, x))
		write(t, x, "x2")
		expect(t, out, "a file written", ev(Write, x))
		y := filepath.Join(dir, "y")
		rename(t, x, y)
		expect(t, out, "a file renamed", ev(Remove, x), ev(Create, y))
		remove(t, y)
		expect(t, out, "a file removed", ev(Remove, y))

		sub := filepath.Join(dir, "sub")
		mkdir(t, sub)
		expect(t, out, "a directory made", dirEv(Create, sub))
		// Not recursive: what is in it is not watched.
		write(t, filepath.Join(sub, "f"), "f")
		none(t, out, "a file in a subdirectory")

		write(t, a, "a2")
		expect(t, out, "the first file written", ev(Write, a))
	})
}

// A recursive watch follows directories made below it, and leaves out
// what Ignore ignores.
func TestRecursive(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		mkfiles(t, dir, map[string]string{"a/b/c": "c", "node_modules/m/i.js": "x", "out/bin": "x"})
		o := testOptions(t, poll)
		o.Recursive = true
		o.Ignore = NewIgnore(IgnoreConfig{Patterns: DefaultPatterns, Dirs: []string{filepath.Join(dir, "out")}}).Match
		w := New(o)
		w.Set([]string{dir})
		out := start(t, w)
		// dir, a, a/b, a/b/c
		if n := watched(w); n != 4 {
			t.Errorf("watched = %d, want 4", n)
		}

		c := filepath.Join(dir, "a", "b", "c")
		write(t, c, "c2")
		expect(t, out, "a file deep down written", ev(Write, c))

		d := filepath.Join(dir, "a", "d")
		mkdir(t, filepath.Join(d, "e"))
		f := filepath.Join(d, "e", "f")
		write(t, f, "f")
		expect(t, out, "a tree made", dirEv(Create, d), dirEv(Create, filepath.Join(d, "e")), ev(Create, f))
		write(t, f, "f2")
		expect(t, out, "a file in the new tree written", ev(Write, f))

		write(t, filepath.Join(dir, "node_modules", "m", "i.js"), "y")
		write(t, filepath.Join(dir, "out", "bin"), "y")
		mkdir(t, filepath.Join(dir, ".git"))
		write(t, filepath.Join(dir, ".git", "HEAD"), "ref")
		write(t, filepath.Join(dir, "a", "c.swp"), "swap")
		write(t, filepath.Join(dir, "a", "4913"), "")
		none(t, out, "ignored paths")

		remove(t, d)
		expect(t, out, "a tree removed", dirEv(Remove, d), dirEv(Remove, filepath.Join(d, "e")), ev(Remove, f))
	})
}

// An editor that saves by writing a temporary file and renaming it over
// the old one makes one change, to the file saved.
func TestRenameSave(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		main := filepath.Join(dir, "main.go")
		write(t, main, "package main\n")
		o := testOptions(t, poll)
		o.Ignore = NewIgnore(IgnoreConfig{Patterns: DefaultPatterns}).Match
		w := New(o)
		w.Set([]string{dir})
		out := start(t, w)
		for i, tmp := range []string{main + "~", filepath.Join(dir, ".#main.go"), filepath.Join(dir, "main.go___jb_tmp___")} {
			write(t, tmp, fmt.Sprintf("package main // %d\n", i))
			rename(t, tmp, main)
			expect(t, out, "saved through "+filepath.Base(tmp), ev(Write, main))
		}
		// A file given, not its directory: the same.
		w.Set([]string{main})
		tmp := filepath.Join(dir, "save.tmp")
		write(t, tmp, "package main // given\n")
		rename(t, tmp, main)
		expect(t, out, "a file given saved", ev(Write, main))
		// Deleted, then made again, as some programs save.
		remove(t, main)
		time.Sleep(10 * time.Millisecond)
		write(t, main, "package main // made again\n")
		expect(t, out, "deleted and made again", ev(Write, main))
	})
}

// Changes made while the receiver is busy wait as one batch, and a burst
// of them is one batch.
func TestCoalesce(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		w := New(testOptions(t, poll))
		w.Set([]string{dir})
		out := start(t, w)
		var want []Event
		for i := range 50 {
			p := filepath.Join(dir, fmt.Sprintf("f%02d", i))
			write(t, p, "x")
			want = append(want, ev(Create, p))
		}
		time.Sleep(300 * time.Millisecond) // found; nobody receives
		write(t, filepath.Join(dir, "f00"), "y")
		time.Sleep(300 * time.Millisecond)
		select {
		case b := <-out:
			if !slices.Equal(b.Changes, want) {
				t.Errorf("one batch: %d changes %v, want the 50 files made", len(b.Changes), b.Changes)
			}
			if b.At.IsZero() {
				t.Error("the batch has no time")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("nothing reported")
		}
		none(t, out, "after the batch")
	})
}

func TestMerge(t *testing.T) {
	t0 := time.Now()
	b := Batch{At: t0.Add(time.Second), Changes: []Event{
		ev(Create, "a"), ev(Create, "b"), ev(Remove, "c"), ev(Write, "d"), {Path: "e", Op: Write, Same: true}, {Path: "f", Op: Write, Same: true},
	}}
	b.merge(Batch{At: t0, Changes: []Event{
		ev(Remove, "a"), ev(Write, "b"), ev(Create, "c"), ev(Remove, "d"), {Path: "e", Op: Write, Same: true}, ev(Write, "f"), ev(Create, "g"),
	}})
	want := []Event{ev(Create, "b"), ev(Write, "c"), ev(Remove, "d"), {Path: "e", Op: Write, Same: true}, ev(Write, "f"), ev(Create, "g")}
	if !slices.Equal(b.Changes, want) || !b.At.Equal(t0) {
		t.Errorf("merged = %v at %v, want %v", b.Changes, b.At, want)
	}
}

// commentBlind is a Content that judges files by their lines that are not
// comments.
var commentBlind = Content{
	Match: func(p string) bool { return strings.HasSuffix(p, ".src") },
	Sum: func(_ string, data []byte) []byte {
		var keep []string
		for l := range strings.SplitSeq(string(data), "\n") {
			if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
				keep = append(keep, l)
			}
		}
		return []byte(strings.Join(keep, "\n"))
	},
}

func TestContent(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		src, other := filepath.Join(dir, "a.src"), filepath.Join(dir, "b.txt")
		write(t, src, "one\n")
		write(t, other, "one\n")
		o := testOptions(t, poll)
		o.Content = commentBlind
		w := New(o)
		w.Set([]string{dir})
		out := start(t, w)

		write(t, src, "# a comment\none\n")
		expect(t, out, "a comment", Event{Path: src, Op: Write, Same: true})
		write(t, src, "# a comment\none\ntwo\n")
		expect(t, out, "a real edit", ev(Write, src))
		write(t, other, "# not judged\none\n")
		expect(t, out, "a file not judged", ev(Write, other))
		// A file made is judged from then on.
		n := filepath.Join(dir, "new.src")
		write(t, n, "x\n")
		expect(t, out, "a file made", ev(Create, n))
		write(t, n, "x\n# later\n")
		expect(t, out, "a comment in the file made", Event{Path: n, Op: Write, Same: true})
	})
}

func TestFilter(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.src")
	write(t, src, "one\n")
	o := testOptions(t, true)
	o.Content = commentBlind
	o.Filter = func(e Event) bool { return !e.Same }
	w := New(o)
	w.Set([]string{dir})
	out := start(t, w)
	write(t, src, "# a comment\none\n")
	none(t, out, "a comment, filtered out")
	write(t, src, "two\n")
	expect(t, out, "a real edit", ev(Write, src))
}

// A scan that catches a save between its truncating the file and writing
// it sees an empty file. The edit is judged by what the save writes: one
// to a comment is not a real change because the file was empty in passing.
func TestTruncatedSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.src")
	const src = "one\ntwo\n"
	touch(t, path, src)
	w := New(Options{NoNotify: true, Content: commentBlind, Steady: 2 * time.Second})
	w.Set([]string{path})
	w.bg.Wait() // the fingerprint is taken

	touch(t, path, "")
	done := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		done <- retry(func() error { return os.WriteFile(path, []byte(src+"# A comment.\n"), 0o644) })
	}()
	got := w.Sync(context.Background()).Changes
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []Event{{Path: path, Op: Write, Same: true}}) {
		t.Errorf("got %v, want the comment's edit the same", got)
	}
	if got := w.Sync(context.Background()).Changes; len(got) > 0 {
		t.Errorf("the next scan judges the file again: %v", got)
	}
}

// A file without Content found empty is looked at again before it is
// reported, so that it is reported once, whole; one that stays empty is
// reported when the time is up.
func TestSteady(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data")
	touch(t, path, "one")
	w := New(Options{NoNotify: true, Steady: 200 * time.Millisecond})
	w.Set([]string{path})

	touch(t, path, "")
	done := make(chan error, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		const data = "the whole of it"
		err := retry(func() error { return os.WriteFile(path, []byte(data), 0o644) })
		if err == nil {
			future := time.Now().Add(time.Duration(len(data)+1) * time.Second)
			err = retry(func() error { return os.Chtimes(path, future, future) })
		}
		done <- err
	}()
	if got := w.Sync(context.Background()).Changes; !slices.Equal(got, []Event{ev(Write, path)}) {
		t.Errorf("got %v, want one write", got)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := w.Sync(context.Background()).Changes; len(got) > 0 {
		t.Errorf("the save was reported in two parts: %v", got)
	}

	touch(t, path, "")
	start := time.Now()
	if got := w.Sync(context.Background()).Changes; !slices.Equal(got, []Event{ev(Write, path)}) {
		t.Errorf("an emptied file: %v", got)
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > 2*time.Second {
		t.Errorf("an emptied file was reported after %s, want about Steady", d)
	}
}

// A source that lost events has everything listed again, which finds a
// file that a scan of the stamps alone cannot: here the directory's time
// is put back as it was.
func TestLostEventsRescan(t *testing.T) {
	dir := t.TempDir()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	o := testOptions(t, false)
	w := New(o)
	withFake(w, src)
	var fulls atomic.Int32
	w.onFull = func() { fulls.Add(1) }
	w.Set([]string{dir})
	out := start(t, w)

	x := filepath.Join(dir, "x")
	write(t, x, "x")
	if err := os.Chtimes(dir, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	if got := w.Sync(context.Background()).Changes; len(got) > 0 {
		t.Fatalf("a scan of the stamps found %v; the test cannot hide the file", got)
	}
	src.lost()
	expect(t, out, "after events were lost", ev(Create, x))
	if fulls.Load() != 1 {
		t.Errorf("%d full rescans, want 1", fulls.Load())
	}
}

// A notification about a path in a directory given has the directory
// listed again, even when its time did not move; one about an ignored
// path does not.
func TestNotifiedPathRelists(t *testing.T) {
	dir := t.TempDir()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	o := testOptions(t, false)
	o.Ignore = NewIgnore(IgnoreConfig{Patterns: DefaultPatterns}).Match
	w := New(o)
	withFake(w, src)
	w.Set([]string{dir})
	out := start(t, w)
	if got := <-src.dirs; !slices.Contains(got, dir) {
		t.Errorf("the source watches %v, want %s among them", got, dir)
	}

	x, swp := filepath.Join(dir, "x"), filepath.Join(dir, "x.swp")
	write(t, x, "x")
	write(t, swp, "x")
	os.Chtimes(dir, fi.ModTime(), fi.ModTime())
	src.push(swp)
	none(t, out, "an ignored path notified")
	src.push(x)
	expect(t, out, "a path notified", ev(Create, x))
}

// A source that fails leaves the watcher polling, saying why.
func TestSourceFailureFallsBack(t *testing.T) {
	dir := t.TempDir()
	src := newFakeSource()
	o := testOptions(t, false)
	var mu sync.Mutex
	var logged []string
	o.Logf = func(format string, args ...any) {
		mu.Lock()
		logged = append(logged, fmt.Sprintf(format, args...))
		mu.Unlock()
	}
	w := New(o)
	withFake(w, src)
	w.Set([]string{dir})
	out := start(t, w)
	src.fail(errors.New("out of watches"))
	select {
	case <-src.closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the failed source was not closed")
	}
	mu.Lock()
	if len(logged) != 1 || !strings.Contains(logged[0], "out of watches") || !strings.Contains(logged[0], "every") {
		t.Errorf("logged %q", logged)
	}
	mu.Unlock()
	x := filepath.Join(dir, "x")
	write(t, x, "x")
	expect(t, out, "a change found by polling", ev(Create, x))
}

func TestQueueOverflow(t *testing.T) {
	q := newQueue()
	for i := range queueCap + 1 {
		q.push(fmt.Sprint(i))
	}
	paths, all, err := q.take()
	if len(paths) != queueCap || !all || err != nil {
		t.Errorf("take = %d paths, all %v, err %v", len(paths), all, err)
	}
	if paths, all, _ := q.take(); len(paths) != 0 || all {
		t.Error("take did not empty the queue")
	}
}

// Run, when its context is done, leaves no goroutine behind: not its
// own, not the source's, not the fingerprinting's.
func TestNoLeaks(t *testing.T) {
	forBoth(t, func(t *testing.T, poll bool) {
		dir := t.TempDir()
		mkfiles(t, dir, map[string]string{"a.src": "a", "sub/b.src": "b"})
		before := runtime.NumGoroutine()
		reported := time.NewTimer(5 * time.Second)
		defer reported.Stop()
		for range 3 {
			o := testOptions(t, poll)
			o.Recursive = true
			o.Content = commentBlind
			w := New(o)
			w.Set([]string{dir})
			ctx, cancel := context.WithCancel(context.Background())
			out := make(chan Batch)
			done := make(chan struct{})
			go func() {
				w.Run(ctx, out)
				close(done)
			}()
			time.Sleep(50 * time.Millisecond)
			write(t, filepath.Join(dir, "sub", "b.src"), "b2")
			reported.Reset(5 * time.Second)
			select {
			case <-out:
			case <-reported.C:
				t.Fatal("nothing reported")
			}
			w.Set([]string{dir, filepath.Join(dir, "c.src")})
			cancel()
			<-done
		}
		deadline := time.Now().Add(2 * time.Second)
		for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if n := runtime.NumGoroutine(); n > before {
			buf := make([]byte, 1<<16)
			t.Errorf("%d goroutines after, %d before:\n%s", n, before, buf[:runtime.Stack(buf, true)])
		}
	})
}

// A large tree dropped from the set leaves maps made again at the size of
// what is left, which still reports its changes.
func TestDroppedTreeCompacts(t *testing.T) {
	big, small := t.TempDir(), t.TempDir()
	files := map[string]string{}
	for i := range compactBelow + 100 {
		files[fmt.Sprintf("d%d/f%d", i%50, i)] = "x"
	}
	mkfiles(t, big, files)
	a := filepath.Join(small, "a")
	touch(t, a, "a")
	w := New(Options{NoNotify: true, Recursive: true})
	w.Set([]string{big, small})
	if w.peak < compactBelow {
		t.Fatalf("peak %d after watching %d files", w.peak, len(files))
	}
	w.Sync(context.Background())
	w.Set([]string{small})
	if n := watched(w); n != 2 || w.peak != 2 {
		t.Errorf("after the tree was dropped: %d paths, peak %d; want 2, 2", n, w.peak)
	}
	touch(t, a, "a2")
	if got := w.Sync(context.Background()).Changes; !slices.Equal(got, []Event{ev(Write, a)}) {
		t.Errorf("after compacting: %v, want a written", got)
	}
}

// A scan with nothing changed allocates nothing per path watched.
func TestQuietScanAllocatesLittle(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{}
	for i := range 2000 {
		files[fmt.Sprintf("d%d/f%d", i%20, i)] = "x"
	}
	mkfiles(t, dir, files)
	w := New(Options{NoNotify: true, Recursive: true})
	w.Set([]string{dir})
	w.check(false, nil)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	before := ms.TotalAlloc
	w.check(false, nil)
	runtime.ReadMemStats(&ms)
	// os.Stat's own garbage, some 200 bytes a path, is not the scan's;
	// a map of the stamps alone was more than that again.
	if per := (ms.TotalAlloc - before) / uint64(watched(w)); per > 400 {
		t.Errorf("a quiet scan allocated %d bytes a path", per)
	}
}

// Concurrent Sync calls without Run, and Sync racing with Run exit, never deadlock.
func TestSyncConcurrentAndRunExit(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	touch(t, a, "a")
	w := New(Options{NoNotify: true, Poll: 20 * time.Millisecond})
	w.Set([]string{a})

	// Two concurrent Sync calls without Run.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = w.Sync(ctx)
		})
	}
	wg.Wait()

	// Sync racing with Run starting and exiting repeatedly.
	deadlock := time.NewTimer(3 * time.Second)
	defer deadlock.Stop()
	for range 20 {
		ctx, cancel := context.WithCancel(context.Background())
		out := make(chan Batch, 1)
		runDone := make(chan struct{})
		go func() {
			w.Run(ctx, out)
			close(runDone)
		}()
		syncDone := make(chan struct{})
		go func() {
			sctx, scancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer scancel()
			_ = w.Sync(sctx)
			close(syncDone)
		}()
		cancel()
		deadlock.Reset(3 * time.Second)
		select {
		case <-syncDone:
		case <-deadlock.C:
			t.Fatal("Sync deadlocked while Run exited")
		}
		<-runDone
	}
}

// Sync while Run is active drains pending OS notifications before scanning.
func TestSyncDrainsPendingNotifications(t *testing.T) {
	dir := t.TempDir()
	fi, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	src := newFakeSource()
	o := testOptions(t, false)
	o.Settle = 5 * time.Second // keep settle timer pending when Sync runs
	w := New(o)
	withFake(w, src)
	w.Set([]string{dir})
	_ = start(t, w)
	<-src.dirs

	x := filepath.Join(dir, "x")
	write(t, x, "x")
	if err := os.Chtimes(dir, fi.ModTime(), fi.ModTime()); err != nil {
		t.Fatal(err)
	}
	src.push(x)
	// Without draining src.take(), dir's unchanged mtime hides x from Sync.
	got := w.Sync(context.Background()).Changes
	if !slices.Equal(got, []Event{ev(Create, x)}) {
		t.Fatalf("Sync got %v, want %v", got, []Event{ev(Create, x)})
	}
}

// Concurrent Set with fingerprinting racing with Run exit never panics on WaitGroup.
func TestFingerprintRunExitNoRace(t *testing.T) {
	dir := t.TempDir()
	files := make([]string, 8)
	for i := range files {
		files[i] = filepath.Join(dir, fmt.Sprintf("f%d.src", i))
		touch(t, files[i], "one\n")
	}
	w := New(Options{NoNotify: true, Poll: 10 * time.Millisecond, Content: commentBlind})
	for range 15 {
		ctx, cancel := context.WithCancel(context.Background())
		out := make(chan Batch, 4)
		done := make(chan struct{})
		go func() {
			w.Run(ctx, out)
			close(done)
		}()
		var wg sync.WaitGroup
		wg.Go(func() {
			for i := range files {
				w.Set(files[:i+1])
			}
		})
		cancel()
		<-done
		wg.Wait()
	}
}

type blockingWatchSource struct {
	*fakeSource
	calls   int
	inWatch chan struct{}
	release chan struct{}
}

func (b *blockingWatchSource) watch(dirs []string) error {
	b.calls++
	if b.calls == 2 {
		close(b.inWatch)
		<-b.release
	}
	return b.fakeSource.watch(dirs)
}

// A change occurring while Set is reconfiguring OS watches is caught by the
// post-rewatch scan without waiting for the safety-net rescan.
func TestSetRewatchCatchesConcurrentChange(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main.go")
	write(t, main, "package main\n")
	src := &blockingWatchSource{
		fakeSource: newFakeSource(),
		inWatch:    make(chan struct{}),
		release:    make(chan struct{}),
	}
	o := testOptions(t, false)
	w := New(o)
	w.newSource = func(bool, func(string) bool) (source, error) { return src, nil }
	w.Set([]string{dir})
	out := start(t, w)
	<-src.dirs

	// Switch to watching only the file, wait until Run is mid-rewatch, then
	// replace the file without emitting any OS notification from the source.
	w.Set([]string{main})
	<-src.inWatch
	tmp := filepath.Join(dir, "save.tmp")
	write(t, tmp, "package main // given\n")
	rename(t, tmp, main)
	close(src.release)
	<-src.dirs
	expect(t, out, "change during Set rewatch", ev(Write, main))
}

// Duplicate Remove deltas for the same path produce a single Remove event.
func TestJudgeDeduplicatesRemoves(t *testing.T) {
	w := New(Options{NoNotify: true, Steady: -1})
	st := stamp{exist: true, size: 4}
	evs := w.judge([]delta{
		{path: "/tmp/a", old: st, new: stamp{}},
		{path: "/tmp/a", old: st, new: stamp{}},
		{path: "/tmp/b", old: stamp{}, new: stamp{}}, // create+drop cancelled
	})
	want := []Event{{Path: "/tmp/a", Op: Remove}}
	if !slices.Equal(evs, want) {
		t.Fatalf("judge = %v, want %v", evs, want)
	}
}
