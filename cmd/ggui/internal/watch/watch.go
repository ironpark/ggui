// Package watch reports changes to a set of files by polling their
// modification times.
//
// Polling is chosen over the operating system's notifications on purpose.
// The set is what one program is built from, a few hundred to a few
// thousand files, and a stat costs about a microsecond, so a scan every
// few hundred milliseconds is noise. In exchange it needs no dependency
// and no per-platform code, holds no descriptor per file (kqueue on macOS
// needs one each and runs into the open-file limit on a large module), and
// is indifferent to how an editor saves: a write in place, a rename over
// the old file and a delete followed by a create all leave a different
// modification time, size or existence behind, where an event-based
// watcher must re-arm itself on the new inode or miss the next save. The
// cost is latency, at most one interval, which the debounce that follows
// any change dwarfs anyway.
package watch

import (
	"context"
	"io/fs"
	"os"
	"slices"
	"sync"
	"time"
)

// stamp is what a scan compares: a file that differs in any of these has
// changed.
type stamp struct {
	mod   time.Time
	size  int64
	exist bool
}

func stat(path string) stamp {
	fi, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	s := stamp{mod: fi.ModTime(), exist: true}
	// A directory's size is noise on some file systems; its modification
	// time is what a file being added or removed changes.
	if fi.Mode()&fs.ModeType == 0 {
		s.size = fi.Size()
	}
	return s
}

// Watcher polls a set of paths. Directories in the set report files added
// to or removed from them, not changes inside those files.
type Watcher struct {
	// Interval is the time between scans.
	Interval time.Duration
	// Quiet is how long the set must go unchanged after a change before
	// the change is reported, so that saving several files, or an editor
	// writing one in steps, is one rebuild rather than several.
	Quiet time.Duration

	mu    sync.Mutex
	files map[string]stamp
}

// New returns a watcher for paths, stamped as they are now.
func New(paths []string) *Watcher {
	w := &Watcher{Interval: 250 * time.Millisecond, Quiet: 200 * time.Millisecond}
	w.Set(paths)
	return w
}

// Set replaces the watched set. A path already watched keeps the stamp it
// had, so a change made between two calls is still reported; a path new
// to the set is stamped as it is now.
func (w *Watcher) Set(paths []string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	files := make(map[string]stamp, len(paths))
	for _, p := range paths {
		if s, ok := w.files[p]; ok {
			files[p] = s
		} else {
			files[p] = stat(p)
		}
	}
	w.files = files
}

// Len returns the number of paths watched.
func (w *Watcher) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.files)
}

// Scan stats every path and returns those that changed since the last
// scan, sorted.
func (w *Watcher) Scan() []string {
	w.mu.Lock()
	paths := make([]string, 0, len(w.files))
	for p := range w.files {
		paths = append(paths, p)
	}
	w.mu.Unlock()

	// Stat outside the lock, so Set is never held up by a slow disk.
	now := make(map[string]stamp, len(paths))
	for _, p := range paths {
		now[p] = stat(p)
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	var changed []string
	for p, s := range now {
		old, ok := w.files[p]
		if !ok {
			continue // dropped by a Set while this scan ran
		}
		if old != s {
			changed = append(changed, p)
			w.files[p] = s
		}
	}
	slices.Sort(changed)
	return changed
}

// Run scans until ctx is done and sends each settled batch of changes on
// out. Changes that happen while the receiver is busy, rebuilding, join
// the batch waiting to be sent rather than queueing a batch each, so a
// slow build is followed by one more build, not by one per save.
func (w *Watcher) Run(ctx context.Context, out chan<- []string) {
	tick := time.NewTicker(w.Interval)
	defer tick.Stop()
	pending := map[string]bool{}
	var last time.Time
	for {
		var send chan<- []string
		var batch []string
		if len(pending) > 0 && time.Since(last) >= w.Quiet {
			send = out
			for p := range pending {
				batch = append(batch, p)
			}
			slices.Sort(batch)
		}
		select {
		case <-ctx.Done():
			return
		case send <- batch:
			pending = map[string]bool{}
		case <-tick.C:
			if changed := w.Scan(); len(changed) > 0 {
				for _, p := range changed {
					pending[p] = true
				}
				last = time.Now()
			}
		}
	}
}
