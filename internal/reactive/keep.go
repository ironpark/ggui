package reactive

import (
	"encoding/json"

	"github.com/ironpark/ggui/internal/devstate"
)

// Keep marks s as state ggui dev keeps across the restart that follows
// each rebuild, under key, and returns s, so that it chains onto State:
//
//	tab := ggui.State("general").Keep("settings.tab")
//
// When ggui dev restarts the app, it first asks the running instance for
// the values it keeps, as JSON, and hands them to the new one. There Keep
// restores the value handed over under key when it is called, before it
// returns, so a state kept in the app's setup shows it in the first frame,
// and one kept in a component that mounts later, after a navigation say,
// takes it when that component mounts. A value is handed to the first Keep
// of its key and claimed by it: a component mounted again afterwards
// starts from its initial value, as it would without ggui dev.
//
// The value is decoded over a JSON copy of s's initial value, so a field
// the new build added keeps its initial value and one it removed is
// ignored. A value that no longer decodes, because its type changed in a
// way JSON cannot follow, is dropped, said once on stderr, and s keeps its
// initial value; one that does not encode is not kept. What JSON does not
// carry, an unexported field say, is not kept either.
//
// s is kept while its owner lives, and let go with it. A key kept by two
// states at once is the later one's, and a ggui_debug build says so once:
// give each a key of its own, with an ID in it for a component shown more
// than once. The values the old instance was handed and nothing claimed
// are handed on.
//
// Run without ggui dev, or in a release build, Keep does nothing and costs
// one check. ggui dev --web keeps the values too, in the page's
// sessionStorage across its reload onto the new build.
func (s *StateValue[T]) Keep(key string) *StateValue[T] {
	if !devstate.On() {
		return s
	}
	release, replaced := devstate.Keep(key, s.restoreKept, s.saveKept)
	if replaced && Debug {
		ReportOnce("keep "+key, "ggui: two states are kept as %q at once; ggui dev keeps the one kept last. Give each a key of its own", key)
	}
	if CurrentOwner() != nil {
		OnCleanup(release)
	}
	return s
}

// restoreKept sets s to the value b holds, decoded over a JSON copy of the
// value s holds now: a copy, so that a map, slice or pointer the initial
// value shares with other code is never decoded into.
func (s *StateValue[T]) restoreKept(b []byte) error {
	s.mu.Lock()
	cur := s.val
	s.mu.Unlock()
	var v T
	if init, err := json.Marshal(cur); err == nil {
		if json.Unmarshal(init, &v) != nil {
			var zero T
			v = zero
		}
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	s.Set(v)
	return nil
}

// saveKept encodes s's value. It reads it under s's lock, without the UI
// thread's check, so that the browser's channel can take it from the
// page's call as well.
func (s *StateValue[T]) saveKept() ([]byte, error) {
	s.mu.Lock()
	v := s.val
	s.mu.Unlock()
	return json.Marshal(v)
}
