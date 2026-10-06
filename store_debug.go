//go:build ggui_debug

package ggui

import (
	"sync"
	"weak"

	"github.com/ironpark/ggui/internal/goid"
	"github.com/ironpark/ggui/internal/reactive"
)

// A ggui_debug build checks every live Store after each frame's input.
// watched holds each weakly, so a store no longer used is let go.
var (
	watchedMu sync.Mutex
	watched   []func(report func(store, sel string)) (live bool)
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
	s.checks = append(s.checks, selectCheck{reactive.Origin(), func() (bool, bool) {
		if reactive.Disposed(d) {
			return false, false
		}
		return !reactive.Same(d, Untrack(d.Get), fn(s.model)), true
	}})
}

func watchStore[M any](s *Store[M]) {
	s.origin = reactive.Origin()
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

// checkStores reports a Select whose model changed with nothing published,
// once per place it was made.
func checkStores() {
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
	reactive.ReportOnce("store "+sel, "ggui: the model of the Store made at %s changed without being published, so the Select at %s shows a stale value; "+
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
