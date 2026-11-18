package devstate

import (
	"crypto/rand"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// listenTest makes a named pipe, as ggui dev does on Windows, and returns
// its name and a function that takes the connection to it. The stand-in
// serves one client, on a pipe opened for plain blocking I/O, which is all
// a test's server needs.
func listenTest(t *testing.T) (string, func() (io.ReadWriteCloser, error)) {
	t.Helper()
	name := pipePrefix + "ggui-dev-test-" + rand.Text()
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateNamedPipe(path, windows.PIPE_ACCESS_DUPLEX,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, 64<<10, 64<<10, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	f := os.NewFile(uintptr(h), name)
	t.Cleanup(func() { f.Close() })
	return name, func() (io.ReadWriteCloser, error) {
		if err := windows.ConnectNamedPipe(h, nil); err != nil && err != windows.ERROR_PIPE_CONNECTED {
			return nil, err
		}
		return f, nil
	}
}

// A pipe that does not exist fails at once rather than after the timeout:
// an app whose ggui dev is gone starts without waiting.
func TestDialMissingPipe(t *testing.T) {
	start := time.Now()
	c, err := dial(pipePrefix+"ggui-dev-test-missing-"+rand.Text(), 5*time.Second)
	if err == nil {
		c.Close()
		t.Fatal("dialled a pipe that does not exist")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("dialling a missing pipe took %s", d)
	}
}

// The pipe dial opens is on Go's poller, so a deadline ends a read that
// waits: what bounds the hello, and every wait after it.
func TestDialPipeDeadline(t *testing.T) {
	addr, accept := listenTest(t)
	c, err := dial(addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := accept(); err != nil {
		t.Fatal(err)
	}
	if err := c.SetDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatalf("the pipe takes no deadline: %v", err)
	}
	var b [1]byte
	_, err = c.Read(b[:])
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("a read past the deadline = %v, want os.ErrDeadlineExceeded", err)
	}
}

// Close ends a read that waits on the pipe.
func TestDialPipeCloseEndsRead(t *testing.T) {
	addr, accept := listenTest(t)
	c, err := dial(addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accept(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		var b [1]byte
		_, err := c.Read(b[:])
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	c.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Error("a read on a closed pipe returned no error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not end the read")
	}
}
