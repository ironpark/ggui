package fswatch

import (
	"bytes"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"time"
)

// stamp is what a scan compares: a path that differs in any of these has
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
	// time is what an entry being added or removed changes.
	if fi.Mode()&fs.ModeType == 0 {
		s.size = fi.Size()
	}
	return s
}

// isDir reports whether path is a directory, not following a link.
func isDir(path string) bool {
	fi, err := os.Lstat(path)
	return err == nil && fi.IsDir()
}

// delta is a path whose stamp changed.
type delta struct {
	path     string
	old, new stamp
}

// readDir returns dir's entries that are watched: those not ignored, and
// those given. w.mu is held.
func (w *Watcher) readDir(dir string) map[string]bool {
	des, _ := os.ReadDir(dir) // what was read before an error, if any
	out := make(map[string]bool, len(des))
	for _, de := range des {
		p := filepath.Join(dir, de.Name())
		d := de.IsDir()
		if !w.roots[p] && w.ignored(p, d) {
			continue
		}
		out[p] = d
	}
	return out
}

// reconcile brings the paths watched in line with the roots and the
// entries of the directories among them: a directory newly to be listed
// is read, and its entries stamped as they are now; what nothing leads to
// any more is dropped. It returns the paths that came, and those that
// went with their last stamps. w.mu is held.
func (w *Watcher) reconcile() (added []string, dropped map[string]stamp) {
	reach := make(map[string]bool, len(w.entries))
	var visit func(dir string)
	visit = func(dir string) {
		if reach[dir] {
			return
		}
		reach[dir] = true
		ents, ok := w.entries[dir]
		if !ok {
			ents = w.readDir(dir)
			w.entries[dir] = ents
			w.shape++
		}
		for e, sub := range ents {
			if _, ok := w.files[e]; !ok {
				w.files[e] = stat(e)
				added = append(added, e)
			}
			if w.o.Recursive && sub {
				visit(e)
			}
		}
	}
	for r := range w.roots {
		if w.files[r].dir {
			visit(r)
		}
	}
	for d := range w.entries {
		if !reach[d] {
			delete(w.entries, d)
			w.shape++
		}
	}
	for p, st := range w.files {
		if w.roots[p] {
			continue
		}
		if ents := w.entries[filepath.Dir(p)]; ents != nil {
			if _, ok := ents[p]; ok {
				continue
			}
		}
		if dropped == nil {
			dropped = map[string]stamp{}
		}
		dropped[p] = st
		delete(w.files, p)
		delete(w.sums, p)
	}
	w.compact()
	return added, dropped
}

// seenStamp is a path and its stamp, as a scan saw it.
type seenStamp struct {
	path string
	st   stamp
}

// compactBelow is the fewest paths a watcher must once have held before
// its maps are made again for having shrunk.
const compactBelow = 4096

// compact makes the maps again, at the size of what they hold, once the
// set has shrunk to a quarter of the most it held: a map never gives
// back the room it grew to, and a large tree watched once and then
// dropped kept hundreds of megabytes of it. w.mu is held.
func (w *Watcher) compact() {
	n := len(w.files)
	w.peak = max(w.peak, n)
	if w.peak < compactBelow || n > w.peak/4 {
		return
	}
	w.files = remake(w.files)
	w.entries = remake(w.entries)
	w.sums = remake(w.sums)
	w.peak = n
}

// remake returns a copy of m, sized to what it holds.
func remake[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	maps.Copy(out, m)
	return out
}

