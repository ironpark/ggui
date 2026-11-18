//go:build !(js && wasm)

package devstate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// The tests here stand in for ggui dev on the channel the system's ggui
// dev listens on, which listenTest makes: a unix socket, or on Windows a
// named pipe.

// The app's end of the channel against a stand-in for ggui dev: the hello
// names the instance, the answer is the state handed over, and a request
// is answered with what is kept, read through Dispatch.
func TestChannel(t *testing.T) {
	addr, accept := listenTest(t)
	t.Setenv(Env, "4:"+addr)

	type answer struct {
		hello message
		saved message
		err   error
	}
	got := make(chan answer, 1)
	kept := make(chan struct{})
	go func() {
		var a answer
		defer func() { got <- a }()
		c, err := accept()
		if a.err = err; err != nil {
			return
		}
		defer c.Close()
		enc, dec := json.NewEncoder(c), json.NewDecoder(c)
		if a.err = dec.Decode(&a.hello); a.err != nil {
			return
		}
		enc.Encode(message{Type: "state", State: State{"n": json.RawMessage("41")}})
		<-kept
		enc.Encode(message{Type: "save"})
		a.err = dec.Decode(&a.saved)
	}()

	dispatched := make(chan bool, 1)
	Dispatch = func(fn func()) bool { dispatched <- true; fn(); return true }
	defer func() { Dispatch = nil }()
	s := connect()
	if s == nil {
		t.Fatal("no session with ggui dev's channel in the environment")
	}
	n := 0
	s.mu.Lock()
	handed := string(s.handed["n"])
	s.mu.Unlock()
	if handed != "41" {
		t.Fatalf("handed n=%s, want 41", handed)
	}
	// What Keep does, by hand: claim the value and keep one more.
	s.mu.Lock()
	json.Unmarshal(s.handed["n"], &n)
	delete(s.handed, "n")
	s.entries["n"] = &entry{save: func() ([]byte, error) { return json.Marshal(n + 1) }}
	s.mu.Unlock()
	close(kept)

	var a answer
	select {
	case a = <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("the stand-in for ggui dev got no answer")
	}
	if a.err != nil {
		t.Fatal(a.err)
	}
	if a.hello.Type != "hello" || a.hello.Instance != "4" {
		t.Errorf("hello = %+v, want instance 4", a.hello)
	}
	if a.saved.Type != "saved" || string(a.saved.State["n"]) != "42" {
		t.Errorf("saved = %+v, want n=42", a.saved)
	}
	select {
	case <-dispatched:
	default:
		t.Error("the state was not read through Dispatch")
	}
}

// A ggui dev that takes the connection and never answers holds the app's
// start no longer than connectTimeout, and the app keeps nothing.
func TestChannelNoAnswer(t *testing.T) {
	addr, accept := listenTest(t)
	t.Setenv(Env, "5:"+addr)
	prev := connectTimeout
	connectTimeout = 300 * time.Millisecond
	defer func() { connectTimeout = prev }()
	var errs bytes.Buffer
	prevErr := Stderr
	Stderr = &errs
	defer func() { Stderr = prevErr }()
	reported.Delete("channel")
	held := make(chan struct{})
	defer close(held)
	go func() {
		if c, err := accept(); err == nil {
			<-held
			c.Close()
		}
	}()

	start := time.Now()
	if s := connect(); s != nil {
		t.Error("a session with a ggui dev that never answered")
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("connect waited %s for an answer, past connectTimeout", d)
	}
	if !strings.Contains(errs.String(), "did not hand state over") {
		t.Errorf("stderr = %q, want word that no state is kept", errs.String())
	}
}

// Without the variable there is no session, and nothing is dialled.
func TestNoChannel(t *testing.T) {
	t.Setenv(Env, "")
	if s := connect(); s != nil {
		t.Error("a session without ggui dev")
	}
}
