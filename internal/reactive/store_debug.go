//go:build ggui_debug

package reactive

import (
	"reflect"
	"sync"
	"weak"

	"github.com/ironpark/ggui/internal/reactive/goid"
)

// A ggui_debug build checks every live Store after each frame's input.
// watched holds each weakly, so a store no longer used is let go.
var (
	watchedMu sync.Mutex
	watched   []func(report func(store, sel string)) (live bool)
	// checkMu has CheckStores check one at a time.
	checkMu sync.Mutex
)

// storeChecks is where a Store was made and a check for each live Select.
type storeChecks struct {
	origin string
	checks []selectCheck
}

type selectCheck struct {
	origin string
	// stale reports whether the Select's model changed unpublished, and
	// live whether it is still in use.
	stale func() (stale, live bool)
}

func watchSelect[M, T any](s *Store[M], d *DerivedValue[T], fn func(M) T) {
	s.checks = append(s.checks, selectCheck{Origin(), func() (bool, bool) {
		// A disposed Select keeps its last value, which no one reads.
		if d.eff.disposed {
			return false, false
		}
		// A Select that has not been evaluated yet or is currently dirty from
		// a published store change must not be eagerly evaluated here.
		if d.eff.state == stateDirty || len(d.eff.sources) == 0 {
			return false, true
		}
		return !same(d, Untrack(d.Get), fn(s.model)), true
	}})
}

// same reports whether m's equality takes a and b for one value, falling
// back on reflect.DeepEqual where m has none; for the check comparing a
// result with the one m holds.
func same[T any](m *DerivedValue[T], a, b T) bool {
	if eq := m.sig.eq; eq != nil {
		return eq(a, b)
	}
	return reflect.DeepEqual(a, b)
}

func watchStore[M any](s *Store[M]) {
	s.origin = Origin()
	p, g := weak.Make(s), goid.ID()
	watchedMu.Lock()
	defer watchedMu.Unlock()
	watched = append(watched, func(report func(store, sel string)) bool {
		s := p.Value()
		// A store is the goroutine's that made it, as its signals are.
		if s != nil && g == goid.ID() {
			s.checkStale(report)
		}
		return s != nil
	})
}

// CheckStores reports a Select whose model changed with nothing published,
// once per place it was made. The frame loop calls it after each frame's
// input.
func CheckStores() {
	// Frames on goroutines of their own check at once; one at a time, or
	// one would cut watched down below the length another read.
	checkMu.Lock()
	defer checkMu.Unlock()
	watchedMu.Lock()
	list := watched
	watchedMu.Unlock()
	live := make([]func(func(string, string)) bool, 0, len(list))
	for _, check := range list {
		if check(reportStale) {
			live = append(live, check)
		}
	}
	watchedMu.Lock()
	// Stores made while checking were appended past the end of list.
	watched = append(live, watched[len(list):]...)
	watchedMu.Unlock()
}

func reportStale(store, sel string) {
	ReportOnce("store "+sel, "ggui: the model of the Store made at %s changed without being published, so the Select at %s shows a stale value; "+
		"change the model in Store.Update or Store.Action, or call Store.Changed after changing it", store, sel)
}

// checkStale reports every Select whose model changed with nothing
// published, and forgets those disposed.
func (s *Store[M]) checkStale(report func(store, sel string)) {
	live := s.checks[:0]
	for _, c := range s.checks {
		stale, ok := c.stale()
		if stale {
			report(s.origin, c.origin)
		}
		if ok {
			live = append(live, c)
		}
	}
	clear(s.checks[len(live):])
	s.checks = live
}
