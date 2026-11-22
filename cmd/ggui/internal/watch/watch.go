// Package watch reports changes to the files a program is built from.
//
// The watching itself is fswatch's: a scan of every path in the set is
// the truth, and what the operating system says only wakes it. This
// package is what ggui dev knows about Go on top of it. The set is the
// files and package directories the go command lists for the build. A
// Go file whose tokens and build directives are as they were, an edit to
// comments or layout only, needs no build: fswatch is handed the
// comparison, gosrc.Fingerprint, as its content hook. A package directory
// that gained or lost a file is listed again with the go command, and
// needs a build only if the program now has a file it did not, so that a
// new test file or an editor's temporary file passes in silence.
package watch

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/fswatch"
	"github.com/ironpark/ggui/cmd/ggui/internal/gosrc"
)

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

// PollEnv names the environment variable that, set to "poll", makes the
// watcher poll instead of asking the operating system, for a file system
// whose notifications cannot be trusted, such as a network mount, and for
// tests.
const PollEnv = "GGUI_WATCH"

// Ignore says which paths are not worth listing the sources again for:
// .git, the directories given, such as ggui build's output, and what git
// ignores in a git work tree, or, outside one or without git,
// node_modules. It never takes a path out of the set the app is built
// from: a generated file that git ignores but the app embeds still
// rebuilds it.
type Ignore struct {
	git *fswatch.Ignore
	// names leaves out of a package directory's entries what is never
	// worth a look, without asking git: editors' temporary files, version
	// control, node_modules and the directories given.
	names *fswatch.Ignore
}

// NewIgnore returns the rules, with dirs and all below them ignored.
func NewIgnore(dirs ...string) *Ignore {
	return &Ignore{
		git:   fswatch.NewIgnore(fswatch.IgnoreConfig{Patterns: []string{".git"}, Dirs: dirs, Git: true, Fallback: []string{"node_modules"}}),
		names: fswatch.NewIgnore(fswatch.IgnoreConfig{Patterns: fswatch.DefaultPatterns, Dirs: dirs}),
	}
}

// Reset forgets git's answers, for when a .gitignore changed.
func (ig *Ignore) Reset() {
	if ig != nil {
		ig.git.Reset()
	}
}

// Ignored reports whether path is ignored.
func (ig *Ignore) Ignored(path string) bool {
	if ig == nil {
		return false
	}
	return ig.git.Ignored(path)
}

// Filter reports, for each of paths, whether it is ignored, asking git
// once for each work tree.
func (ig *Ignore) Filter(paths []string) []bool {
	if ig == nil {
		return make([]bool, len(paths))
	}
	return ig.git.Filter(paths)
}

func (ig *Ignore) entry(path string, isDir bool) bool {
	return ig != nil && ig.names.Match(path, isDir)
}

// Options configure a Watcher.
type Options struct {
	// List returns the set again, as the go command lists it now. A
	// change to a directory, or to a file the program is not built from,
	// calls it; so does any real change, since an import may have been
	// added. Nil keeps the set as it is.
	List func(ctx context.Context) ([]string, error)
	// Ignore are the paths whose changes are not worth a listing.
	Ignore *Ignore
	// Settle is how long the watcher gathers what the system reports
	// before it scans, for the steps of one save.
	Settle time.Duration
	// Steady is how long a file found empty or still changing is looked
	// at again before it is judged as it is. Zero uses fswatch's default.
	Steady time.Duration
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
	o  Options
	fs *fswatch.Watcher

	syncs   chan chan Change // Sync's requests to Run
	relists chan struct{}    // Relist's

	mu  sync.Mutex
	set map[string]bool
}

