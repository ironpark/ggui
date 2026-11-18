// Package watch reports changes to the files a program is built from.
//
// The truth is a scan: a stat of every file in the set, a few hundred to a
// few thousand, which costs about a microsecond each and is indifferent to
// how an editor saves. A write in place, a rename over the old file and a
// delete followed by a create all leave a different modification time,
// size or existence behind. What the operating system says about changes,
// FSEvents on macOS, inotify on Linux and ReadDirectoryChangesW on
// Windows, only wakes the scan up, so that a save is seen in milliseconds;
// a slow scan every few seconds catches whatever a notification missed,
// and where there are none the scan runs four times a second instead.
//
// A change found is sorted before it is reported. A Go file whose tokens
// and build directives are as they were, an edit to comments or layout
// only, needs no build; a directory whose entries changed is listed again
// with the go command, and needs a build only if the program now has a
// file it did not, so that a new test file or an editor's temporary file
// passes in silence.
package watch

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gosrc"
	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// stamp is what a scan compares: a file that differs in any of these has
// changed.
type stamp struct {
	mod   time.Time
	size  int64
	exist bool
	dir   bool
}

func stat(path string) stamp {
	fi, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	s := stamp{mod: fi.ModTime(), exist: true, dir: fi.IsDir()}
	// A directory's size is noise on some file systems; its modification
	// time is what a file being added or removed changes.
	if fi.Mode()&fs.ModeType == 0 {
		s.size = fi.Size()
	}
	return s
}

// delta is a path whose stamp changed.
type delta struct {
	path     string
	old, new stamp
}

// fileSum is a Go file's fingerprint and the stamp of the version it was
// taken of.
type fileSum struct {
	st  stamp
	sum gosrc.Sum
}

// Change is what the watcher found.
type Change struct {
	// Real are the paths whose change needs a build: a file of the set
	// changed, appeared or went, or a directory gained a file the program
	// is built from.
	Real []string
	// Cosmetic are Go files whose only change is to comments or layout.
	Cosmetic []string
	// At is when the first of these was found.
	At time.Time
}

func (c *Change) merge(o Change) {
	if c.At.IsZero() || !o.At.IsZero() && o.At.Before(c.At) {
		c.At = o.At
	}
	c.Real = union(c.Real, o.Real)
	c.Cosmetic = union(c.Cosmetic, o.Cosmetic)
	c.Cosmetic = slices.DeleteFunc(c.Cosmetic, func(p string) bool { return slices.Contains(c.Real, p) })
}

func union(a, b []string) []string {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}

// Options configure a Watcher.
type Options struct {
	// List returns the set again, as the go command lists it now. A
	// change to a directory, or to a file the program is not built from,
	// calls it; so does any real change, since an import may have been
	// added. Nil keeps the set as it is.
	List func(ctx context.Context) ([]string, error)
	// Ignore are the paths whose changes do not wake the scan.
	Ignore *Ignore
	// Settle is how long the watcher gathers what the system reports
	// before it scans, for the steps of one save.
	Settle time.Duration
	// Poll is the time between scans when there are no notifications;
	// SafetyNet when there are.
	Poll, SafetyNet time.Duration
	// Logf prints what the developer should know, such as falling back to
	// polling; Tracef, when set, each decision.
	Logf, Tracef func(format string, args ...any)
}

// Watcher watches a set of paths. Directories in the set report files
// added to or removed from them, not changes inside those files.
type Watcher struct {
	o Options

	syncs   chan chan Change // Sync's requests to Run
	relists chan struct{}    // Relist's

	mu    sync.Mutex
	files map[string]stamp
	sums  map[string]fileSum
}

// New returns a watcher for paths, stamped as they are now.
func New(paths []string, o Options) *Watcher {
	w := &Watcher{o: o, sums: map[string]fileSum{}, syncs: make(chan chan Change), relists: make(chan struct{}, 1)}
	w.Set(paths)
	return w
}

func (w *Watcher) logf(format string, args ...any) {
	if w.o.Logf != nil {
		w.o.Logf(format, args...)
	}
}

func (w *Watcher) tracef(format string, args ...any) {
	if w.o.Tracef != nil {
		w.o.Tracef(format, args...)
	}
}

