package fswatch

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// Op is what happened to a path.
type Op uint8

const (
	// Create is a path that did not exist and now does.
	Create Op = iota + 1
	// Write is a file whose content, size or modification time changed,
	// including one replaced by another renamed over it, or deleted and
	// made again.
	Write
	// Remove is a path that existed and now does not.
	Remove
)

// String returns the operation's name, such as "write".
func (op Op) String() string {
	switch op {
	case Create:
		return "create"
	case Write:
		return "write"
	case Remove:
		return "remove"
	}
	return fmt.Sprintf("Op(%d)", uint8(op))
}

// Event is a change to one path.
type Event struct {
	// Path is the path as the watcher holds it: cleaned, below the path
	// given that it was found through.
	Path string
	// Op is what happened to it. A rename is a Remove of the old name and
	// a Create of the new one.
	Op Op
	// Dir reports whether the path is a directory, or for a Remove was
	// one.
	Dir bool
	// Same reports, for a Write, that [Options.Content] judged the file
	// the same as before: it was written, but nothing that matters
	// changed. Drop such events with [Options.Filter] if they are of no
	// interest.
	Same bool
}

// String returns the event as "write /path".
func (e Event) String() string {
	s := e.Op.String() + " " + e.Path
	if e.Same {
		s += " (same)"
	}
	return s
}

// Batch is the changes found close together, sorted by path, each path
// once.
type Batch struct {
	Changes []Event
	// At is when the first of them was found.
	At time.Time
}

// Paths returns the paths of b's changes.
func (b Batch) Paths() []string {
	out := make([]string, len(b.Changes))
	for i, e := range b.Changes {
		out[i] = e.Path
	}
	return out
}

// merge adds o to b: a path in both takes the outcome of the two changes
// in turn.
func (b *Batch) merge(o Batch) {
	if len(o.Changes) == 0 {
		return
	}
	if b.At.IsZero() || !o.At.IsZero() && o.At.Before(b.At) {
		b.At = o.At
	}
	byPath := make(map[string]int, len(b.Changes))
	for i, e := range b.Changes {
		byPath[e.Path] = i
	}
	drop := map[int]bool{}
	for _, e := range o.Changes {
		i, ok := byPath[e.Path]
		if !ok || drop[i] {
			byPath[e.Path] = len(b.Changes)
			b.Changes = append(b.Changes, e)
			continue
		}
		if m, keep := then(b.Changes[i], e); keep {
			b.Changes[i] = m
		} else {
			drop[i] = true
		}
	}
	out := b.Changes[:0]
	for i, e := range b.Changes {
		if !drop[i] {
			out = append(out, e)
		}
	}
	b.Changes = out
	sortEvents(b.Changes)
}

// then returns what a followed by b amounts to, or false for nothing: a
// path made and removed again before anyone was told of it.
func then(a, b Event) (Event, bool) {
	switch {
	case a.Op == Create && b.Op == Remove:
		return Event{}, false
	case a.Op == Remove && b.Op != Remove:
		b.Op, b.Same = Write, false
		return b, true
	case a.Op == Create:
		b.Op, b.Same = Create, false
		return b, true
	case b.Op == Remove:
		return b, true
	}
	b.Op, b.Same = Write, a.Same && b.Same
	return b, true
}

func sortEvents(es []Event) {
	slices.SortFunc(es, func(a, b Event) int { return strings.Compare(a.Path, b.Path) })
}

// Content lets the caller judge what an edit to a file changed. It is
// asked only about the files Match picks, so that the watcher reads no
// others.
type Content struct {
	// Match reports whether the file at path is to be judged. Nil judges
	// none.
	Match func(path string) bool
	// Sum returns what of data, the file's content, matters: two versions
	// whose sums are equal are the same, and an edit from one to the other
	// is reported with [Event.Same] set. It may return data itself.
	Sum func(path string, data []byte) []byte
}

func (c Content) matches(path string) bool {
	return c.Match != nil && c.Sum != nil && c.Match(path)
}

// The defaults of [Options].
const (
	// DefaultSettle is how long the watcher gathers what the system
	// reports before it scans: a program's save is several events a few
	// milliseconds apart.
	DefaultSettle = 15 * time.Millisecond
	// DefaultSteady is how long a file found empty or still changing is
	// looked at again before it is reported as it is.
	DefaultSteady = 50 * time.Millisecond
	// DefaultPoll is the time between scans without notifications.
	DefaultPoll = 250 * time.Millisecond
	// DefaultRescan is the time between scans with them.
	DefaultRescan = 2 * time.Second
)

