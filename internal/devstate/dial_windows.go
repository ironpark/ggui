package devstate

import (
	"net"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// pipePrefix begins every named pipe's name.
const pipePrefix = `\\.\pipe\`

// dial connects to ggui dev's channel: on Windows a named pipe, which ggui
// dev listens on there, or a unix socket when the address is not a pipe's
// name.
//
// The pipe is opened for overlapped I/O, so that the *os.File made of it
// is on Go's poller: its deadlines bound the hello, and Close ends a read
// that waits. Opening never waits: a pipe whose every instance is taken
// is tried again until timeout, and one that does not exist fails at once.
// The open asks for identification alone, so the end that answers, which
// should be ggui dev, cannot act as this user.
func dial(addr string, timeout time.Duration) (stream, error) {
	if !strings.HasPrefix(addr, pipePrefix) {
		return net.DialTimeout("unix", addr, timeout)
	}
	name, err := windows.UTF16PtrFromString(addr)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
			windows.FILE_FLAG_OVERLAPPED|windows.SECURITY_SQOS_PRESENT|windows.SECURITY_IDENTIFICATION, 0)
		if err == nil {
			return os.NewFile(uintptr(h), addr), nil
		}
		if err != windows.ERROR_PIPE_BUSY || time.Now().After(deadline) {
			return nil, &os.PathError{Op: "open", Path: addr, Err: err}
		}
		time.Sleep(10 * time.Millisecond)
	}
}
