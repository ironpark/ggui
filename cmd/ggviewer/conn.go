//go:build (unix && !ios && !android) || windows

package main

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
)

// handshakeTimeout is how long an app that connected has to say hello
// and describe its window.
const handshakeTimeout = 5 * time.Second

// pendingBatch pairs a received batch with the shared memory mapping that was
// active when it arrived, so replacing shm does not corrupt or unmap memory
// still needed by queued batches.
type pendingBatch struct {
	batch *remote.Batch
	shm   []byte
}

// app is an app process attached to the viewer.
type app struct {
	id   int
	pid  int
	conn *remote.Conn
	opts remote.OpenWindow

	mu      sync.Mutex
	shm     []byte
	oldShm  [][]byte // replaced, to unmap on the main thread after replay
	pending []pendingBatch

	// shown reports that a frame of the app's was shown. Main thread only.
	shown bool
}

// handshake welcomes an app and reads its hello and its window.
func (v *session) handshake(c *net.UnixConn) (*app, error) {
	_ = c.SetDeadline(time.Now().Add(handshakeTimeout))
	conn := remote.NewConn(c)
	if err := conn.Send(&remote.Welcome{Version: remote.Version, Driver: v.info}); err != nil {
		return nil, err
	}
	m, err := conn.Receive()
	if err != nil {
		return nil, err
	}
	h, ok := m.(*remote.Hello)
	if !ok {
		return nil, fmt.Errorf("an app said %T before hello", m)
	}
	if h.Version != remote.Version {
		// The app refuses the viewer too, and says why.
		return nil, fmt.Errorf("app %d speaks protocol version %d, the viewer %d: build the viewer and the app with the same ggwd", h.PID, h.Version, remote.Version)
	}
	if m, err = conn.Receive(); err != nil {
		return nil, err
	}
	o, ok := m.(*remote.OpenWindow)
	if !ok {
		return nil, fmt.Errorf("app %d said %T before its window", h.PID, m)
	}
	_ = c.SetDeadline(time.Time{})
	return &app{pid: h.PID, conn: conn, opts: *o}, nil
}

// attach makes a the app shown, in place of the one before. When applyOpts
// is true (every app after the one that opened the window), a's window
// options are queued to be applied live on the main thread.
func (v *session) attach(a *app, applyOpts bool) {
	v.mu.Lock()
	v.nextID++
	a.id = v.nextID
	old := v.cur
	v.cur, v.closeAsked = a, false
	v.apps[a] = struct{}{}
	// What the app before asked of the window is not this one's.
	v.resetRequests(old != nil)
	if applyOpts {
		o := a.opts
		v.req.options = &o
	}
	cfg, have := v.cfg, v.haveCfg
	v.mu.Unlock()
	if old != nil {
		_ = old.conn.Close()
	}
	if have {
		_ = a.conn.Send(&cfg)
	}
	v.events.send(event{Type: evAppAttached, PID: a.pid})
	go v.read(a)
}

// resetRequests clears any pending app-originated requests and, when reset
// is true, asks the main thread to reset per-app viewer state (cursor, text
// input, icon, color mode, dock icon, menu bar). Launcher control-channel
// requests are preserved. v.mu is held.
func (v *session) resetRequests(reset bool) {
	v.req = requests{
		reset:   v.req.reset || reset,
		actions: v.req.actions,
		inputs:  v.req.inputs,
		dumps:   v.req.dumps,
		close:   v.req.close,
	}
}

// accept attaches the apps that connect after the first, until the
// listener closes. Each connection handshakes in its own goroutine so a
// stalled peer cannot block subsequent reconnects.
func (v *session) accept() {
	for c := range v.conns {
		go func(c *net.UnixConn) {
			a, err := v.handshake(c)
			if err != nil {
				log.Print(err)
				_ = c.Close()
				return
			}
			select {
			case <-v.quitting:
				_ = a.conn.Close()
				return
			default:
			}
			v.attach(a, true)
			v.wake()
		}(c)
	}
}

// detachAll closes every app's connection, which ends the apps.
func (v *session) detachAll() {
	v.mu.Lock()
	apps := make([]*app, 0, len(v.apps))
	for a := range v.apps {
		apps = append(apps, a)
	}
	v.mu.Unlock()
	for _, a := range apps {
		_ = a.conn.Close()
	}
}

// read reads a's messages until it goes away.
func (v *session) read(a *app) {
	for {
		m, err := a.conn.Receive()
		if err != nil {
			v.mu.Lock()
			delete(v.apps, a)
			if v.cur == a {
				v.cur = nil
				v.resetRequests(true)
			}
			v.gone = append(v.gone, a)
			v.mu.Unlock()
			v.events.send(event{Type: evAppDetached, PID: a.pid})
			v.wake()
			v.kick()
			return
		}
		switch m := m.(type) {
		case *remote.SharedMemory:
			b, err := m.Map()
			if err != nil {
				log.Printf("app %d: mapping its shared memory: %v", a.pid, err)
				_ = a.conn.Close()
				continue
			}
			a.mu.Lock()
			if a.shm != nil {
				a.oldShm = append(a.oldShm, a.shm)
			}
			a.shm = b
			a.mu.Unlock()
		case *remote.Batch:
			a.mu.Lock()
			a.pending = append(a.pending, pendingBatch{batch: m, shm: a.shm})
			a.mu.Unlock()
			v.kick()
			continue
		case *remote.Idle:
			continue
		case *remote.ClipboardRequest:
			// The app's clipboard is this system's, which the app may not reach
			// itself (X11 needs a window of its own).
			text, err := ggwd.ClipboardText()
			if err != nil && v.opts.debug {
				log.Printf("app %d: reading the clipboard: %v", a.pid, err)
			}
			_ = a.conn.Send(&remote.ClipboardText{Seq: m.Seq, Text: text, OK: err == nil})
			continue
		case *remote.SetClipboard:
			if err := ggwd.SetClipboardText(m.Text); err != nil {
				log.Printf("app %d: writing the clipboard: %v", a.pid, err)
			}
			continue
		case *remote.Closed:
			v.mu.Lock()
			if v.cur == a {
				v.req.close = v.req.close || v.closeAsked
			}
			v.mu.Unlock()
		default:
			v.request(a, m)
		}
		v.wake()
	}
}

// request queues an app's request of the window for the main thread if a is
// still the active app.
func (v *session) request(a *app, m remote.Message) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cur != a {
		return
	}
	switch m := m.(type) {
	case *remote.OpenWindow:
		v.req.options = m
	case *remote.SetTitle:
		v.req.title = &m.Title
	case *remote.SetSize:
		v.req.size = m
	case *remote.SetPosition:
		v.req.position = m
	case *remote.SetIcon:
		v.req.icon = m
	case *remote.SetColorMode:
		v.req.colorMode = m
	case *remote.SetDockIcon:
		v.req.dockIcon = m
	case *remote.SetMenu:
		v.req.menu = m
		v.req.menuUpdates = nil
	case *remote.UpdateMenuItem:
		v.req.menuUpdates = append(v.req.menuUpdates, m)
	case *remote.SetTray:
		v.req.trays = append(v.req.trays, trayOp{set: m})
	case *remote.RemoveTray:
		v.req.trays = append(v.req.trays, trayOp{remove: m.Key})
	case *remote.SetCursor:
		v.req.cursor = m
	case *remote.TextInput:
		v.req.textInput = m
	case *remote.WindowRequest:
		v.req.window = append(v.req.window, m.Request)
	}
}