// Options configure a [Watcher]. The zero value is ready to use.
type Options struct {
	// Recursive has a directory in the set stand for everything below
	// it, not only its entries. What Ignore ignores, and everything below
	// an ignored directory, is left out.
	Recursive bool
	// Ignore reports whether a path found in a directory of the set is
	// to be left out, with all below it. It is never asked about a path
	// given to [Watcher.Add] or [Watcher.Set], which is watched whatever
	// it says. It is called from more than one goroutine. [Ignore.Match]
	// is one. Nil leaves nothing out.
	Ignore func(path string, isDir bool) bool
	// Filter, when set, is asked about each event before it is reported;
	// false drops it.
	Filter func(Event) bool
	// Content, when its fields are set, judges edits to the files it
	// picks; see [Content].
	Content Content
	// Settle is how long the watcher gathers what the system reports
	// before it scans, so that the steps of one save are one scan and one
	// batch. Zero is [DefaultSettle].
	Settle time.Duration
	// Steady is how long a file found empty, or changing while it is
	// looked at, is looked at again before it is reported as it is: a
	// save may be caught between truncating a file and writing it. Zero is
	// [DefaultSteady]; a negative duration reports at once.
	Steady time.Duration
	// Poll is the time between scans when there are no notifications.
	// Zero is [DefaultPoll].
	Poll time.Duration
	// Rescan is the time between scans when there are, for a
	// notification that never came. Zero is [DefaultRescan].
	Rescan time.Duration
	// NoNotify polls instead of asking the operating system, for a file
	// system whose notifications cannot be trusted, such as a network
	// mount, and for tests.
	NoNotify bool
	// Logf, when set, is told what a person should know, such as the
	// watcher falling back to polling and why.
	Logf func(format string, args ...any)
	// Tracef, when set, is told each decision the watcher makes.
	Tracef func(format string, args ...any)
}

func (o *Options) defaults() {
	if o.Settle <= 0 {
		o.Settle = DefaultSettle
	}
	if o.Steady == 0 {
		o.Steady = DefaultSteady
	}
	if o.Poll <= 0 {
		o.Poll = DefaultPoll
	}
	if o.Rescan <= 0 {
		o.Rescan = DefaultRescan
	}
}

// Watcher watches a set of paths. A file in the set reports its own
// changes; a directory, the entries that are added to it, written or
// removed from it, and with [Options.Recursive], all below it. Its
// methods may be called from any goroutine.
type Watcher struct {
	o Options
	// newSource opens the system's source; tests replace it.
	newSource func(recursive bool, skip func(dir string) bool) (source, error)

	syncs   chan chan Batch // Sync's requests to Run
	rescans chan struct{}   // Rescan's
	changed chan struct{}   // the set changed shape: Add, Remove, Set
	run     sync.Mutex      // held while Run runs
	runMu   sync.Mutex      // guards state and stateCh
	state   runState        // idle, busy (Sync / Run setup-teardown), or running
	stateCh chan struct{}   // closed on any state transition out of non-idle
	bgMu    sync.Mutex      // serializes bg.Add against bg.Wait
	bg      sync.WaitGroup  // fingerprinting

	mu    sync.Mutex
	roots map[string]bool  // the paths given
	files map[string]stamp // every path watched: the roots, and the entries found
	// entries maps each directory whose entries are watched to them,
	// each with whether it is a directory (not a link to one), which a
	// recursive watch lists in turn.
	entries map[string]map[string]bool
	sums    map[string]fileSum
	shape   int // counts changes to roots and entries' keys
	peak    int // the most paths files held since its maps were made
	// scanned is check's slice of the paths and stamps it stats, kept
	// for the next check; only the one goroutine checking uses it.
	scanned []seenStamp
	// onFull, when set, is called as a full rescan starts; for tests.
	onFull func()
}

// fileSum is a file's Content sum and the stamp of the version it was
// taken of.
type fileSum struct {
	st  stamp
	sum []byte
}

