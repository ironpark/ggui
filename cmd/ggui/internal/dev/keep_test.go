package dev

import (
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// keepModule writes a module whose program stands in for an app that keeps
// a count, speaking ggui dev's channel as ggui's devstate does. It says
// what it was handed, then counts one more, as a click would, and hands
// that over when asked. The message it is built with changes it:
//
//	"hang"   never answers the request for its state
//	"exit"   exits with status 3 once it said what it was handed
//	"text"   keeps its count as text, as a build whose type changed would
//
// A build of another message that is handed a count it cannot decode, as
// after "text", says dropped and counts from 0.
func keepModule(t *testing.T, msg string) string {
	t.Helper()
	dir := module(t, msg)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"app/msg"
)

type message struct {
	Type     string                     `+"`json:\"type\"`"+`
	Instance string                     `+"`json:\"instance,omitempty\"`"+`
	State    map[string]json.RawMessage `+"`json:\"state,omitempty\"`"+`
}

func main() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM, os.Interrupt)
	id, addr, _ := strings.Cut(os.Getenv("GGUI_DEV_STATE"), ":")
	// On Windows the channel is a named pipe, which a plain open reaches.
	var conn io.ReadWriter
	var err error
	if strings.HasPrefix(addr, `+"`\\\\.\\pipe\\`"+`) {
		conn, err = os.OpenFile(addr, os.O_RDWR, 0)
	} else {
		conn, err = net.Dial("unix", addr)
	}
	if err != nil {
		fmt.Println(msg.Text, "no channel:", err)
		<-c
		return
	}
	enc, dec := json.NewEncoder(conn), json.NewDecoder(conn)
	enc.Encode(message{Type: "hello", Instance: id})
	var m message
	dec.Decode(&m)
	handed := string(m.State["n"])
	var n int
	var text string
	switch {
	case handed == "":
	case msg.Text == "text":
		if json.Unmarshal(m.State["n"], &n) != nil {
			json.Unmarshal(m.State["n"], &text)
		}
	case json.Unmarshal(m.State["n"], &n) != nil:
		fmt.Println(msg.Text, "dropped", handed)
	}
	n++
	fmt.Printf("%s handed n=%s\n", msg.Text, handed)
	if msg.Text == "exit" {
		os.Exit(3)
	}
	go func() {
		for {
			var r message
			if dec.Decode(&r) != nil {
				return
			}
			if msg.Text == "hang" {
				continue
			}
			v, _ := json.Marshal(n)
			if msg.Text == "text" {
				v, _ = json.Marshal(fmt.Sprint("count ", n))
			}
			enc.Encode(message{Type: "saved", State: map[string]json.RawMessage{"n": v}})
		}
	}()
	<-c
}
`), 0o644)
	return dir
}

// keepNative runs dir's app under ggui dev, giving it handWait to hand
// over what it keeps.
func keepNative(t *testing.T, dir string, handWait time.Duration) (out, log *syncBuffer, stop func()) {
	t.Helper()
	out, log = &syncBuffer{}, &syncBuffer{}
	n := &Native{
		Dir: dir, Package: ".", Watch: true,
		StopTimeout: 5 * time.Second, handWait: handWait,
		Log: log, Stdout: out, Stderr: out,
	}
	cancel, done := runNative(t, n)
	return out, log, func() {
		cancel()
		if err := wait(t, done); err != nil {
			t.Errorf("Run = %v", err)
		}
	}
}

// Each restart hands the new app what the old one kept; a failed build in
// between leaves the running app, and what it keeps, alone; an app that
// crashed is followed by one handed what the crashed one was.
func TestKeepAcrossRestarts(t *testing.T) {
	dir := keepModule(t, "v1")
	out, log, stop := keepNative(t, dir, 2*time.Second)
	defer stop()

	waitFor(t, out, "v1 handed n=\n")
	writeMsg(t, dir, "v2")
	waitFor(t, out, "v2 handed n=1\n")

	writeMsg(t, dir, "v3 broken\"")
	waitFor(t, log, "the app keeps running the last build that succeeded")
	writeMsg(t, dir, "v3")
	waitFor(t, out, "v3 handed n=2\n")

	writeMsg(t, dir, "exit")
	waitFor(t, out, "exit handed n=3\n")
	waitFor(t, log, "waiting for a change")
	writeMsg(t, dir, "v4")
	waitFor(t, out, "v4 handed n=3\n")
}

