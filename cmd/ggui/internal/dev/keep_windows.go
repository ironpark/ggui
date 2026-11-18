package dev

import (
	"crypto/rand"
	"errors"
	"net"
	"os"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows the keeper listens on a named pipe rather than a unix socket:
// every Windows has pipes, where unix sockets came with Windows 10 1803,
// and one path on the system is simpler to trust than a fallback that the
// machines tests run on would never take. The pipe speaks what the socket
// does; ggui's devstate dials it when the address it is given is a pipe's
// name.
//
// The pipe is the session's alone. Its name ends in 128 random bits, so
// nothing else guesses it; its security descriptor allows the user running
// ggui dev and nobody else, to open it or to make instances of it, so
// another user's process can neither read the state nor answer in ggui
// dev's place; FILE_FLAG_FIRST_PIPE_INSTANCE fails the listen should the
// name exist already, and PIPE_REJECT_REMOTE_CLIENTS refuses a client
// from another machine.
//
// The pipe is opened for overlapped I/O, so that nothing on it blocks for
// good: waiting for a client wakes when the listener closes, and once a
// client connects the pipe is an *os.File on Go's poller, whose deadlines
// and Close end a read or write that waits.

// pipePrefix begins every named pipe's name.
const pipePrefix = `\\.\pipe\`

// listenKeep listens on a named pipe of the session's own.
func listenKeep() (keepListener, string, func(), error) {
	name := pipePrefix + "ggui-dev-" + rand.Text()
	l, err := newPipeListener(name)
	if err != nil {
		return nil, "", nil, err
	}
	return l, name, func() {}, nil
}

// pipeListener takes connections on a named pipe, one instance of the pipe
// a client: the instance a client connected to becomes its connection, and
// another is made for the next.
type pipeListener struct {
	name     string
	path     *uint16
	sa       *windows.SecurityAttributes
	stop     windows.Handle // an event Close sets, which wakes a waiting Accept
	done     chan struct{}  // closed once Accept has returned for good
	doneOnce sync.Once

	mu     sync.Mutex
	closed bool
	next   windows.Handle // the instance the next client connects to: Accept's
	err    error          // why there is no next instance

	// ov is the pending connect's OVERLAPPED. The system writes to it until
	// the connect completes or is cancelled, so it lives on the heap,
	// reachable from here, rather than on a stack that may move.
	ov *windows.Overlapped
}

func newPipeListener(name string) (*pipeListener, error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	// Protected, so nothing is inherited, and generic access for the user
	// alone: no Everyone, no anonymous logon, as a pipe's default has.
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return nil, err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, err
	}
	l := &pipeListener{name: name, path: path, sa: sa, stop: stop, done: make(chan struct{})}
	if l.next, err = l.instance(true); err != nil {
		windows.CloseHandle(stop)
		return nil, err
	}
	return l, nil
}

// instance makes an instance of the pipe; the first must be the first of
// its name.
func (l *pipeListener) instance(first bool) (windows.Handle, error) {
	flags := uint32(windows.PIPE_ACCESS_DUPLEX | windows.FILE_FLAG_OVERLAPPED)
	if first {
		flags |= windows.FILE_FLAG_FIRST_PIPE_INSTANCE
	}
	mode := uint32(windows.PIPE_TYPE_BYTE | windows.PIPE_READMODE_BYTE | windows.PIPE_WAIT | windows.PIPE_REJECT_REMOTE_CLIENTS)
	return windows.CreateNamedPipe(l.path, flags, mode, windows.PIPE_UNLIMITED_INSTANCES, 64<<10, 64<<10, 0, l.sa)
}

// Accept waits for a client and returns its connection.
func (l *pipeListener) Accept() (keepStream, error) {
	for {
		h, err := l.connect()
		if err != nil {
			l.finish()
			return nil, err
		}
		if h != 0 {
			return os.NewFile(uintptr(h), l.name), nil
		}
	}
}

// connect waits on the next instance for a client and returns that
// instance, connected, having made another for the client after. It
// returns 0 for a client gone before it was taken, whose instance is ready
// for another.
func (l *pipeListener) connect() (windows.Handle, error) {
	l.mu.Lock()
	h, closed, err := l.next, l.closed, l.err
	l.mu.Unlock()
	switch {
	case closed:
		return 0, net.ErrClosed
	case h == 0:
		return 0, err
	}
	ev, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(ev)
	l.ov = &windows.Overlapped{HEvent: ev}
	defer func() { l.ov = nil }()
	var n uint32
	switch err := windows.ConnectNamedPipe(h, l.ov); err {
	case nil, windows.ERROR_PIPE_CONNECTED:
	case windows.ERROR_NO_DATA:
		// A client that connected and closed already.
		windows.DisconnectNamedPipe(h)
		return 0, nil
	case windows.ERROR_IO_PENDING:
		w, err := windows.WaitForMultipleObjects([]windows.Handle{ev, l.stop}, false, windows.INFINITE)
		if err != nil || w != windows.WAIT_OBJECT_0 {
			// Closed: the connect is cancelled, and waited for, before its
			// OVERLAPPED and event go.
			windows.CancelIoEx(h, l.ov)
			windows.GetOverlappedResult(h, l.ov, &n, true)
			if err == nil {
				err = net.ErrClosed
			}
			return 0, err
		}
		if windows.GetOverlappedResult(h, l.ov, &n, false) != nil {
			windows.DisconnectNamedPipe(h)
			return 0, nil
		}
	default:
		return 0, err
	}
	next, err := l.instance(false)
	l.mu.Lock()
	l.next, l.err = next, err
	if err != nil {
		l.next = 0
	}
	l.mu.Unlock()
	return h, nil
}

// finish closes the instance no client will connect to now, and says that
// Accept is done.
func (l *pipeListener) finish() {
	l.mu.Lock()
	h := l.next
	l.next = 0
	if l.err == nil {
		l.err = net.ErrClosed
	}
	l.mu.Unlock()
	if h != 0 {
		windows.CloseHandle(h)
	}
	l.doneOnce.Do(func() { close(l.done) })
}

// closeWait bounds how long Close waits for Accept to see it: a moment,
// since the wait it wakes returns at once.
const closeWait = 5 * time.Second

// Close stops the listener: a waiting Accept returns net.ErrClosed, and so
// does every later one. The connections it returned stay open.
func (l *pipeListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	l.mu.Unlock()
	windows.SetEvent(l.stop)
	select {
	case <-l.done:
		windows.CloseHandle(l.stop)
	case <-time.After(closeWait):
		// Accept never came back to see it; the event stays open rather
		// than close under a wait that may still use it.
		return errors.New("the pipe's Accept did not return")
	}
	return nil
}