// check scans and returns what changed. full lists every directory
// again; relist names those to list again besides the ones whose stamps
// changed.
func (w *Watcher) check(full bool, relist map[string]bool) Batch {
	at := time.Now()
	w.mu.Lock()
	// The paths and their stamps go in a slice kept from scan to scan: a
	// watch of a large tree stats every path each scan, and a map or a
	// slice made afresh for each was garbage of hundreds of megabytes
	// every few seconds.
	seen := w.scanned[:0]
	if cap(seen) > 2*len(w.files)+64 {
		seen = nil // the set shrank: let the larger go
	}
	for p, st := range w.files {
		seen = append(seen, seenStamp{p, st})
	}
	onFull := w.onFull
	w.mu.Unlock()
	if full && onFull != nil {
		onFull()
	}

	// Stat outside the lock, so Set is never held up by a slow
	// disk, keeping only the paths that changed.
	changed := seen[:0]
	for _, e := range seen {
		if s := stat(e.path); s != e.st {
			changed = append(changed, seenStamp{e.path, s})
		}
	}
	defer func() {
		clear(seen) // hold no paths the set may drop
		w.scanned = seen[:0]
	}()

	w.mu.Lock()
	if relist == nil {
		relist = map[string]bool{}
	}
	reshape := full
	var ds []delta
	for _, e := range changed {
		p, s := e.path, e.st
		old, ok := w.files[p]
		if !ok || old == s {
			continue // unchanged, or dropped while this scan ran
		}
		ds = append(ds, delta{p, old, s})
		w.files[p] = s
		if old.dir != s.dir {
			reshape = true
		}
		if s.dir {
			if _, listed := w.entries[p]; listed {
				relist[p] = true
			}
		}
	}
	if full {
		for d := range w.entries {
			relist[d] = true
		}
	}
	if reshape || len(relist) > 0 {
		for d := range relist {
			if _, listed := w.entries[d]; listed {
				w.entries[d] = w.readDir(d)
			}
		}
		added, dropped := w.reconcile()
		for _, p := range added {
			if st := w.files[p]; st.exist {
				ds = append(ds, delta{p, stamp{}, st})
			}
		}
		if len(dropped) > 0 {
			var byPath map[string]int
			if len(ds) > 0 {
				byPath = make(map[string]int, len(ds))
				for i, d := range ds {
					byPath[d.path] = i
				}
			}
			for p, st := range dropped {
				if !st.exist {
					continue
				}
				if i, ok := byPath[p]; ok {
					ds[i].new = stamp{}
				} else {
					ds = append(ds, delta{p, st, stamp{}})
				}
			}
		}
	}
	w.mu.Unlock()

	b := Batch{Changes: w.judge(ds), At: at}
	if len(b.Changes) > 0 {
		w.tracef("scan: %d changed (%s)", len(b.Changes), short(b.Paths()))
	}
	return b
}

// look is a file being judged: one created or written, which is looked
// at until it holds still.
type look struct {
	ev      Event
	d       delta
	content bool  // Content judges it
	prev    stamp // as last seen
	data    []byte
	done    bool
	steady  bool // it held still: data, if read, is a whole version
}

// steadyStep is the time between looks at a file that does not hold
// still.
const steadyStep = 5 * time.Millisecond

// judge turns what a scan found into events. A file created or written
// is looked at until it holds still, for up to Options.Steady across all
// of them, and its content judged when Options.Content picks it.
func (w *Watcher) judge(ds []delta) []Event {
	var evs []Event
	var looks []*look
	for _, d := range ds {
		ev := Event{Path: d.path}
		switch {
		case !d.old.exist && !d.new.exist:
			continue
		case !d.old.exist && d.new.exist:
			ev.Op, ev.Dir = Create, d.new.dir
		case d.old.exist && !d.new.exist:
			ev.Op, ev.Dir = Remove, d.old.dir
		case d.old.dir && d.new.dir:
			continue // a directory's entries changed, which they say
		default:
			ev.Op, ev.Dir = Write, d.new.dir
		}
		if ev.Op == Remove || ev.Dir {
			if ev.Op == Remove {
				w.mu.Lock()
				delete(w.sums, d.path)
				w.mu.Unlock()
			}
			evs = append(evs, ev)
			continue
		}
		looks = append(looks, &look{ev: ev, d: d, content: w.o.Content.matches(d.path), prev: d.new})
	}
	w.steady(looks)
	for _, l := range looks {
		if ev, ok := w.settled(l); ok {
			evs = append(evs, ev)
		}
	}
	sortEvents(evs)
	if len(evs) > 1 {
		dedup := evs[:0]
		for _, e := range evs {
			if n := len(dedup); n > 0 && dedup[n-1].Path == e.Path {
				if dedup[n-1].Op == Remove && e.Op == Remove {
					continue
				}
				if m, keep := then(dedup[n-1], e); keep {
					dedup[n-1] = m
				} else {
					dedup = dedup[:n-1]
				}
				continue
			}
			dedup = append(dedup, e)
		}
		evs = dedup
	}
	if w.o.Filter != nil {
		kept := evs[:0]
		for _, e := range evs {
			if w.o.Filter(e) {
				kept = append(kept, e)
			}
		}
		evs = kept
	}
	return evs
}

