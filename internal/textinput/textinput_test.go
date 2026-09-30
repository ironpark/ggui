// Copyright 2025 The Ebitengine Authors
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

//go:build darwin && !ios

package textinput_test

import (
	"testing"

	"github.com/ironpark/ggui/internal/textinput"
)

// TestClearQueueDropsDiscardedMarkedText verifies the macOS fix: a preedit
// queued after a commit closed the channel would be replayed as a live
// composition by the next session's start(). Clearing the queue first (as macOS
// start() does after discarding the OS marked text) drops the stale preedit.
func TestClearQueueDropsDiscardedMarkedText(t *testing.T) {
	for _, tc := range []struct {
		name       string
		clearQueue bool
		want       bool
	}{
		{
			// The queued preedit is replayed as a live composition: the bug.
			name:       "without clear",
			clearQueue: false,
			want:       true,
		},
		{
			// Clearing after the discard drops the stale preedit.
			name:       "with clear",
			clearQueue: true,
			want:       false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ev textinput.TextInputEvents

			// Session 1: the IME commits, then closes the channel.
			ev.Start()
			ev.Send(textinput.TextInputState{Text: "committed", CommitKind: textinput.CommitRegular})
			ev.End()

			// A keystroke between sessions begins a preedit; with the channel
			// closed, it is queued.
			ev.Send(textinput.TextInputState{Text: "l"})

			if tc.clearQueue {
				ev.ClearQueue()
			}

			// Session 2: the queued preedit is replayed only if not cleared.
			if got := ev.StartSessionCompositing(); got != tc.want {
				t.Errorf("StartSessionCompositing() = %v, want %v", got, tc.want)
			}
		})
	}
}

// commitState returns a state representing a committed text.
func commitState(text string) textinput.TextInputState {
	return textinput.TextInputState{Text: text, CommitKind: textinput.CommitRegular}
}

// compositionState returns a state representing text being composed, with
// the caret at its end.
func compositionState(text string) textinput.TextInputState {
	return textinput.TextInputState{Text: text, CompositionSelectionStartInBytes: len(text), CompositionSelectionEndInBytes: len(text)}
}

// TestQueuedCommitsReachSuccessiveSessions verifies that several commits
// arriving before the next tick are all delivered, in order. A session ends at
// its first commit, so flushing every queued commit into one session's channel
// would close that channel over the ones it never read.
func TestQueuedCommitsReachSuccessiveSessions(t *testing.T) {
	var ev textinput.TextInputEvents

	// Three keystrokes land between ticks, with no session open to take them.
	for _, text := range []string{"a", "b", "c"} {
		if ev.Send(commitState(text)) {
			t.Fatalf("Send(%q) reported delivery with no session open", text)
		}
	}

	// The application starts a session per commit it observes, as Composer
	// does within a single Update.
	for _, want := range []string{"a", "b", "c"} {
		got, ok := ev.StartSessionCommit()
		if !ok {
			t.Fatalf("no commit delivered, want %q", want)
		}
		if got != want {
			t.Errorf("commit = %q, want %q", got, want)
		}
	}

	if got, ok := ev.StartSessionCommit(); ok {
		t.Errorf("a fourth session got commit %q, want none", got)
	}
}

// TestQueuedCompositionsPrecedeTheirCommit verifies that the states before a
// commit reach the same session as the commit, and that what follows it does
// not.
func TestQueuedCompositionsPrecedeTheirCommit(t *testing.T) {
	var ev textinput.TextInputEvents

	ev.Send(textinput.TextInputState{Text: "a"})
	ev.Send(commitState("A"))
	ev.Send(textinput.TextInputState{Text: "b"})
	ev.Send(commitState("B"))

	for _, want := range []string{"A", "B"} {
		got, ok := ev.StartSessionCommit()
		if !ok {
			t.Fatalf("no commit delivered, want %q", want)
		}
		if got != want {
			t.Errorf("commit = %q, want %q", got, want)
		}
	}
}

// TestSendReportsDelivery verifies that Send distinguishes a state an open
// session took from one merely queued. The X11 backend ends a session only on
// the former, so that a commit no session read cannot push the deadline that
// decides whether queued states still belong to a session about to start.
func TestSendReportsDelivery(t *testing.T) {
	var ev textinput.TextInputEvents

	// No session: the state is queued, not delivered.
	if ev.Send(commitState("a")) {
		t.Error("Send with no session open reported delivery")
	}

	// A session takes the queued commit, and ends at it, so a further commit
	// belongs to the next session.
	ev.Start()
	if ev.Send(commitState("b")) {
		t.Error("Send after the session took its commit reported delivery")
	}
	ev.End()

	// A session with nothing queued takes a commit directly.
	var fresh textinput.TextInputEvents
	fresh.Start()
	if !fresh.Send(commitState("c")) {
		t.Error("Send to an open session did not report delivery")
	}
	fresh.End()
}

// A platform-side ending recorded as the user's, e.g. the user dismissing the
// virtual keyboard, must reach the session observing the closed channel, so
// that the application can stop text inputting instead of restarting it,
// which would show the virtual keyboard again.
func TestUserEndingReachesSession(t *testing.T) {
	d := textinput.NewSession()

	// The user composes, then dismisses the virtual keyboard.
	d.Send(compositionState("か"))
	d.EndByUser()
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !d.Closed() {
		t.Error("SessionClosed() = false, want true")
	}
	if !d.ClosedByUser() {
		t.Error("SessionClosedByUser() = false, want true")
	}

	// The record must not leak into the next session.
	d.StartNext()
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if d.ClosedByUser() {
		t.Error("SessionClosedByUser() = true for the next session, want false")
	}
}

// A platform teardown that is not the user's, e.g. a focus loss, must not be
// recorded as the user's ending.
func TestPlatformTeardownIsNotUserEnding(t *testing.T) {
	d := textinput.NewSession()

	d.Send(compositionState("か"))
	d.End()
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !d.Closed() {
		t.Error("SessionClosed() = false, want true")
	}
	if d.ClosedByUser() {
		t.Error("SessionClosedByUser() = true, want false")
	}
}

// A commit and the user's dismissal can land in the same tick: the commit ends
// the session, and the ending must still be recorded as the user's, or the
// application would restart text inputting and show the virtual keyboard
// again.
func TestUserEndingAfterCommitInSameTick(t *testing.T) {
	d := textinput.NewSession()

	d.Send(commitState("a"))
	d.EndByUser()
	if err := d.Update(); err != nil {
		t.Fatalf("Update: %v", err)
	}
	c := d.Commit()
	if c == nil {
		t.Fatal("the session got no commit")
	}
	if got, want := c.Text(), "a"; got != want {
		t.Errorf("Text() = %q, want %q", got, want)
	}
	if !d.ClosedByUser() {
		t.Error("SessionClosedByUser() = false, want true")
	}
}
