package fswatch

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// A source is the operating system's word that something below a set of
// directories changed. It only wakes the scan, which decides what changed
// by the paths' stamps: a source may report a path twice, report paths
// nobody asked about, or lose events, which it says, and the scan still
// sees the truth. Each platform has one, in notify_*.go; where none works
// the watcher polls.
type source interface {
	// watch replaces the directories whose entries are of interest. A
	// source watches each directory once, never each file in it, and may
	// watch more, such as the whole tree below each.
	watch(dirs []string) error
	// wake receives a value when take has something to return.
	wake() <-chan struct{}
	// take returns the paths reported since the last take. all reports
	// that events were lost, so that anything may have changed; err, that
	// the source no longer works and the watcher must poll.
	take() (paths []string, all bool, err error)
	// close releases the source's handles and ends its goroutines before
	// it returns.
	close()
}

// queueCap bounds the paths a queue holds between takes; past it the
// queue reports that events were lost, which costs one full rescan.
const queueCap = 4096

// queue holds what a source reported until the watcher takes it. The
// platform code pushes from whatever thread it is called on and never
// blocks.
type queue struct {
	mu    sync.Mutex
	paths []string
	all   bool
	err   error
	ch    chan struct{}
}

func newQueue() queue { return queue{ch: make(chan struct{}, 1)} }

func (q *queue) push(path string) {
	q.mu.Lock()
	switch {
	case len(q.paths) > 0 && q.paths[len(q.paths)-1] == path:
		// The same file again, as a write in steps reports it.
	case len(q.paths) < queueCap:
		q.paths = append(q.paths, path)
	default:
		q.all = true
	}
	q.mu.Unlock()
	q.signal()
}

// lost says that events were dropped.
func (q *queue) lost() {
	q.mu.Lock()
	q.all = true
	q.mu.Unlock()
	q.signal()
}

// fail says that the source stopped working.
func (q *queue) fail(err error) {
	q.mu.Lock()
	if q.err == nil {
		q.err = err
	}
	q.mu.Unlock()
	q.signal()
}

func (q *queue) signal() {
	select {
	case q.ch <- struct{}{}:
	default:
	}
}

func (q *queue) wake() <-chan struct{} { return q.ch }

func (q *queue) take() ([]string, bool, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	p, all, err := q.paths, q.all, q.err
	q.paths, q.all = nil, false
	return p, all, err
}

// roots returns the fewest directories whose trees hold every one of dirs:
// dirs without those below another of them. A recursive source watches
// these.
func roots(dirs []string) []string {
	s := slices.Clone(dirs)
	for i, d := range s {
		s[i] = filepath.Clean(d)
	}
	slices.Sort(s)
	s = slices.Compact(s)
	var out []string
	for _, d := range s {
		// Sorted, an ancestor comes first, though not always last: "/a/b-c"
		// sorts between "/a/b" and "/a/b/c".
		if !slices.ContainsFunc(out, func(r string) bool { return within(r, d) }) {
			out = append(out, d)
		}
	}
	return out
}

// within reports whether path is dir or below it.
func within(dir, path string) bool {
	if path == dir {
		return true
	}
	if !strings.HasPrefix(path, dir) {
		return false
	}
	return strings.HasSuffix(dir, string(filepath.Separator)) || path[len(dir)] == filepath.Separator
}
