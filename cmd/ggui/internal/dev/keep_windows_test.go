package dev

import (
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

// On Windows the keeper listens on a named pipe of the session's own.
func TestKeepListensOnAPipe(t *testing.T) {
	a, err := newKeeper()
	if err != nil {
		t.Fatal(err)
	}
	defer a.close()
	b, err := newKeeper()
	if err != nil {
		t.Fatal(err)
	}
	defer b.close()
	if !strings.HasPrefix(a.addr, `\\.\pipe\ggui-dev-`) {
		t.Errorf("the keeper listens on %q, not a named pipe", a.addr)
	}
	if a.addr == b.addr {
		t.Errorf("two sessions share the pipe %q", a.addr)
	}
}

func testPipeListener(t *testing.T) *pipeListener {
	t.Helper()
	l, err := newPipeListener(pipePrefix + "ggui-dev-test-" + rand.Text())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	return l
}

// accepted is what an Accept returned.
type accepted struct {
	c   keepStream
	err error
}

// acceptAsync starts l.Accept.
func acceptAsync(l *pipeListener) <-chan accepted {
	ch := make(chan accepted, 1)
	go func() {
		c, err := l.Accept()
		ch <- accepted{c, err}
	}()
	return ch
}

// await takes an Accept's connection, failing rather than hanging when it
// does not come.
func await(t *testing.T, ch <-chan accepted) keepStream {
	t.Helper()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatal(r.err)
		}
		t.Cleanup(func() { r.c.Close() })
		return r.c
	case <-time.After(10 * time.Second):
		t.Fatal("Accept did not return")
		return nil
	}
}

// dialPipe opens the pipe as a client, trying again while every instance
// is taken, as the app's dial does.
func dialPipe(t *testing.T, name string) *os.File {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		f, err := os.OpenFile(name, os.O_RDWR, 0)
		if err == nil {
			t.Cleanup(func() { f.Close() })
			return f
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A second listener cannot take a name in use: nothing else can stand in
// for ggui dev on its pipe.
func TestPipeFirstInstance(t *testing.T) {
	l := testPipeListener(t)
	if other, err := newPipeListener(l.name); err == nil {
		other.Close()
		t.Error("a second listener took the pipe's name")
	}
}

// Each client gets an instance of its own, also after one that connected
// and went before it was taken, and the connection carries bytes both
// ways, with deadlines.
func TestPipeServesClients(t *testing.T) {
	l := testPipeListener(t)
	gone := dialPipe(t, l.name)
	gone.Close()
	for i := range 3 {
		ch := acceptAsync(l)
		c := dialPipe(t, l.name)
		s := await(t, ch)
		msg := []byte{'a' + byte(i)}
		if _, err := c.Write(msg); err != nil {
			t.Fatal(err)
		}
		s.SetReadDeadline(time.Now().Add(5 * time.Second))
		b := make([]byte, 1)
		if _, err := io.ReadFull(s, b); err != nil || b[0] != msg[0] {
			t.Fatalf("client %d: read %q, %v", i, b, err)
		}
		if _, err := s.Write([]byte("ok")); err != nil {
			t.Fatal(err)
		}
		b = make([]byte, 2)
		if _, err := io.ReadFull(c, b); err != nil || string(b) != "ok" {
			t.Fatalf("client %d: answered %q, %v", i, b, err)
		}
		if i == 0 {
			// A read the client never answers ends at the deadline.
			s.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if _, err := s.Read(b); !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Errorf("a read past the deadline = %v, want os.ErrDeadlineExceeded", err)
			}
		}
	}
}

// Close wakes an Accept waiting for a client, which returns net.ErrClosed,
// and so does every Accept after.
func TestPipeCloseEndsAccept(t *testing.T) {
	l := testPipeListener(t)
	ch := acceptAsync(l)
	time.Sleep(100 * time.Millisecond)
	if err := l.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
	select {
	case r := <-ch:
		if !errors.Is(r.err, net.ErrClosed) {
			t.Errorf("Accept after Close = %v, want net.ErrClosed", r.err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not wake Accept")
	}
	if _, err := l.Accept(); !errors.Is(err, net.ErrClosed) {
		t.Errorf("a later Accept = %v, want net.ErrClosed", err)
	}
}