// New returns a watcher for paths, stamped as they are now.
func New(paths []string, o Options) *Watcher {
	w := &Watcher{o: o, syncs: make(chan chan Change), relists: make(chan struct{}, 1)}
	poll := os.Getenv(PollEnv) == "poll"
	if poll {
		w.tracef("polling, as %s asks", PollEnv)
	}
	w.fs = fswatch.New(fswatch.Options{
		Ignore: o.Ignore.entry,
		Content: fswatch.Content{
			Match: isGo,
			Sum: func(_ string, src []byte) []byte {
				s := gosrc.Fingerprint(src)
				return s[:]
			},
		},
		Settle:   o.Settle,
		Steady:   o.Steady,
		Poll:     o.Poll,
		Rescan:   o.SafetyNet,
		NoNotify: poll,
		Logf:     o.Logf,
		Tracef:   o.Tracef,
	})
	w.Set(paths)
	return w
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
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		set[filepath.Clean(p)] = true
	}
	w.mu.Lock()
	w.set = set
	w.mu.Unlock()
	return w.fs.Set(paths)
}

// Len returns the number of paths in the set.
func (w *Watcher) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.set)
}

func isGo(path string) bool { return strings.HasSuffix(path, ".go") }

// isGoSource reports whether path names a Go file a build could read.
func isGoSource(path string) bool { return isGo(path) && !strings.HasSuffix(path, "_test.go") }

// classify sorts what fswatch found: changes that need a build, Go files
// changed in comments or layout only, and directories whose entries
// changed, which take listing the set again to judge. list reports a Go
// file the program is not built from now, but whose edit, as to its build
// constraint, may make it one.
func (w *Watcher) classify(b fswatch.Batch) (real, cosmetic, dirs []string, list bool) {
	var maybe []fswatch.Event
	w.mu.Lock()
	for _, e := range b.Changes {
		if filepath.Base(e.Path) == ".gitignore" {
			w.o.Ignore.Reset()
		}
		in := w.set[e.Path]
		switch {
		case in && e.Dir:
			dirs = append(dirs, e.Path)
		case in && e.Same:
			cosmetic = append(cosmetic, e.Path)
		case in:
			real = append(real, e.Path)
		default:
			maybe = append(maybe, e)
		}
	}
	w.mu.Unlock()
	if len(maybe) == 0 {
		return real, cosmetic, dirs, false
	}
	moved, list := w.outside(maybe)
	dirs = append(dirs, moved...)
	slices.Sort(dirs)
	return real, cosmetic, slices.Compact(dirs), list
}

// outside sorts the changes to paths outside the set: what is ignored
// passes in silence, an edit to a Go file may make it one the program is
// built from, and a file that came or went changed its directory.
func (w *Watcher) outside(events []fswatch.Event) (dirs []string, list bool) {
	paths := make([]string, len(events))
	for i, e := range events {
		paths[i] = e.Path
	}
	ignored := w.o.Ignore.Filter(paths)
	for i, e := range events {
		switch {
		case ignored[i]:
		case e.Op == fswatch.Write:
			list = list || isGoSource(e.Path)
		default:
			dirs = append(dirs, filepath.Dir(e.Path))
		}
	}
	return dirs, list
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
	ctx, cancel := context.WithCancel(ctx)
	batches := make(chan fswatch.Batch)
	fsDone := make(chan struct{})
	go func() {
		w.fs.Run(ctx, batches)
		close(fsDone)
	}()
	defer func() {
		cancel()
		<-fsDone
	}()

	s := runState{w: w, results: make(chan listed, 1)}
	var stop bool
	for !stop {
		var send chan<- Change
		var c Change
		if s.outbox != nil {
			send, c = out, *s.outbox
		}
		select {
		case <-ctx.Done():
			return
		case send <- c:
			s.outbox = nil
		case b := <-batches:
			s.take(ctx, b)
		case <-w.relists:
			w.tracef("asked to list the sources again")
			s.startList(ctx)
		case reply := <-w.syncs:
			reply <- s.scanNow(ctx)
		case r := <-s.results:
			stop = s.listEnded(ctx, r)
		}
	}
}

// runState is what Run keeps between the things it waits on: the change
// waiting to be sent, the change waiting for a listing, and the listings.
type runState struct {
	w         *Watcher
	outbox    *Change
	pend      *pending
	listGen   int  // listings started
	listing   bool // one is running
	listAgain bool // another is wanted when it ends
	results   chan listed
}

