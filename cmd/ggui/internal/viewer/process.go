// Package viewer runs ggviewer, the window ggui dev shows an app in while
// the app is rebuilt and restarted, and speaks its control channel: JSON
// lines on its stdin and stdout, as cmd/ggviewer's documentation lays
// them out.
//
// The viewer is not part of this command, which imports nothing of ggui:
// it is built from the project's own module graph, so that it speaks the
// protocol of the ggwd and ggfx the app is built with, and kept in the
// user's cache directory under a key of everything that decides the
// binary (see Resolve).
package viewer

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/proc"
)

// Command is a message to the viewer.
type Command struct {
	Type   string `json:"type"`
	Output string `json:"output,omitempty"`
	Status string `json:"status,omitempty"`
}

// The commands.
const (
	Building    = "building"     // a build runs
	BuildFailed = "build-failed" // with the compiler's Output
	AppExited   = "app-exited"   // the app ended on its own: Status and Output
	Clear       = "clear"
	Quit        = "quit"
)

// Event is a message from the viewer.
type Event struct {
	Type   string `json:"type"`
	Socket string `json:"socket,omitempty"`
	PID    int    `json:"pid,omitempty"`
}

// The events.
const (
	Ready        = "ready"
	AppAttached  = "app-attached"
	FirstFrame   = "first-frame"
	AppDetached  = "app-detached"
	WindowClosed = "window-closed"
)

// readyTimeout is how long a viewer has to say it listens.
const readyTimeout = 15 * time.Second

// Process is a running viewer.
type Process struct {
	p      *proc.Process
	dir    string // the socket's directory, removed with the viewer
	socket string
	events chan Event

	mu      sync.Mutex
	stdin   io.WriteCloser
	stopped chan struct{} // closed by Stop: nobody reads events any more
}

// Options says how to start a viewer.
type Options struct {
	// Project names the project, so that the viewer remembers where its
	// window was: the module's root directory.
	Project string
	Stderr  io.Writer // the viewer's logs
}

// Start starts the viewer exe listening on a socket of its own and waits
// until it is ready.
func Start(ctx context.Context, exe string, o Options) (*Process, error) {
	dir, socket, err := SocketPath("v.sock")
	if err != nil {
		return nil, err
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		inR.Close()
		inW.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	args := []string{"-socket", socket}
	if o.Project != "" {
		args = append(args, "-project", o.Project)
	}
	p, err := proc.Start(exe, args, proc.Options{Stdin: inR, Stdout: outW, Stderr: o.Stderr})
	// The child has its own ends now; the viewer reads EOF on stdin once
	// inW closes, and this side reads EOF once the viewer exits.
	inR.Close()
	outW.Close()
	if err != nil {
		inW.Close()
		outR.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	v := &Process{p: p, dir: dir, socket: socket, events: make(chan Event, 16), stdin: inW, stopped: make(chan struct{})}
	ready := make(chan Event, 1)
	go v.read(outR, ready)

	timer := time.NewTimer(readyTimeout)
	defer timer.Stop()
	select {
	case ev, ok := <-ready:
		if ok {
			if ev.Socket != "" {
				v.socket = ev.Socket
			}
			return v, nil
		}
		err = fmt.Errorf("it ended (%s) before it was ready", p.Exited())
	case <-timer.C:
		err = fmt.Errorf("it was not ready after %s", readyTimeout)
	case <-ctx.Done():
		err = ctx.Err()
	}
	v.Stop(0)
	return nil, err
}

// SocketPath returns a new directory, which only its user may enter, and
// the path of a unix socket called name in it: the viewer's, and off
// Windows, which has a named pipe for it, ggui dev's channel for the state
// an app keeps. A unix socket's path is limited to 104 bytes on macOS and
// 108 on Linux and Windows, so the
// directory is short: the temporary directory when it leaves room, /tmp
// otherwise. Windows has no /tmp, and its temporary directory, under the
// user's profile, leaves room but for the longest user names; then there
// is no socket, and the app opens its own window.
func SocketPath(name string) (dir, socket string, err error) {
	const maxPath = 103 // with the 0 that ends it, in macOS's 104 bytes
	base := ""
	if len(os.TempDir())+len(`/ggui-1234567890/`)+len(name) > maxPath && runtime.GOOS != "windows" {
		base = "/tmp"
	}
	dir, err = os.MkdirTemp(base, "ggui-")
	if err != nil {
		return "", "", err
	}
	socket = filepath.Join(dir, name)
	if len(socket) > maxPath {
		os.RemoveAll(dir)
		return "", "", fmt.Errorf("the viewer's socket would be %s, longer than a unix socket's %d bytes", socket, maxPath)
	}
	return dir, socket, nil
}

// read decodes the viewer's events until it exits, the first, ready, to
// ready and the rest to v.events, which it closes once the viewer is gone.
func (v *Process) read(r io.ReadCloser, ready chan<- Event) {
	defer r.Close()
	sc := bufio.NewScanner(r)
	first := true
	for sc.Scan() {
		var ev Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Type == "" {
			continue
		}
		if first && ev.Type == Ready {
			first = false
			ready <- ev
			continue
		}
		select {
		case v.events <- ev:
		case <-v.stopped:
		}
	}
	_ = sc.Err() // the viewer exiting ends the stream
	<-v.p.Done()
	if first {
		close(ready)
	}
	close(v.events)
}

// Socket is the socket the app connects to.
func (v *Process) Socket() string { return v.socket }

// Pid is the viewer's process ID.
func (v *Process) Pid() int { return v.p.Pid() }

// Events delivers what the viewer says after ready, and is closed once it
// has exited.
func (v *Process) Events() <-chan Event { return v.events }

// Exited describes how a viewer whose Events closed exited.
func (v *Process) Exited() string { return v.p.Exited() }

// Send sends the viewer a command.
func (v *Process) Send(c Command) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.stdin == nil {
		return errors.New("the viewer is stopped")
	}
	_, err = v.stdin.Write(append(b, '\n'))
	return err
}

// Stop ends the viewer: it is told to quit and its stdin closes, which
// it exits on; if it has not exited after timeout, it is killed. The
// socket's directory goes with it.
func (v *Process) Stop(timeout time.Duration) {
	v.Send(Command{Type: Quit})
	v.mu.Lock()
	if v.stdin != nil {
		v.stdin.Close()
		v.stdin = nil
		close(v.stopped)
	}
	v.mu.Unlock()
	select {
	case <-v.p.Done():
	case <-time.After(timeout):
	}
	v.p.Stop(0)
	os.RemoveAll(v.dir)
}
