// Copyright 2026 The Ebitengine Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !js

package textinput

import (
	"image"
)

// Composer drives an IME loop for a single text-input target. It owns
// the per-tick session lifecycle and dispatches composition and commit
// events through caller-registered callbacks.
//
// A session can span multiple ticks. While one is in progress, do not
// edit the target buffer except in response to [Composer.OnCommit]: a
// commit is applied relative to the surrounding text captured by
// [Composer.OnNewSession], so editing underneath a live session can leave
// that position stale. Call [Composer.Confirm] before a caller-driven edit.
//
// See examples/textinput in the Ebitengine repository for a complete
// usage example.
type Composer struct {
	// OnNewSession is called when Composer needs to start a new session
	// (initial use, after a commit, or after a platform-side teardown).
	// It must return the caret bounds and the text immediately around the
	// caret so that the IME can position the candidate window and use the
	// surrounding text for prediction, or nil to start no session for now
	// (Update asks again later). Required.
	OnNewSession func() *SessionOptions

	// OnComposition is called when the IME's preedit changes, including
	// when the session ends and the preedit should be cleared (the
	// Composition is then the zero value). Optional.
	OnComposition func(c *Composition)

	// OnCommit is called when the IME commits. The caller applies the
	// committed text to its own buffer. Optional.
	OnCommit func(c *Commit)

	// OnEndByUser is called when the user ends text inputting from the
	// platform side, e.g. by dismissing the virtual keyboard. The caller
	// should stop text inputting, typically by unfocusing its text field:
	// starting another session would show the virtual keyboard again.
	// An in-progress composition is committed through OnCommit first, as
	// [Composer.Confirm] does. Optional.
	OnEndByUser func()

	s *session
}

// SessionOptions describes the IME's view of the caret. It is returned
// by Composer.OnNewSession when Composer needs to start a new session.
type SessionOptions struct {
	// CaretBounds is the rectangle of the caret in logical pixels.
	// The platform IME may use it to position the candidate window.
	CaretBounds image.Rectangle

	// TextBeforeCaret is text immediately before the caret, typically the
	// portion of the current line that lies before the caret. The platform
	// IME may use it for prediction and reconversion.
	TextBeforeCaret string

	// TextAfterCaret is text immediately after the caret, typically the
	// portion of the current line that lies after the caret. The platform
	// IME may use it for prediction and reconversion.
	TextAfterCaret string
}

// Composition describes the IME's current preedit text. It is passed to
// Composer.OnComposition.
type Composition struct {
	text     string
	selStart int
	selEnd   int
}

// Text returns the preedit text. Empty when no composition is in progress.
func (c *Composition) Text() string {
	return c.text
}

// SelectionRangeInBytes returns the IME-side selection within Text as a
// half-open byte range [start, end).
func (c *Composition) SelectionRangeInBytes() (start, end int) {
	return c.selStart, c.selEnd
}

// Commit describes the data the IME recorded when committing. It is passed
// to [Composer.OnCommit].
//
// In the typical case the caller inserts Text at the current caret. When
// the IME requested a replacement of part of the surrounding text — see
// [Commit.IsSurroundingTextReplaced] and [Commit.SurroundingText] — the
// caller replaces the original [SessionOptions.TextBeforeCaret] +
// [SessionOptions.TextAfterCaret] slice with prefix + Text + suffix, where
// prefix and suffix come from SurroundingText, and places the caret at
// the boundary after Text.
type Commit struct {
	text            string
	textBeforeCaret string
	textAfterCaret  string
	replStart       int
	replEnd         int
	passthroughKey  bool
}

// Text returns the text the IME committed.
func (c *Commit) Text() string {
	return c.text
}

// HasPassthroughKey reports whether the commit's triggering key press also
// passes through to the game's own key input, in which case the caller
// usually still acts on that key.
func (c *Commit) HasPassthroughKey() bool {
	return c.passthroughKey
}

// IsSurroundingTextReplaced reports whether the IME requested a
// replacement of the before-caret or after-caret region. Both are false in
// the typical case (the IME just inserted Text at the caret); the caller
// can simply insert Text at its current caret without consulting
// [Commit.SurroundingText].
func (c *Commit) IsSurroundingTextReplaced() (before, after bool) {
	preLen := len(c.textBeforeCaret)
	return c.replStart != preLen, c.replEnd != preLen
}

// SurroundingText returns the prefix and suffix that remain after the
// IME's replacement region is removed from the joined
// [SessionOptions.TextBeforeCaret] + [SessionOptions.TextAfterCaret]
// slice. The two strings equal the original SessionOptions values when
// [Commit.IsSurroundingTextReplaced] returns (false, false).
func (c *Commit) SurroundingText() (before, after string) {
	preLen := len(c.textBeforeCaret)
	if c.replStart <= preLen {
		before = c.textBeforeCaret[:c.replStart]
	} else {
		before = c.textBeforeCaret + c.textAfterCaret[:c.replStart-preLen]
	}
	if c.replEnd >= preLen {
		after = c.textAfterCaret[c.replEnd-preLen:]
	} else {
		after = c.textBeforeCaret[c.replEnd:] + c.textAfterCaret
	}
	return
}

