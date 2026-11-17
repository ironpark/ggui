package watch

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// A source is the operating system's word that something below a set of
// directories changed. It only wakes the scan, which decides what changed
// by the files' stamps: a source may report a path twice, report paths
// nobody asked about, or lose events, which it says, and the scan
// still sees the truth. Each platform has one, in notify_*.go; where none
// works the watcher polls.
type source interface {
	// watch replaces the directories whose entries are of interest. A
	// source may watch more, such as the whole tree below each.
	watch(dirs []string) error
	// wake receives a value when take has something to return.
	wake() <-chan struct{}
	// take returns the paths reported since the last take. all reports
	// that events were lost, so that anything may have changed.
	take() (paths []string, all bool)
	close()
}

// PollEnv names the environment variable that, set to "poll", makes the
// watcher poll instead of asking the operating system, for a file system
// whose notifications cannot be trusted, such as a network mount, and for
// tests.
const PollEnv = "GGUI_WATCH"

// errPolling is the reason for polling when PollEnv asks for it.
var errPolling = errors.New(PollEnv + "=poll")

// openSource returns the platform's source, or why there is none. A
// source that watches directories one by one asks skip whether to watch
// one that appeared.
func openSource(skip func(dir string) bool) (source, error) {
	if os.Getenv(PollEnv) == "poll" {
		return nil, errPolling
	}
	return newSource(skip)
}

// queueCap bounds the paths a queue holds between takes; past it the
// queue reports that events were lost, which costs one full scan.
const queueCap = 4096

// queue holds what a source reported until the watcher takes it. The
// platform code pushes from whatever thread it is called on and never
// blocks.
type queue struct {
	mu    sync.Mutex
	paths []string
	all   bool
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

func (q *queue) signal() {
	select {
	case q.ch <- struct{}{}:
	default:
	}
}

func (q *queue) wake() <-chan struct{} { return q.ch }

func (q *queue) take() ([]string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	p, all := q.paths, q.all
	q.paths, q.all = nil, false
	return p, all
}

// roots returns the fewest directories whose trees hold every one of dirs:
// dirs without those below another of them. A recursive source watches
// these; for a module they are its root, which holds its go.mod.
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