// An app that does not answer is not waited for beyond the timeout, and
// the next one starts with nothing.
func TestKeepNoAnswer(t *testing.T) {
	dir := keepModule(t, "v1")
	out, log, stop := keepNative(t, dir, 300*time.Millisecond)
	defer stop()

	waitFor(t, out, "v1 handed n=\n")
	writeMsg(t, dir, "hang")
	waitFor(t, out, "hang handed n=1\n")
	writeMsg(t, dir, "v2")
	waitFor(t, out, "v2 handed n=\n")
	if !strings.Contains(log.String(), "did not hand over the state it keeps within 300ms") {
		t.Errorf("no word of the app that did not answer:\n%s", log.String())
	}
}

// A kept value whose type changed is handed over as it was, for the new
// build to decode or drop; what that build keeps then is handed on.
func TestKeepTypeChange(t *testing.T) {
	dir := keepModule(t, "v1")
	out, _, stop := keepNative(t, dir, 2*time.Second)
	defer stop()

	waitFor(t, out, "v1 handed n=\n")
	writeMsg(t, dir, "text")
	waitFor(t, out, "text handed n=1\n")
	writeMsg(t, dir, "v2")
	waitFor(t, out, `v2 dropped "count 2"`)
	waitFor(t, out, `v2 handed n="count 2"`)
	writeMsg(t, dir, "v3")
	waitFor(t, out, "v3 handed n=1\n")
}

// dialKeep connects to a keeper as an app does: to a named pipe on
// Windows, which a plain open reaches, and to a unix socket elsewhere. A
// connection is closed after ten seconds, so that a test waiting on an
// answer that never comes fails rather than hangs.
func dialKeep(t *testing.T, addr string) io.ReadWriteCloser {
	t.Helper()
	var c io.ReadWriteCloser
	var err error
	if strings.HasPrefix(addr, `\\.\pipe\`) {
		c, err = os.OpenFile(addr, os.O_RDWR, 0)
	} else {
		c, err = net.Dial("unix", addr)
	}
	if err != nil {
		t.Fatal(err)
	}
	timer := time.AfterFunc(10*time.Second, func() { c.Close() })
	t.Cleanup(func() {
		timer.Stop()
		c.Close()
	})
	return c
}

// A hello from another instance than the one started last, as from a
// process the app started with its environment, is answered with nothing.
func TestKeepAnswersOnlyTheInstance(t *testing.T) {
	k, err := newKeeper()
	if err != nil {
		t.Fatalf("no channel: %v", err)
	}
	defer k.close()
	k.last = map[string]json.RawMessage{"n": json.RawMessage("7")}
	env := k.handTo()
	_, rest, _ := strings.Cut(env, "=")
	id, addr, _ := strings.Cut(rest, ":")

	hello := func(id string) keepMsg {
		c := dialKeep(t, addr)
		json.NewEncoder(c).Encode(keepMsg{Type: "hello", Instance: id})
		var m keepMsg
		if err := json.NewDecoder(c).Decode(&m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if m := hello("other"); m.Type != "state" || m.State != nil {
		t.Errorf("another instance was answered %+v, want a state with nothing", m)
	}
	if m := hello(id); string(m.State["n"]) != "7" {
		t.Errorf("the instance was answered %+v, want n=7", m)
	}
	if m := hello(id); m.State != nil {
		t.Errorf("a second hello of the instance was answered %+v, want nothing", m)
	}
}

// Shown by a viewer, the app is handed its state as in its own window,
// also across a restart of the viewer.
func TestKeepWithViewer(t *testing.T) {
	dir := keepModule(t, "v1")
	d := &fakeDisplay{}
	var out, log syncBuffer
	n := &Native{Dir: dir, Package: ".", Watch: true, Display: d, StopTimeout: 5 * time.Second, Log: &log, Stdout: &out, Stderr: &out}
	cancel, done := runNative(t, n)
	defer func() {
		cancel()
		wait(t, done)
	}()
	waitFor(t, &out, "v1 handed n=\n")
	writeMsg(t, dir, "v2")
	waitFor(t, &out, "v2 handed n=1\n")
	d.mu.Lock()
	d.stale = true
	d.mu.Unlock()
	writeMsg(t, dir, "v3")
	waitFor(t, &out, "v3 handed n=2\n")
	if d.viewer(1) == nil {
		t.Error("the viewer was not restarted")
	}
}
