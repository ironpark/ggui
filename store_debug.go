//go:build ggui_debug

package ggui

import (
	"log"
	"sync"
	"weak"

	"github.com/ironpark/ggui/internal/goid"
)

// A ggui_debug build checks every live Store after each frame's input.
// watched holds each weakly, so a store no longer used is let go.
var (
	watchedMu     sync.Mutex
	watched       []func(report func(store, sel string)) (live bool)
	staleReported sync.Map
)

func watchStore[M any](s *Store[M]) {
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
	if _, seen := staleReported.LoadOrStore(sel, true); !seen {
		log.Printf("ggui: the model of the Store made at %s changed without being published, so the Select at %s shows a stale value; "+
			"change the model in Store.Update or Store.Action, or call Store.Changed after changing it", store, sel)
	}
}

// checkStale reports every Select whose model changed with nothing
// published, once per place.
func (s *Store[M]) checkStale(report func(store, sel string)) {
	for _, check := range s.checks {
		if origin, stale := check(); stale {
			report(s.origin, origin)
		}
	}
}
