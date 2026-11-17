package dev

import (
	"slices"
	"time"
)

// plan decides when to build and whether a build's result may be used.
// It is the loop's arithmetic without the loop: it is told of changes,
// of time passing and of builds ending, and says what to do, so that a
// test can drive it with a clock of its own.
//
// A real change is held until the sources go quiet for quietTime, or
// quietTimeMany once many files changed, but no longer than debounceCap
// after the first; then a build starts. A real change while a build runs
// cancels it: the build is of code that changed since, and the next one
// starts after the same wait, with the cancelled build's changes joined to
// the new. A build that ends after a change it did not cancel, as one the
// last scan before a restart finds, is thrown away the same way. The app
// is never restarted into a build of sources older than those on disk.
type plan struct {
	pending []string  // real changes not yet in a build
	since   time.Time // when the earliest of pending, carried ones too, was found
	first   time.Time // the first change since the last build started
	last    time.Time // the latest
	many    bool

	building   bool
	id         int // the build running, or the last that ran
	batch      []string
	batchSince time.Time
}

// change takes a change found at at. It reports whether the running build
// must be cancelled.
func (p *plan) change(at time.Time, real []string) (cancel bool) {
	if len(real) == 0 {
		return false
	}
	if p.building {
		p.building = false
		p.pending = union(p.batch, p.pending)
		p.since = earliest(p.since, p.batchSince)
		cancel = true
	}
	if p.first.IsZero() {
		p.first = at
	}
	if at.After(p.last) {
		p.last = at
	}
	p.since = earliest(p.since, at)
	p.pending = union(p.pending, real)
	if len(p.pending) >= manyChanges {
		p.many = true
	}
	return cancel
}

// idle reports whether no build is running or waiting to.
func (p *plan) idle() bool { return !p.building && len(p.pending) == 0 }

// due returns when the pending changes are to be built, and false when
// there are none or a build runs.
func (p *plan) due() (time.Time, bool) {
	if p.building || len(p.pending) == 0 {
		return time.Time{}, false
	}
	q := quietTime
	if p.many {
		q = quietTimeMany
	}
	t := p.last.Add(q)
	if limit := p.first.Add(debounceCap); limit.Before(t) {
		t = limit
	}
	return t, true
}

// start takes the pending changes into a build and returns its id, the
// changes and when the earliest of them was found.
func (p *plan) start() (id int, batch []string, since time.Time) {
	p.id++
	p.building = true
	p.batch, p.batchSince = p.pending, p.since
	p.pending, p.since, p.first, p.last, p.many = nil, time.Time{}, time.Time{}, time.Time{}, false
	return p.id, p.batch, p.batchSince
}

// done ends build id and reports whether its result is current: it was
// not cancelled, and no change came while it ran.
func (p *plan) done(id int) bool {
	if !p.building || id != p.id {
		return false
	}
	p.building = false
	return true
}

func earliest(a, b time.Time) time.Time {
	if a.IsZero() || !b.IsZero() && b.Before(a) {
		return b
	}
	return a
}

func union(a, b []string) []string {
	out := append(slices.Clone(a), b...)
	slices.Sort(out)
	return slices.Compact(out)
}
