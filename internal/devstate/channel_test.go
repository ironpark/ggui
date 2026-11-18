//go:build !(js && wasm)

package devstate

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The app's end of the channel against a stand-in for ggui dev: the hello
// names the instance, the answer is the state handed over, and a request
// is answered with what is kept, read through Dispatch.
func TestChannel(t *testing.T) {
	dir, err := os.MkdirTemp("", "ggui-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	sock := filepath.Join(dir, "k.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Skipf("no unix socket here: %v", err)
	}
	defer ln.Close()
	t.Setenv(Env, "4:"+sock)

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
		c, err := ln.Accept()
		if a.err = err; err != nil {
			return
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(10 * time.Second))
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

	a := <-got
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

// Without the variable there is no session, and nothing is dialled.
func TestNoChannel(t *testing.T) {
	t.Setenv(Env, "")
	if s := connect(); s != nil {
		t.Error("a session without ggui dev")
	}
}
