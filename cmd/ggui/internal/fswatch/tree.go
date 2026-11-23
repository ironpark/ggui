package fswatch

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

// A Tree reports that something changed below a set of directories,
// trusting the operating system's notifications alone: it keeps no stamp
// of any path and never scans, so what it costs does not grow with the
// files below, as a recursive [Watcher]'s does, a few hundred bytes a path
// that a home folder of millions makes a gigabyte. It is for a caller
// that reads what changed itself once told, as a source control panel
// runs git status, and only needs to be woken.
//
// Being woken is all it promises: a notification late, doubled or about a
// file nobody cares for wakes it, and one the system loses is reported as
// a change in every directory of the set. Where the system cannot follow
// the set, [Tree.Run] returns [ErrUnwatched], and the caller polls.
//
// macOS and Windows follow a directory's whole tree from the directory
// alone, at no cost per file. Linux's inotify follows one directory at a
// time, so a Tree there watches each directory below the set, but those
// TreeOptions.Ignore leaves out, and gives up past TreeOptions.MaxDirs.
type Tree struct {
	o         TreeOptions
	newSource func(recursive bool, skip func(dir string) bool) (source, error)

	mu      sync.Mutex
	roots   []string
	changed chan struct{} // Set's, to Run
	run     sync.Mutex    // held while Run runs
}

// TreeOptions configure a [Tree].
type TreeOptions struct {
	// Ignore reports whether a path below the set is of no interest:
	// a change to it wakes nobody, and on Linux an ignored directory is
	// not watched, with all below it. [Ignore.Match] is one. It is called
	// from more than one goroutine. Nil leaves nothing out.
	Ignore func(path string, isDir bool) bool
	// Settle is how long the tree gathers what the system reports before
	// it tells of it, so that a build or a checkout writing thousands of
	// files is told of once. Zero is [DefaultTreeSettle].
	Settle time.Duration
	// MaxDirs bounds the directories a Tree watches on Linux, one inotify
	// watch each; past it, Run returns [ErrUnwatched]. Zero is
	// [DefaultTreeMaxDirs]. Elsewhere the system follows a tree whole and
	// it is not counted.
	MaxDirs int
}

// The defaults of [TreeOptions].
const (
	DefaultTreeSettle  = 200 * time.Millisecond
	DefaultTreeMaxDirs = 20000
)

// ErrUnwatched is [Tree.Run]'s error when the system cannot follow the
// set: it has no notifications, it ran out of them, or, on Linux, the set
// has more directories than [TreeOptions.MaxDirs]. The caller then looks
// for changes itself, as by polling.
var ErrUnwatched = errors.New("fswatch: the system cannot follow these directories")

// NewTree returns a tree watcher of nothing yet; [Tree.Set] gives it
// directories.
func NewTree(opts TreeOptions) *Tree {
	if opts.Settle <= 0 {
		opts.Settle = DefaultTreeSettle
	}
	if opts.MaxDirs <= 0 {
		opts.MaxDirs = DefaultTreeMaxDirs
	}
	return &Tree{o: opts, newSource: newSource, changed: make(chan struct{}, 1)}
}

// Set makes dirs the directories whose trees are followed. It may be
// called from any goroutine, before Run or while it runs.
func (t *Tree) Set(dirs []string) {
	rs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		if d != "" {
			rs = append(rs, filepath.Clean(d))
		}
	}
	slices.Sort(rs)
	rs = slices.Compact(rs)
	t.mu.Lock()
	t.roots = rs
	t.mu.Unlock()
	select {
	case t.changed <- struct{}{}:
	default:
	}
}

// Run follows the trees until ctx is done, sending on out, each time
// something changed, the directories of the set it changed below, as Set
// gave them. Changes made while nobody receives join those waiting to be
// sent. It returns nil once ctx is done, or an error wrapping
// [ErrUnwatched] when the system cannot follow the set, having closed
// what it opened. Only one Run may run at a time.
func (t *Tree) Run(ctx context.Context, out chan<- []string) error {
	t.run.Lock()
	defer t.run.Unlock()
	src, err := t.newSource(true, func(dir string) bool { return t.ignored(dir, true) })
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnwatched, err)
	}
	defer src.close()

	var roots []string
	rewatch := func() error {
		t.mu.Lock()
		roots = t.roots
		t.mu.Unlock()
		if ts, ok := src.(treeSource); ok {
			return ts.watchTrees(roots, t.o.MaxDirs)
		}
		return src.watch(roots)
	}
	if err := rewatch(); err != nil {
		return fmt.Errorf("%w: %v", ErrUnwatched, err)
	}

	pending := map[string]bool{}
	var settle <-chan time.Time
	for {
		var send chan<- []string
		var batch []string
		if settle == nil && len(pending) > 0 {
			send, batch = out, slices.Sorted(maps.Keys(pending))
		}
		select {
		case <-ctx.Done():
			return nil
		case send <- batch:
			clear(pending)
		case <-t.changed:
			if err := rewatch(); err != nil {
				return fmt.Errorf("%w: %v", ErrUnwatched, err)
			}
		case <-src.wake():
			if settle == nil {
				settle = time.After(t.o.Settle)
			}
		case <-settle:
			settle = nil
			paths, all, err := src.take()
			switch {
			case err != nil:
				return fmt.Errorf("%w: %v", ErrUnwatched, err)
			case all:
				for _, r := range roots {
					pending[r] = true
				}
			default:
				for _, p := range paths {
					if r, ok := t.rootOf(roots, p); ok {
						pending[r] = true
					}
				}
			}
		}
	}
}

// rootOf is the directory of roots path is below, unless it is ignored,
// or below one ignored, or below none.
func (t *Tree) rootOf(roots []string, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	path = filepath.Clean(path)
	best := ""
	for _, r := range roots {
		if within(r, path) && len(r) > len(best) {
			best = r
		}
	}
	if best == "" {
		return "", false
	}
	if path == best {
		return best, true
	}
	// Each directory between the root and the path, as a recursive
	// Watcher would not have gone into an ignored one.
	rel, err := filepath.Rel(best, path)
	if err != nil {
		return "", false
	}
	p := best
	for el := range strings.SplitSeq(rel, string(filepath.Separator)) {
		p = filepath.Join(p, el)
		if t.ignored(p, p != path) {
			return "", false
		}
	}
	return best, true
}

func (t *Tree) ignored(path string, isDir bool) bool {
	return t.o.Ignore != nil && t.o.Ignore(path, isDir)
}

// A treeSource is a source that follows a directory's tree only by
// watching each directory in it, as inotify does: watchTrees watches
// those below roots, but the ones its skip leaves out, and fails past
// maxDirs.
type treeSource interface {
	watchTrees(roots []string, maxDirs int) error
}