// New returns a watcher of nothing yet; [Watcher.Add] gives it paths.
func New(opts Options) *Watcher {
	opts.defaults()
	return &Watcher{
		o:         opts,
		newSource: newSource,
		syncs:     make(chan chan Batch),
		rescans:   make(chan struct{}, 1),
		changed:   make(chan struct{}, 1),
		roots:     map[string]bool{},
		files:     map[string]stamp{},
		entries:   map[string]map[string]bool{},
		sums:      map[string]fileSum{},
	}
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

func (w *Watcher) ignored(path string, isDir bool) bool {
	return w.o.Ignore != nil && w.o.Ignore(path, isDir)
}

// Add watches paths as well. Each is stamped as it is now, a directory's
// entries with it, and is not reported for existing; a path that does
// not exist yet is reported when it appears.
func (w *Watcher) Add(paths ...string) {
	var stamped []string
	w.mu.Lock()
	for _, p := range paths {
		p = filepath.Clean(p)
		if !w.roots[p] {
			w.roots[p] = true
			w.shape++
		}
		if _, ok := w.files[p]; !ok {
			w.files[p] = stat(p)
			stamped = append(stamped, p)
		}
	}
	found, _ := w.reconcile()
	w.mu.Unlock()
	w.fingerprint(append(stamped, found...))
	w.reshaped()
}

// Remove stops watching paths, and what was watched only for being
// found below them.
func (w *Watcher) Remove(paths ...string) {
	w.mu.Lock()
	for _, p := range paths {
		p = filepath.Clean(p)
		if w.roots[p] {
			delete(w.roots, p)
			w.shape++
		}
	}
	w.reconcile()
	w.mu.Unlock()
	w.reshaped()
}

// Set replaces the paths watched and returns those new to them, sorted.
// A path watched already, given or found, keeps the stamp it had, so a
// change made between two calls is still reported; a path new to the
// watcher is stamped as it is now.
func (w *Watcher) Set(paths []string) (added []string) {
	want := make(map[string]bool, len(paths))
	for _, p := range paths {
		want[filepath.Clean(p)] = true
	}
	w.mu.Lock()
	for p := range w.roots {
		if !want[p] {
			delete(w.roots, p)
			w.shape++
		}
	}
	for p := range want {
		if w.roots[p] {
			continue
		}
		w.roots[p] = true
		w.shape++
		added = append(added, p)
		if _, ok := w.files[p]; !ok {
			w.files[p] = stat(p)
		}
	}
	found, _ := w.reconcile()
	w.mu.Unlock()
	w.fingerprint(append(found, added...))
	w.reshaped()
	slices.Sort(added)
	return added
}

// Len returns the number of paths watched: those given, and those found
// in the directories among them.
func (w *Watcher) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.files)
}

// reshaped tells Run that the directories to watch may have changed.
func (w *Watcher) reshaped() {
	select {
	case w.changed <- struct{}{}:
	default:
	}
}

// Rescan has Run list every directory in the set again and stat every
// path, as it does when the system says it lost events: for a change
// that nothing tells of, such as a file system mounted over a directory.
// It does not wait; asked twice before Run has rescanned, it rescans
// once.
func (w *Watcher) Rescan() {
	select {
	case w.rescans <- struct{}{}:
	default:
	}
}

type runState uint8

const (
	stateIdle runState = iota
	stateBusy
	stateRunning
)

func (w *Watcher) setState(st runState) {
	w.runMu.Lock()
	old := w.stateCh
	w.state = st
	if st == stateIdle {
		w.stateCh = nil
	} else {
		w.stateCh = make(chan struct{})
	}
	w.runMu.Unlock()
	if old != nil {
		close(old)
	}
}

func (w *Watcher) waitBG() {
	w.bgMu.Lock()
	w.bg.Wait()
	w.bgMu.Unlock()
}

// Sync scans now and returns what it found with whatever was waiting to
// be sent: what Run would send next. It is for the moment before acting
// on the files, to make sure nothing changed that a notification has not
// told of yet. While Run runs, Run does the scan, and Sync returns an
// empty batch if ctx is done first; otherwise Sync scans itself.
func (w *Watcher) Sync(ctx context.Context) Batch {
	for {
		w.runMu.Lock()
		switch w.state {
		case stateIdle:
			w.state = stateBusy
			w.stateCh = make(chan struct{})
			w.runMu.Unlock()
			defer w.setState(stateIdle)
			return w.check(false, nil)
		case stateBusy:
			ch := w.stateCh
			w.runMu.Unlock()
			select {
			case <-ch:
			case <-ctx.Done():
				return Batch{}
			}
		case stateRunning:
			ch := w.stateCh
			w.runMu.Unlock()
			reply := make(chan Batch, 1)
			select {
			case w.syncs <- reply:
			case <-ch:
				continue
			case <-ctx.Done():
				return Batch{}
			}
			select {
			case b := <-reply:
				return b
			case <-ctx.Done():
				return Batch{}
			}
		}
	}
}