// Set replaces the watched set and returns the paths new to it. A path
// already watched keeps the stamp it had, so a change made between two
// calls is still reported; a path new to the set is stamped as it is now.
func (w *Watcher) Set(paths []string) (added []string) {
	w.mu.Lock()
	files := make(map[string]stamp, len(paths))
	for _, p := range paths {
		p = filepath.Clean(p)
		if s, ok := w.files[p]; ok {
			files[p] = s
		} else {
			files[p] = stat(p)
			added = append(added, p)
		}
	}
	for p := range w.sums {
		if _, ok := files[p]; !ok {
			delete(w.sums, p)
		}
	}
	w.files = files
	w.mu.Unlock()
	slices.Sort(added)

	// The fingerprints a later edit is compared with, taken meanwhile.
	var gofiles []string
	for _, p := range added {
		if isGo(p) {
			gofiles = append(gofiles, p)
		}
	}
	if len(gofiles) > 0 {
		go w.fingerprint(gofiles)
	}
	return added
}

// fingerprint takes the fingerprints of files, each with the stamp of the
// version read, or none for a file that changed while it was read.
func (w *Watcher) fingerprint(files []string) {
	for _, p := range files {
		before := stat(p)
		src, err := shared.ReadFile(p)
		if err != nil || stat(p) != before {
			continue
		}
		sum := gosrc.Fingerprint(src)
		w.mu.Lock()
		_, watched := w.files[p]
		if _, ok := w.sums[p]; watched && !ok {
			w.sums[p] = fileSum{before, sum}
		}
		w.mu.Unlock()
	}
}

func isGo(path string) bool { return strings.HasSuffix(path, ".go") }

// isGoSource reports whether path names a Go file a build could read.
func isGoSource(path string) bool { return isGo(path) && !strings.HasSuffix(path, "_test.go") }

// Len returns the number of paths watched.
func (w *Watcher) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.files)
}

// dirs returns the directories whose entries are of interest: each
// directory of the set, and the directory of each file in it.
func (w *Watcher) dirs() map[string]bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string]bool{}
	for p, s := range w.files {
		if s.dir {
			out[p] = true
		}
		out[filepath.Dir(p)] = true
	}
	return out
}

// Scan stats every path and returns those that changed since the last
// scan, sorted.
func (w *Watcher) Scan() []string {
	var out []string
	for _, d := range w.scan() {
		out = append(out, d.path)
	}
	return out
}