// Update processes IME events for one tick and dispatches them through
// the registered callbacks. It returns true if the IME consumed input
// during this tick — the caller should skip its own key handlers when
// handled is true to avoid double-processing keys.
//
// A commit delivered with a key press that the game also receives leaves
// handled false, so the caller still acts on that key.
//
// Update may invoke OnCommit and OnComposition multiple times in one call
// if the platform queued multiple compositions between ticks.
//
// Update should be called every tick (Update) during editing. It is safe to
// call more often than once per tick.
func (c *Composer) Update() (handled bool, err error) {
	for {
		if c.s == nil {
			var open bool
			open, err = c.openSession()
			if err != nil {
				return false, err
			}
			if !open {
				return handled, nil
			}
		}

		if err = c.s.Update(); err != nil {
			c.s = nil
			c.dispatchEmptyComposition()
			return handled, err
		}

		h, again := c.stepSession()
		handled = handled || h
		if !again {
			break
		}
	}
	return handled, nil
}

// stepSession dispatches what one session tick produced and reports
// whether the loop should open another session: a commit the user did
// not end leaves one more pass.
func (c *Composer) stepSession() (handled, again bool) {
	if c.s.IsCommitted() {
		h, endedByUser := c.commitSession()
		return h, !endedByUser
	}
	if c.s.IsClosed() {
		c.closeSession()
		return false, false
	}
	// Active session: composing or idle.
	return c.dispatchActive(), false
}

// commitSession dispatches the committed text and an empty composition,
// and reports whether the IME consumed the key and whether the user ended
// the session.
func (c *Composer) commitSession() (handled, endedByUser bool) {
	if c.OnCommit != nil {
		c.OnCommit(c.s.Commit())
	}
	c.dispatchEmptyComposition()
	// A commit whose key passes through to the game leaves handled false.
	handled = !c.s.IsCommittedWithPassthroughKey()
	endedByUser = c.s.IsClosedByUser()
	c.s = nil
	if endedByUser {
		c.dispatchEndByUser()
	}
	return handled, endedByUser
}

// openSession starts a session when the platform offers one, reporting
// whether a session is running.
func (c *Composer) openSession() (bool, error) {
	var opts *SessionOptions
	if c.OnNewSession != nil {
		opts = c.OnNewSession()
		if opts == nil {
			return false, nil
		}
	}
	sess, err := startSession(opts)
	if err != nil {
		return false, err
	}
	if sess == nil {
		return false, nil
	}
	c.s = sess
	return true, nil
}

// closeSession ends a session the platform closed: the user's ending
// commits what they last saw, as Confirm does.
func (c *Composer) closeSession() {
	// The user's ending discards no composition: what the user last saw
	// is committed, as Confirm does.
	if c.s.IsClosedByUser() && c.OnCommit != nil && c.s.loadComposition().text != "" {
		c.OnCommit(c.s.compositionAsCommit())
	}
	endedByUser := c.s.IsClosedByUser()
	c.s = nil
	c.dispatchEmptyComposition()
	if endedByUser {
		c.dispatchEndByUser()
	}
}

// dispatchActive reports the session's composition and whether the IME
// is composing.
func (c *Composer) dispatchActive() bool {
	c.dispatchComposition(c.s.Composition())
	return c.s.IsCompositing()
}

// Confirm ends the current session if any. Any in-progress composition is
// committed through OnCommit as if the IME had committed it, then
// OnComposition is fired with an empty composition so the caller can clear
// its preedit overlay.
func (c *Composer) Confirm() {
	c.end(true)
}

// Cancel ends the current session if any, discarding an in-progress
// composition: unlike [Composer.Confirm], nothing is committed.
// OnComposition is fired with an empty composition so the caller can clear
// its preedit overlay.
func (c *Composer) Cancel() {
	c.end(false)
}

// end ends the current session if any, committing an in-progress
// composition through OnCommit only when commit is true.
func (c *Composer) end(commit bool) {
	if c.s == nil {
		return
	}
	if commit && c.OnCommit != nil && c.s.loadComposition().text != "" {
		c.OnCommit(c.s.compositionAsCommit())
	}
	c.s.Cancel()
	c.s = nil
	c.dispatchEmptyComposition()
}

func (c *Composer) dispatchComposition(comp Composition) {
	if c.OnComposition != nil {
		c.OnComposition(&comp)
	}
}

func (c *Composer) dispatchEmptyComposition() {
	if c.OnComposition != nil {
		c.OnComposition(&Composition{})
	}
}

func (c *Composer) dispatchEndByUser() {
	if c.OnEndByUser != nil {
		c.OnEndByUser()
	}
}

// Rebase points c's session at new text around the caret, as after the
// caller replaced its buffer outside a commit: the IME's next edit applies
// to it, and the platform reads it back. Unlike ending the session, the
// platform's text input stays on, so a key typed before the next tick
// still reaches the IME. It is a no-op without a session.
func Rebase(c *Composer, before, after string) {
	if c.s == nil || c.s.IsClosed() {
		return
	}
	c.s.textBeforeCaret, c.s.textAfterCaret = before, after
	if c.s.input != nil {
		c.s.input.impl.setContext(before, after)
	}
}