// Run watches until ctx is done and sends each batch of changes on out.
// Changes found while nobody receives join the batch waiting to be sent.
// It returns once every goroutine and system handle it started is gone.
// Only one Run may run at a time.
func (w *Watcher) Run(ctx context.Context, out chan<- Batch) {
	w.run.Lock()
	defer w.run.Unlock()
	for {
		w.runMu.Lock()
		if w.state == stateIdle {
			w.state = stateBusy
			w.stateCh = make(chan struct{})
			w.runMu.Unlock()
			break
		}
		ch := w.stateCh
		w.runMu.Unlock()
		<-ch
	}
	defer w.setState(stateIdle)
	defer w.waitBG()

	tick := time.NewTicker(w.o.Poll)
	defer tick.Stop()
	src := w.openSource()
	defer func() {
		if src != nil {
			src.close()
		}
	}()
	if src != nil {
		tick.Reset(w.o.Rescan)
	}
	shape := -1
	rewatch := func() bool {
		w.mu.Lock()
		s := w.shape
		var dirs []string
		if s != shape && src != nil {
			dirs = w.dirs()
		}
		w.mu.Unlock()
		if s == shape {
			return false
		}
		shape = s
		if src != nil {
			if err := src.watch(dirs); err != nil {
				src = w.fallBack(src, err, tick)
			}
		}
		return true
	}
	rewatch()

	var (
		outbox *Batch
		settle <-chan time.Time
		timer  *time.Timer
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	check := func(full bool, relist map[string]bool) {
		if b := w.check(full, relist); len(b.Changes) > 0 {
			if outbox == nil {
				outbox = &b
			} else {
				outbox.merge(b)
			}
		}
		rewatch()
	}
	w.setState(stateRunning)
	defer w.setState(stateBusy)
	for {
		var send chan<- Batch
		var b Batch
		if outbox != nil {
			send, b = out, *outbox
		}
		var wake <-chan struct{}
		if src != nil {
			wake = src.wake()
		}
		select {
		case <-ctx.Done():
			return
		case send <- b:
			outbox = nil
		case <-wake:
			if settle == nil {
				timer = time.NewTimer(w.o.Settle)
				settle = timer.C
			}
		case <-settle:
			settle, timer = nil, nil
			if src == nil {
				continue
			}
			paths, all, err := src.take()
			if err != nil {
				src = w.fallBack(src, err, tick)
				check(true, nil)
				continue
			}
			if all {
				w.tracef("the system lost events: rescanning everything")
				check(true, nil)
				continue
			}
			scan, relist := w.relevant(paths)
			w.tracef("notified of %d paths: scan %v", len(paths), scan)
			if scan {
				check(false, relist)
			}
		case <-tick.C:
			check(false, nil)
		case <-w.rescans:
			w.tracef("asked to rescan everything")
			check(true, nil)
		case <-w.changed:
			if rewatch() {
				// Scan once after updating OS watches to close the window
				// between Add/Set stamping paths and the new watches being live.
				check(false, nil)
			}
		case reply := <-w.syncs:
			if timer != nil {
				timer.Stop()
				settle, timer = nil, nil
			}
			var full bool
			var relist map[string]bool
			if src != nil {
				select {
				case <-src.wake():
				default:
				}
				paths, all, err := src.take()
				if err != nil {
					src = w.fallBack(src, err, tick)
					full = true
				} else if all {
					w.tracef("the system lost events: rescanning everything")
					full = true
				} else if len(paths) > 0 {
					_, relist = w.relevant(paths)
				}
			}
			check(full, relist)
			var b Batch
			if outbox != nil {
				b, outbox = *outbox, nil
			}
			reply <- b
		}
	}
}

// openSource returns the system's source watching the set's directories,
// or nil to poll, saying why when it matters.
func (w *Watcher) openSource() source {
	if w.o.NoNotify {
		w.tracef("polling every %s, as asked", w.o.Poll)
		return nil
	}
	src, err := w.newSource(w.o.Recursive, func(dir string) bool { return w.ignored(dir, true) })
	if err != nil {
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

// dirs returns the directories whose entries are of interest to a
// source: each directory whose entries are watched, and the directory of
// each path given, where it would appear or go. w.mu is held.
func (w *Watcher) dirs() []string {
	set := make(map[string]bool, len(w.entries)+len(w.roots))
	for d := range w.entries {
		set[d] = true
	}
	for r := range w.roots {
		set[filepath.Dir(r)] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	slices.Sort(out)
	return out
}

// relevant reports whether paths, as a source reported them, are worth a
// scan, and which directories are to be listed again: one is watched, or
// is not ignored and is in a directory whose entries are.
func (w *Watcher) relevant(paths []string) (scan bool, relist map[string]bool) {
	relist = map[string]bool{}
	var maybe []string
	w.mu.Lock()
	for _, p := range paths {
		p = filepath.Clean(p)
		if _, ok := w.files[p]; ok {
			scan = true
			if _, listed := w.entries[p]; listed {
				relist[p] = true
			}
			continue
		}
		if _, listed := w.entries[filepath.Dir(p)]; listed {
			maybe = append(maybe, p)
		}
	}
	w.mu.Unlock()
	for _, p := range maybe {
		if !w.ignored(p, isDir(p)) {
			scan = true
			relist[filepath.Dir(p)] = true
		}
	}
	return scan, relist
}

// short names up to three paths, for a trace.
func short(paths []string) string {
	if len(paths) <= 3 {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:3], ", ") + ", ..."
}