// emit puts c in the outbox, merged with what waits there to be sent.
func (s *runState) emit(c Change) {
	if len(c.Real) > 0 {
		// A build is coming, which reads the tree as it is then;
		// whatever waited for a listing is in it.
		s.pend = nil
	}
	if s.outbox == nil {
		s.outbox = &c
	} else {
		s.outbox.merge(c)
	}
}

// startList has the set listed again on a goroutine of its own, which
// sends the outcome on results. Asked while a listing runs, it lists once
// more when that one ends.
func (s *runState) startList(ctx context.Context) {
	list := s.w.o.List
	if list == nil {
		return
	}
	if s.listing {
		s.listAgain = true
		return
	}
	s.listing = true
	s.listGen++
	gen, results := s.listGen, s.results
	go func() {
		paths, err := list(ctx)
		results <- listed{gen, paths, err}
	}()
}

// wait holds a change until a listing that starts after now ends.
func (s *runState) wait(ctx context.Context, dirs, cosmetic []string, at time.Time) {
	next := s.listGen + 1
	if s.pend == nil {
		s.pend = &pending{at: at}
	}
	s.pend.after = next
	s.pend.dirs = union(s.pend.dirs, dirs)
	s.pend.cosmetic = union(s.pend.cosmetic, cosmetic)
	s.startList(ctx)
}

// take sorts what a scan found into a change to send and one that waits
// for the set to be listed again.
func (s *runState) take(ctx context.Context, b fswatch.Batch) {
	if len(b.Changes) == 0 {
		return
	}
	real, cosmetic, dirs, list := s.w.classify(b)
	switch {
	case len(real) > 0:
		s.w.tracef("scan: %d changed (%v), %d in comments or layout only", len(real), short(real), len(cosmetic))
		s.emit(Change{Real: real, Cosmetic: cosmetic, At: b.At})
		s.startList(ctx) // an import may have come or gone
	case len(dirs) > 0 || list:
		s.w.tracef("scan: a directory or a file outside the build changed; listing the sources again")
		s.wait(ctx, dirs, cosmetic, b.At)
	case len(cosmetic) > 0:
		s.w.tracef("scan: %v changed in comments or layout only", short(cosmetic))
		s.emit(Change{Cosmetic: cosmetic, At: b.At})
	}
}

// scanNow has fswatch scan at once and returns what Run would send next,
// which then no longer waits to be sent: Sync's answer.
func (s *runState) scanNow(ctx context.Context) Change {
	s.take(ctx, s.w.fs.Sync(ctx))
	var c Change
	if s.outbox != nil {
		c, s.outbox = *s.outbox, nil
	}
	return c
}

// listEnded takes the set a listing returned, and judges the change that
// waited for a listing started after it was found. It reports whether Run
// stops: a listing fails as ctx ends.
func (s *runState) listEnded(ctx context.Context, r listed) (stop bool) {
	s.listing = false
	if r.err != nil {
		if ctx.Err() != nil {
			return true
		}
		s.w.tracef("listing the sources failed: %v", r.err)
		if s.pend != nil && r.gen >= s.pend.after && len(s.pend.dirs) > 0 {
			// Let the build say what is wrong.
			s.emit(Change{Real: s.pend.dirs, At: s.pend.at})
		}
	} else {
		added := s.w.Set(r.paths)
		if s.pend != nil && r.gen >= s.pend.after {
			switch {
			case len(added) > 0:
				s.w.tracef("listed: %d new to the build (%v)", len(added), short(added))
				s.emit(Change{Real: added, Cosmetic: s.pend.cosmetic, At: s.pend.at})
			case len(s.pend.cosmetic) > 0:
				s.emit(Change{Cosmetic: s.pend.cosmetic, At: s.pend.at})
			default:
				s.w.tracef("listed: nothing new to the build; nothing to do")
			}
			s.pend = nil
		}
	}
	if s.listAgain {
		s.listAgain = false
		s.startList(ctx)
	}
	return false
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

// short names up to three paths, for a trace.
func short(paths []string) string {
	if len(paths) <= 3 {
		return strings.Join(paths, ", ")
	}
	return strings.Join(paths[:3], ", ") + ", ..."
}
