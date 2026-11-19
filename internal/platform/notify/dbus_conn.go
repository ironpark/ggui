//go:build (linux && !android) || freebsd || netbsd || openbsd || dragonfly

package notify

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// busConn is a connection to a message bus: calls and their replies, and
// the signals and calls the bus routes to it, which a goroutine of its own
// reads and hands to the onMessage it was dialled with.
type busConn struct {
	c         net.Conn
	r         *bufio.Reader
	wmu       sync.Mutex
	serial    atomic.Uint32
	onMessage func(*message)

	mu    sync.Mutex
	calls map[uint32]chan *message
	err   error // why the connection ended, once it has
	done  chan struct{}
}

// callTimeout is how long a call waits for its reply. The bus starts a
// service that is not running when it is first called, which may take a
// while, and a service that never answers must not hold the posting
// goroutine for ever.
const callTimeout = 10 * time.Second

// dialBus connects to the bus at path, authenticates as the user and says
// Hello, as the specification asks of a client before anything else.
func dialBus(path string, onMessage func(*message)) (*busConn, error) {
	c, err := net.DialTimeout("unix", path, callTimeout)
	if err != nil {
		return nil, err
	}
	b := &busConn{c: c, r: bufio.NewReader(c), onMessage: onMessage, calls: map[uint32]chan *message{}, done: make(chan struct{})}
	if err := b.auth(); err != nil {
		c.Close()
		return nil, err
	}
	go b.read()
	if _, err := b.call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "Hello", ""); err != nil {
		b.close(err)
		return nil, err
	}
	return b, nil
}

// auth authenticates with EXTERNAL, the credentials the kernel passes
// along the socket, which is what a session bus accepts.
func (b *busConn) auth() error {
	_ = b.c.SetDeadline(time.Now().Add(callTimeout))
	defer b.c.SetDeadline(time.Time{})
	uid := hex.EncodeToString([]byte(strconv.Itoa(os.Getuid())))
	if _, err := b.c.Write([]byte("\x00AUTH EXTERNAL " + uid + "\r\n")); err != nil {
		return err
	}
	line, err := b.r.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(line, "OK ") {
		return fmt.Errorf("dbus: the bus refused to authenticate: %s", strings.TrimSpace(line))
	}
	_, err = b.c.Write([]byte("BEGIN\r\n"))
	return err
}

// send writes m with the next serial, after register, if any, has been
// handed that serial: a reply may be read before the write returns.
func (b *busConn) send(m *message, register func(serial uint32)) error {
	b.wmu.Lock()
	defer b.wmu.Unlock()
	serial := b.serial.Add(1)
	data, err := m.marshal(serial)
	if err != nil {
		return err
	}
	if register != nil {
		register(serial)
	}
	_ = b.c.SetWriteDeadline(time.Now().Add(callTimeout))
	_, err = b.c.Write(data)
	return err
}

// call calls a method and waits for its reply, an error reply returned as
// an error.
func (b *busConn) call(dest, path, iface, member, sig string, args ...any) (*message, error) {
	reply := make(chan *message, 1)
	if err := b.closedErr(); err != nil {
		return nil, err
	}
	var serial uint32
	m := &message{typ: msgCall, dest: dest, path: path, iface: iface, member: member, sig: sig, body: args}
	err := b.send(m, func(s uint32) {
		serial = s
		b.mu.Lock()
		b.calls[s] = reply
		b.mu.Unlock()
	})
	if err != nil {
		b.mu.Lock()
		delete(b.calls, serial)
		b.mu.Unlock()
		return nil, err
	}
	select {
	case r := <-reply:
		if r.typ == msgError {
			msg := ""
			if len(r.body) > 0 {
				msg, _ = r.body[0].(string)
			}
			return nil, fmt.Errorf("%s: %s", r.errName, msg)
		}
		return r, nil
	case <-b.done:
		return nil, b.closedErr()
	case <-time.After(callTimeout):
		b.mu.Lock()
		delete(b.calls, serial)
		b.mu.Unlock()
		return nil, fmt.Errorf("dbus: %s.%s had no reply in %s", iface, member, callTimeout)
	}
}

// read reads every message until the connection ends, handing replies to
// their calls and the rest to onMessage.
func (b *busConn) read() {
	for {
		m, err := readMessage(b.r)
		if err != nil {
			b.close(err)
			return
		}
		switch m.typ {
		case msgReturn, msgError:
			b.mu.Lock()
			reply := b.calls[m.replySerial]
			delete(b.calls, m.replySerial)
			b.mu.Unlock()
			if reply != nil {
				reply <- m
			}
		case msgSignal, msgCall:
			if b.onMessage != nil {
				b.onMessage(m)
			}
		}
	}
}

// close ends the connection for err, the first reason given.
func (b *busConn) close(err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.err != nil {
		return
	}
	b.err = err
	b.c.Close()
	close(b.done)
}

// closedErr is why the connection ended.
func (b *busConn) closedErr() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}