func (w *Watcher) scan() []delta {
	w.mu.Lock()
	paths := slices.Collect(maps.Keys(w.files))
	w.mu.Unlock()

	// Stat outside the lock, so Set is never held up by a slow disk.
	now := make(map[string]stamp, len(paths))
	for _, p := range paths {
		now[p] = stat(p)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	var changed []delta
	for p, s := range now {
		old, ok := w.files[p]
		if !ok {
			continue // dropped by a Set while this scan ran
		}
		if old != s {
			changed = append(changed, delta{p, old, s})
			w.files[p] = s
		}
	}
	slices.SortFunc(changed, func(a, b delta) int { return strings.Compare(a.path, b.path) })
	return changed
}

// classify sorts what a scan found: changes that need a build, Go files
// changed in comments or layout only, and directories whose entries
// changed, which take listing the set again to judge.
func (w *Watcher) classify(ds []delta) (real, cosmetic, dirs []string) {
	for _, d := range ds {
		switch {
		case d.old.dir || d.new.dir:
			dirs = append(dirs, d.path)
		case !isGo(d.path) || !d.old.exist || !d.new.exist:
			real = append(real, d.path)
		case w.sameTokens(d):
			cosmetic = append(cosmetic, d.path)
		default:
			real = append(real, d.path)
		}
	}
	return real, cosmetic, dirs
}

// sameTokens reports whether a Go file that changed has the fingerprint
// it had, and keeps the new one for the next change.
//
// The file may have changed again since the scan, as when the scan caught
// an editor between truncating and writing it. Then what is read is
// judged, and its stamp taken as the one the scan saw, so that the next
// scan does not judge it again.
//
// An empty file is taken as one caught there too, though it held still
// while it was read: a save truncates the file and then writes it, and a
// scan and this read can both fall in between. Judged as it is, the edit
// would be a real one, and so would the next, compared with nothing. A Go
// file that stays empty is a real change, said a moment later.
func (w *Watcher) sameTokens(d delta) bool {
	for range 10 {
		before := stat(d.path)
		src, err := shared.ReadFile(d.path)
		after := stat(d.path)
		if err != nil {
			break
		}
		if after != before || len(src) == 0 {
			time.Sleep(5 * time.Millisecond) // being written
			continue
		}
		sum := gosrc.Fingerprint(src)
		w.mu.Lock()
		old, ok := w.sums[d.path]
		w.sums[d.path] = fileSum{after, sum}
		if _, watched := w.files[d.path]; watched {
			w.files[d.path] = after
		}
		w.mu.Unlock()
		return ok && old.st == d.old && old.sum == sum
	}
	w.mu.Lock()
	delete(w.sums, d.path)
	w.mu.Unlock()
	return false
}

// relevant reports whether paths, as a source reported them, are worth a
// scan: one is in the set, or is not ignored and is in, or is, one of
// dirs. list reports that one is a Go file the program is not built from
// now, but whose edit, as to its build constraint, may make it one.
func (w *Watcher) relevant(paths []string, dirs map[string]bool) (scan, list bool) {
	var maybe []string
	w.mu.Lock()
	for _, p := range paths {
		p = filepath.Clean(p)
		if _, ok := w.files[p]; ok {
			scan = true
			continue
		}
		if filepath.Base(p) == ".gitignore" {
			w.o.Ignore.Reset()
			continue
		}
		if dirs[filepath.Dir(p)] || dirs[p] {
			maybe = append(maybe, p)
		}
	}
	w.mu.Unlock()
	if len(maybe) == 0 {
		return scan, false
	}
	ignored := w.o.Ignore.Filter(maybe)
	for i, p := range maybe {
		if ignored[i] {
			continue
		}
		scan = true
		if isGoSource(p) {
			list = true
		}
	}
	return scan, list
}

// listed is the outcome of a List.
type listed struct {
	gen   int
	paths []string
	err   error
}

// pending is a change that waits for the set to be listed again before it
// is judged: a directory's entries changed, or a Go file that is not in
// the set.
type pending struct {
	after    int      // the first listing that started after it was found
	dirs     []string // the directories that changed
	cosmetic []string
	at       time.Time
}

// Run watches until ctx is done and sends each change found on out.
// Changes found while nobody receives join the one waiting to be sent.
func (w *Watcher) Run(ctx context.Context, out chan<- Change) {
	src := w.openSource()
	defer func() {
		if src != nil {
			src.close()
		}
	}()
	interval := w.o.Poll
	if src != nil {
		interval = w.o.SafetyNet
	}
	tick := time.NewTicker(interval)
	defer tick.Stop()

	var (
		outbox    *Change
		settle    <-chan time.Time
		watched   = w.dirs()
		pend      *pending
		listGen   int  // listings started
		listing   bool // one is running
		listAgain bool // another is wanted when it ends
		results   = make(chan listed, 1)
	)
	emit := func(c Change) {
		if len(c.Real) > 0 {
			// A build is coming, which reads the tree as it is then;
			// whatever waited for a listing is in it.
			pend = nil
		}
		if outbox == nil {
			outbox = &c
		} else {
			outbox.merge(c)
		}
	}
	startList := func() {
		if w.o.List == nil {
			return
		}
		if listing {
			listAgain = true
			return
		}
		listing = true
		listGen++
		gen := listGen
		go func() {
			paths, err := w.o.List(ctx)
			results <- listed{gen, paths, err}
		}()
	}
	// wait holds a change until a listing that starts after now ends.
	wait := func(dirs, cosmetic []string, at time.Time) {
		next := listGen + 1
		if pend == nil {
			pend = &pending{at: at}
		}
		pend.after = next
		pend.dirs = union(pend.dirs, dirs)
		pend.cosmetic = union(pend.cosmetic, cosmetic)
		startList()
	}
	check := func(list bool) {
		at := time.Now()
		ds := w.scan()
		real, cosmetic, changedDirs := w.classify(ds)
		switch {
		case len(real) > 0:
			w.tracef("scan: %d changed (%v), %d in comments or layout only", len(real), short(real), len(cosmetic))
			emit(Change{Real: real, Cosmetic: cosmetic, At: at})
			startList() // an import may have come or gone
		case len(changedDirs) > 0 || list:
			w.tracef("scan: a directory or a file outside the build changed; listing the sources again")
			wait(changedDirs, cosmetic, at)
		case len(cosmetic) > 0:
			w.tracef("scan: %v changed in comments or layout only", short(cosmetic))
			emit(Change{Cosmetic: cosmetic, At: at})
		}
	}

	for {
		var send chan<- Change
		var c Change
		if outbox != nil {
			send, c = out, *outbox
		}
		var wake <-chan struct{}
		if src != nil {
			wake = src.wake()
		}
		select {
		case <-ctx.Done():
			return
		case send <- c:
			outbox = nil
		case <-wake:
			if settle == nil {
				settle = time.After(w.o.Settle)
			}
		case <-settle:
			settle = nil
			paths, all := src.take()
			scan, list := all, false
			if !all {
				scan, list = w.relevant(paths, watched)
			}
			w.tracef("notified of %d paths (lost: %v): scan %v", len(paths), all, scan)
			if scan {
				check(list)
			}
		case <-tick.C:
			check(false)
		case <-w.relists:
			w.tracef("asked to list the sources again")
			startList()
		case reply := <-w.syncs:
			check(false)
			var c Change
			if outbox != nil {
				c, outbox = *outbox, nil
			}
			reply <- c
		case r := <-results:
			listing = false
			if r.err != nil {
				if ctx.Err() != nil {
					return
				}
				w.tracef("listing the sources failed: %v", r.err)
				if pend != nil && r.gen >= pend.after && len(pend.dirs) > 0 {
					// Let the build say what is wrong.
					emit(Change{Real: pend.dirs, At: pend.at})
				}
			} else {
				added := w.Set(r.paths)
				watched = w.dirs()
				if src != nil {
					if err := src.watch(slices.Collect(maps.Keys(watched))); err != nil {
						src = w.fallBack(src, err, tick)
					}
				}
				if pend != nil && r.gen >= pend.after {
					switch {
					case len(added) > 0:
						w.tracef("listed: %d new to the build (%v)", len(added), short(added))
						emit(Change{Real: added, Cosmetic: pend.cosmetic, At: pend.at})
					case len(pend.cosmetic) > 0:
						emit(Change{Cosmetic: pend.cosmetic, At: pend.at})
					default:
						w.tracef("listed: nothing new to the build; nothing to do")
					}
					pend = nil
				}
			}
			if listAgain {
				listAgain = false
				startList()
			}
		}
	}
}

// Sync has Run scan now, and returns what it found with whatever was
// waiting to be sent: what Run would send next. It is for the moment
// before a build replaces the running app, to make sure nothing changed
// that a notification has not told of yet. It returns nothing when ctx is
// done first.
func (w *Watcher) Sync(ctx context.Context) Change {
	reply := make(chan Change, 1)
	select {
	case w.syncs <- reply:
	case <-ctx.Done():
		return Change{}
	}
	select {
	case c := <-reply:
		return c
	case <-ctx.Done():
		return Change{}
	}
}

// Relist has Run list the set again, for a change to what the listing is
// made with, such as the build tags, that no change to a watched file
// tells of. It does not wait; asked twice before Run has listed, it lists
// once.
func (w *Watcher) Relist() {
	select {
	case w.relists <- struct{}{}:
	default:
	}
}

// openSource returns the system's source watching the set's directories,
// or nil to poll, saying why when it matters.
func (w *Watcher) openSource() source {
	skip := func(dir string) bool { return w.o.Ignore.Ignored(dir) }
	src, err := openSource(skip)
	if err != nil {
		if err == errPolling {
			w.tracef("polling, as %s asks", PollEnv)
		} else {
			w.logf("%v; looking for changes every %s instead", err, w.o.Poll)
		}
		return nil
	}
	if err := src.watch(slices.Collect(maps.Keys(w.dirs()))); err != nil {
		src.close()
		w.logf("%v; looking for changes every %s instead", err, w.o.Poll)
		return nil
	}
	return src
}

// fallBack gives up on src after err, for polling.
func (w *Watcher) fallBack(src source, err error, tick *time.Ticker) source {
	src.close()
	w.logf("%v; looking for changes every %s instead", err, w.o.Poll)
	tick.Reset(w.o.Poll)
	return nil
}

// short names up to three paths, for a trace.
func short(paths []string) string {
	if len(paths) <= 3 {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:3], ", ") + ", ..."
}
