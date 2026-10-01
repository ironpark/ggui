//go:build !js

package textinput

import (
	"testing"

	"github.com/ironpark/ggfx"
)

// useTextInput makes t the current text input for the test, as WithWindow
// does for a frame. Tests that call it change process-wide state and must
// not run in parallel.
func useTextInput(tb testing.TB, t *textInput) {
	old := theTextInput
	theTextInput = t
	tb.Cleanup(func() { theTextInput = old })
}

// openSession opens a session on t's events and makes it the one owning the
// IME, as startSession does once the backend has started.
func openSession(t *textInput) *session {
	ch, end := t.events.start()
	s := &session{input: t, ch: ch, end: end, events: &t.events}
	t.events.setActiveSession(s)
	return s
}

// Only one session can own the IME: starting another cancels it and drops
// what was queued for its target.
func TestStartSessionCancelsTheSessionOwningTheIME(t *testing.T) {
	input := newTextInput(nil)
	useTextInput(t, input)
	owner := openSession(input)
	input.events.send(textInputState{Text: "a", CommitKind: commitRegular})
	input.events.send(textInputState{Text: "b"}) // queued behind the commit

	s, err := startSession(nil)
	if s != nil || err != nil {
		t.Fatalf("startSession without a window = %v, %v, want no session and no error", s, err)
	}
	if !owner.IsClosed() {
		t.Error("the session owning the IME is still open")
	}
	if got := input.events.getActiveSession(); got != nil {
		t.Error("the cancelled session still owns the IME")
	}
	if n := len(input.events.queuedStates); n != 0 {
		t.Errorf("%d states for the abandoned target are still queued, want 0", n)
	}
}

func TestComposerWithoutWindowStartsNoSession(t *testing.T) {
	useTextInput(t, newTextInput(nil))
	for _, c := range []*Composer{
		{OnNewSession: func() *SessionOptions { return &SessionOptions{TextBeforeCaret: "a"} }},
		{}, // no OnNewSession: default options
	} {
		if handled, err := c.Update(); handled || err != nil || c.s != nil {
			t.Errorf("Update without a window = %v, %v with session %v, want false, nil and none", handled, err, c.s)
		}
	}
}

func TestCloseWindowCancelsItsSession(t *testing.T) {
	w := new(ggfx.Window)
	input := inputFor(w)
	owner := openSession(input)
	input.events.send(textInputState{Text: "a", CommitKind: commitRegular})
	input.events.send(textInputState{Text: "b"})

	CloseWindow(w)
	if !owner.IsClosed() {
		t.Error("closing the window left its session open")
	}
	if n := len(input.events.queuedStates); n != 0 {
		t.Errorf("%d states still queued for the closed window, want 0", n)
	}
	if _, ok := windowInputs[w]; ok {
		t.Error("the closed window's text input is still registered")
	}
	CloseWindow(w) // closing again is a no-op
}
