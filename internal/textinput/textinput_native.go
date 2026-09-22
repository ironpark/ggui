// Copyright 2026 The ggfx Authors
// SPDX-License-Identifier: Apache-2.0

//go:build (darwin && !ios) || windows

package textinput

import (
	"image"

	"github.com/ironpark/ggfx"
)

// All accesses are from ggfx's event-handler goroutine. Each window keeps its
// own session queue; WithWindow selects the context while that window runs a
// frame. Sessions retain their owner even when another window handles events.
var windowInputs = map[*ggfx.Window]*textInput{}

func newTextInput(w *ggfx.Window) *textInput {
	t := &textInput{}
	t.impl.events, t.impl.window = &t.events, w
	return t
}

func inputFor(w *ggfx.Window) *textInput {
	t := windowInputs[w]
	if t == nil {
		t = newTextInput(w)
		windowInputs[w] = t
	}
	return t
}

func WithWindow(w *ggfx.Window) func() {
	old := theTextInput
	theTextInput = inputFor(w)
	return func() {
		// Text arriving before focus was dispatched may seed a session in this
		// frame, but must not leak into a target focused in a later frame.
		if !theTextInput.impl.enabled {
			theTextInput.events.clearQueue()
		}
		theTextInput = old
	}
}

func CloseWindow(w *ggfx.Window) {
	if t := windowInputs[w]; t != nil {
		t.cancelSessionIfNeeded()
		t.abandonTarget()
		delete(windowInputs, w)
	}
}

// HandleEvent feeds the owning window's session without polling the platform.
// A true result means a TextEvent was consumed by its editable target.
func HandleEvent(event ggfx.Event) bool {
	switch e := event.(type) {
	case ggfx.CompositionEvent:
		t := windowInputs[e.Window]
		if t == nil || !t.impl.enabled {
			return false
		}
		t.events.send(textInputState{Text: e.Text,
			CompositionSelectionStartInBytes: e.Start, CompositionSelectionEndInBytes: e.End,
			ReplacementStartInBytes: noReplacement, ReplacementEndInBytes: noReplacement})
		return true
	case ggfx.TextEvent:
		t := inputFor(e.Window)
		st := textInputState{Text: e.Text, CommitKind: commitRegular,
			ReplacementStartInBytes: noReplacement, ReplacementEndInBytes: noReplacement}
		if e.HasReplacement {
			st.ReplacementStartInBytes, st.ReplacementEndInBytes = e.ReplacementStart, e.ReplacementEnd
			st.ReplacementRelativeToCaret = true
		}
		t.events.send(st)
		t.events.end()
		return t.impl.enabled
	}
	return false
}

type textInputImpl struct {
	events  *textInputEvents
	window  *ggfx.Window
	enabled bool
	rect    image.Rectangle // the caret last handed to the window
}

// setRect passes the caret rectangle on only when it moved: the window
// method is a synchronous main-thread hop, and the editor reports the
// caret on every tick it is focused.
func (t *textInputImpl) setRect(bounds image.Rectangle) {
	if bounds == t.rect {
		return
	}
	t.rect = bounds
	t.window.SetTextInputRect(float64(bounds.Min.X), float64(bounds.Min.Y), float64(bounds.Dx()), float64(bounds.Dy()))
}

func (t *textInputImpl) markIMEDiscardNeeded() {
	if t.window != nil && t.enabled {
		t.window.SetTextInputEnabled(false)
	}
	t.enabled = false
}

func (t *textInputImpl) Start(bounds image.Rectangle, before, after string) (<-chan textInputState, func()) {
	if t.window == nil {
		return nil, nil
	}
	t.setRect(bounds)
	t.window.SetTextInputContext(before, after)
	if !t.enabled {
		t.window.SetTextInputEnabled(true)
		t.enabled = true
	}
	return t.events.start()
}

// UpdateCaret follows scrolling and layout even while a composition is active.
func UpdateCaret(bounds image.Rectangle) {
	t := &theTextInput.impl
	if t.window != nil && t.enabled {
		t.setRect(bounds)
	}
}