// steady looks at each file until it holds still: not empty, and the
// same twice running, or, for one whose content is judged, the same
// before and after it was read. A file is looked at for the first time at
// once, and compared with what the scan saw, so one that is written and
// left costs one more stat.
//
// An empty file is taken as one caught being written, though it held
// still: a save truncates a file and then writes it, and a scan and a
// look can both fall in between. One that stays empty is reported as it
// is when the time is up.
func (w *Watcher) steady(looks []*look) {
	if len(looks) == 0 {
		return
	}
	deadline := time.Now().Add(w.o.Steady)
	for {
		left := false
		for _, l := range looks {
			if l.done {
				continue
			}
			if l.content {
				before := stat(l.d.path)
				data, err := readFile(l.d.path)
				after := stat(l.d.path)
				l.prev = after
				switch {
				case !after.exist:
					l.done = true
				case err == nil && before == after && len(data) > 0:
					l.data, l.done, l.steady = data, true, true
				case err != nil && !transient(err) && !os.IsNotExist(err):
					l.done = true // unreadable: reported as found
				}
			} else {
				cur := stat(l.d.path)
				switch {
				case !cur.exist:
					l.done = true
				case cur == l.prev && cur.size > 0:
					l.done, l.steady = true, true
				}
				l.prev = cur
			}
			left = left || !l.done
		}
		if !left || w.o.Steady < 0 || time.Now().After(deadline) {
			return
		}
		time.Sleep(steadyStep)
	}
}

// settled makes the event of a file that was looked at, and keeps its
// stamp as the one the next scan compares with, so that it is not judged
// again for what this look already saw.
func (w *Watcher) settled(l *look) (Event, bool) {
	ev, p, st := l.ev, l.d.path, l.prev
	var sum []byte
	if l.content && l.steady && l.data != nil {
		sum = w.o.Content.Sum(p, l.data)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, watched := w.files[p]; watched {
		w.files[p] = st
	}
	if !st.exist {
		// Gone while it was looked at.
		delete(w.sums, p)
		if ev.Op == Create {
			return Event{}, false
		}
		ev.Op = Remove
		return ev, true
	}
	if !l.content {
		return ev, true
	}
	if sum == nil {
		delete(w.sums, p)
		return ev, true
	}
	old, ok := w.sums[p]
	if _, watched := w.files[p]; watched {
		w.sums[p] = fileSum{st, sum}
	}
	ev.Same = ev.Op == Write && ok && old.st == l.d.old && bytes.Equal(old.sum, sum)
	return ev, true
}

// fingerprint takes, meanwhile, the Content sums a later edit of paths is
// compared with, each with the stamp of the version read, or none for a
// file that changed while it was read.
func (w *Watcher) fingerprint(paths []string) {
	var files []string
	for _, p := range paths {
		if w.o.Content.matches(p) {
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return
	}
	w.bgMu.Lock()
	w.bg.Add(1)
	w.bgMu.Unlock()
	go func() {
		defer w.bg.Done()
		for _, p := range files {
			before := stat(p)
			if !before.exist || before.dir {
				continue
			}
			src, err := readFile(p)
			if err != nil || stat(p) != before {
				continue
			}
			sum := w.o.Content.Sum(p, src)
			w.mu.Lock()
			_, watched := w.files[p]
			if _, ok := w.sums[p]; watched && !ok {
				w.sums[p] = fileSum{before, sum}
			}
			w.mu.Unlock()
		}
	}()
}
