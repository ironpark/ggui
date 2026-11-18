// Package devstate is the app's side of the state ggui dev keeps across a
// restart: the values an app marks with StateValue.Keep, taken from the
// instance a rebuild replaces and handed to the one that replaces it, so
// that a form half filled in, the tab that was open or the page the router
// showed are still there after a save.
//
// It does nothing unless ggui dev started the app. Without ggui dev's
// channel in the environment (on the web, without the hook ggui dev's page
// sets) there is no session: Keep is one check of an atomic and returns,
// and no goroutine, connection or registry is ever made. Under ggui dev the
// first Keep begins the session, synchronously, so that the state handed
// over is in hand before that Keep returns: a state kept in the app's setup
// shows its handed value in the first frame.
//
// Values travel as JSON, which the new build decodes into its own types. A
// type that changed between the builds is the normal case, not an error:
// a value that no longer decodes is dropped, said once on stderr, and its
// state starts from its initial value.
package devstate

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"sync"
	"sync/atomic"
)

// State is what is handed over: each kept value's JSON by its key.
type State = map[string]json.RawMessage

// session is ggui dev's state for one run of the app: what was handed over
// and is not yet claimed, and the values kept now.
type session struct {
	mu      sync.Mutex
	handed  State             // handed over and not yet claimed by a Keep
	entries map[string]*entry // kept now, by key
}

// entry is one kept value.
type entry struct {
	save func() ([]byte, error)
}

var (
	begin sync.Once
	cur   atomic.Pointer[session]
)

// active returns the session, beginning it on the first call when ggui dev
// started the app, or nil when it did not.
func active() *session {
	begin.Do(func() {
		if s := connect(); s != nil {
			cur.Store(s)
		}
	})
	return cur.Load()
}

// On reports whether ggui dev keeps state for this app. The first call
// connects to ggui dev when it started the app.
func On() bool { return active() != nil }

func newSession(handed State) *session {
	return &session{handed: handed, entries: map[string]*entry{}}
}

// Keep keeps a value under key: restore is called at once with the value
// handed over under key, when there is one no Keep has claimed, and save
// is called whenever ggui dev asks for the state, until release. A key
// kept twice is the later one's: replaced reports that it took the key
// over from a value still kept. Without a session Keep does nothing and
// returns a nil release.
//
// restore runs on the caller's goroutine, before Keep returns. A value that
// it fails to decode is claimed all the same, so that it is not offered
// again, and said once on stderr.
func Keep(key string, restore func([]byte) error, save func() ([]byte, error)) (release func(), replaced bool) {
	s := active()
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	b, handed := s.handed[key]
	delete(s.handed, key)
	_, replaced = s.entries[key]
	e := &entry{save: save}
	s.entries[key] = e
	s.mu.Unlock()
	if handed {
		if err := restore(b); err != nil {
			reportf("decode "+key, "ggui: the state kept as %q no longer decodes (%v); it starts from its initial value", key, err)
		}
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.entries[key] == e {
			delete(s.entries, key)
		}
	}, replaced
}

// snapshot encodes the values kept now, with those handed over that no
// Keep has claimed yet: a state the old instance was handed for a page
// it never showed is still there for the next. A value that does not
// encode is left out, and said once on stderr. The values are encoded
// outside the lock, as an encoder may run the app's code.
func (s *session) snapshot() State {
	s.mu.Lock()
	out := maps.Clone(s.handed)
	if out == nil {
		out = State{}
	}
	kept := maps.Clone(s.entries)
	s.mu.Unlock()
	for key, e := range kept {
		b, err := e.save()
		if err != nil {
			reportf("encode "+key, "ggui: the state kept as %q does not encode (%v); it is not kept", key, err)
			continue
		}
		out[key] = b
	}
	return out
}

// Snapshot is what the app would hand over now, or nil without a session.
// Call it on the UI thread.
func Snapshot() State {
	if s := cur.Load(); s != nil {
		return s.snapshot()
	}
	return nil
}

// Begin starts a session with handed as what ggui dev handed over, in place
// of the one the environment would give, and returns the function that
// ends it: for tests, which hand state over without a ggui dev.
func Begin(handed State) (end func()) {
	begin.Do(func() {})
	s := newSession(maps.Clone(handed))
	cur.Store(s)
	return func() { cur.CompareAndSwap(s, nil) }
}

// Stderr is where the session's notices go.
var Stderr io.Writer = os.Stderr

// reported holds the notices already given.
var reported sync.Map

// reportf says something once per key, on Stderr, where ggui dev shows
// the app's output.
func reportf(key, format string, args ...any) {
	if _, dup := reported.LoadOrStore(key, true); !dup {
		fmt.Fprintf(Stderr, format+"\n", args...)
	}
}

// message is one line of the channel to ggui dev, a JSON object a line
// each way, as ggui dev's viewer speaks:
//
//	{"type":"hello","instance":"3"}     the app, once, to ggui dev
//	{"type":"state","state":{...}}      ggui dev's answer: what it hands over
//	{"type":"save"}                     ggui dev, before it stops the app
//	{"type":"saved","state":{...}}      the app's answer: what it keeps now
type message struct {
	Type     string `json:"type"`
	Instance string `json:"instance,omitempty"`
	State    State  `json:"state,omitempty"`
}
