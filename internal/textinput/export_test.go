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

package textinput

type TextInputEvents = textInputEvents

// TextInputState re-exports the internal state record so white-box tests can
// build the events they send.
type TextInputState = textInputState

// CommitKind and its constants re-export the internal commit-kind enum.
type CommitKind = commitKind

const (
	CommitNone               = commitNone
	CommitRegular            = commitRegular
	CommitWithPassthroughKey = commitWithPassthroughKey
)

// QueuedStateCount reports how many states are held for a session to take.
func (s *TextInputEvents) QueuedStateCount() int {
	s.m.Lock()
	defer s.m.Unlock()
	return len(s.queuedStates)
}

func (s *TextInputEvents) Start() {
	s.start()
}

func (s *TextInputEvents) End() {
	s.end()
}

func (s *TextInputEvents) ClearQueue() {
	s.clearQueue()
}

func (s *TextInputEvents) Send(state TextInputState) bool {
	return s.send(state)
}

// StartSessionCommit starts a session on a freshly opened channel, as the
// platform start() does (flushing any queued states), pumps one Update, and
// reports the committed text and whether a commit arrived.
func (s *TextInputEvents) StartSessionCommit() (string, bool) {
	ch, end := s.start()
	sess := &session{ch: ch, end: end, events: s}
	_ = sess.Update()
	if !sess.IsCommitted() {
		return "", false
	}
	return sess.Commit().text, true
}

// StartSessionCompositing starts a session on a freshly opened channel, as the
// platform start() does (flushing any queued states), pumps one Update, and
// reports whether the session observed a live composition.
func (s *TextInputEvents) StartSessionCompositing() bool {
	ch, end := s.start()
	sess := &session{ch: ch, end: end, events: s}
	_ = sess.Update()
	return sess.IsCompositing()
}

// Session is a session over events of its own, as a platform backend opens
// one, for tests of how an ending reaches it.
type Session struct {
	events  textInputEvents
	session *session
}

// NewSession opens a session.
func NewSession() *Session {
	s := &Session{}
	s.StartNext()
	return s
}

// StartNext opens the next session, as the application does after the
// previous one ended.
func (s *Session) StartNext() {
	ch, end := s.events.start()
	s.session = &session{ch: ch, end: end, events: &s.events}
}

// Send hands the session a state, as the platform does.
func (s *Session) Send(state TextInputState) { s.events.send(state) }

// EndByUser ends the events as the platform does for the user's dismissal.
func (s *Session) EndByUser() { s.events.endByUser() }

// End ends the events as the platform does for its own teardown.
func (s *Session) End() { s.events.end() }

// Update drains the session's states, as a tick does.
func (s *Session) Update() error { return s.session.Update() }

// Closed reports whether the session is closed.
func (s *Session) Closed() bool { return s.session.IsClosed() }

// ClosedByUser reports whether the session recorded the user's ending.
func (s *Session) ClosedByUser() bool { return s.session.IsClosedByUser() }

// Commit returns the commit the session observed, or nil if it observed none.
func (s *Session) Commit() *Commit {
	if !s.session.IsCommitted() {
		return nil
	}
	return s.session.Commit()
}
