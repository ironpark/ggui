//go:build !(js && wasm)

package devstate

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"time"
)

// connectTimeout bounds the hello and its answer. ggui dev answers at once
// from a goroutine of its own; the bound is for a ggui dev that is gone
// and a channel nobody serves, which must not hold the app's start. A
// variable, for tests.
var connectTimeout = 2 * time.Second

// dispatchTimeout bounds how long onUI waits for a snapshot posted to the
// UI thread when the app or window shuts down before running it. A
// variable, for tests.
var dispatchTimeout = 2 * time.Second

// stream is a connection to ggui dev: a unix socket's, or a named pipe's
// file on Go's poller.
type stream interface {
	io.ReadWriteCloser
	SetDeadline(time.Time) error
}

// Dispatch, when set, runs fn on the UI thread and reports whether it
// will: the package ggui sets it to post to the running App. The kept
// values are read there, between frames, rather than on the channel's
// goroutine while a handler may be halfway through changing them.
var Dispatch func(fn func()) bool

// connect says hello to ggui dev, when it started the app, and takes what
// it hands over. A channel that fails is said once, and the app runs on
// keeping nothing.
func connect() *session {
	v := os.Getenv(Env)
	if v == "" {
		return nil
	}
	id, addr, ok := strings.Cut(v, ":")
	if !ok {
		reportf("channel", "ggui: %s=%q is not ggui dev's; state is not kept across restarts", Env, v)
		return nil
	}
	c, err := dial(addr, connectTimeout)
	if err != nil {
		reportf("channel", "ggui: no channel to ggui dev (%v); state is not kept across restarts", err)
		return nil
	}
	// The deadline bounds the hello; the timer is there should the channel
	// take no deadline, and closing the channel ends the wait as surely.
	c.SetDeadline(time.Now().Add(connectTimeout))
	guard := time.AfterFunc(connectTimeout, func() { c.Close() })
	enc, dec := json.NewEncoder(c), json.NewDecoder(c)
	var m message
	if err = enc.Encode(message{Type: "hello", Instance: id}); err == nil {
		err = dec.Decode(&m)
	}
	if !guard.Stop() && err == nil {
		err = os.ErrDeadlineExceeded
	}
	if err != nil || m.Type != "state" {
		c.Close()
		reportf("channel", "ggui: ggui dev did not hand state over (%v); state is not kept across restarts", err)
		return nil
	}
	c.SetDeadline(time.Time{})
	s := newSession(m.State)
	go s.serve(c, enc, dec)
	return s
}

// serve answers ggui dev's requests until it closes the channel, as it
// does when it hands this app's state to another instance.
func (s *session) serve(c stream, enc *json.Encoder, dec *json.Decoder) {
	defer func() {
		s.close()
		c.Close()
	}()
	for {
		var m message
		if err := dec.Decode(&m); err != nil {
			return
		}
		if m.Type != "save" {
			continue
		}
		st, ok := s.onUI()
		if !ok {
			return
		}
		if enc.Encode(message{Type: "saved", State: st}) != nil {
			return
		}
	}
}

// onUI takes the snapshot on the UI thread. Before the app runs there is
// no UI thread to post to and the snapshot is taken here; if a posted
// callback never runs because the app shut down or the UI thread is stuck,
// onUI stops waiting once the session closes or dispatchTimeout elapses.
func (s *session) onUI() (State, bool) {
	done := make(chan State, 1)
	d := Dispatch
	if d == nil || !d(func() { done <- s.snapshot() }) {
		return s.snapshot(), true
	}
	timer := time.NewTimer(dispatchTimeout)
	defer timer.Stop()
	select {
	case st := <-done:
		return st, true
	case <-s.closed:
		return nil, false
	case <-timer.C:
		return nil, false
	}
}
