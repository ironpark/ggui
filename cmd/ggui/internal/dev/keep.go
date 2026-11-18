package dev

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"
)

// The state an app keeps across a restart, what it marks with
// StateValue.Keep, goes through ggui dev: the keeper listens on a channel
// whose address, and the instance it is handed to, each run of the app
// finds in keepEnv. The channel is a unix socket in a directory only the
// user may enter, or on Windows a named pipe only the user may open (see
// listenKeep): a Windows older than 10 1803 has no unix sockets, and a
// pipe is what every Windows has, so Windows takes the one path. An app that keeps state says hello on its first
// Keep and is answered with what the instance before it kept, at once, so
// that the app has it before its first frame. Before the app is stopped to
// be replaced, the keeper asks it for what it keeps now and holds that for
// the next. The channel is a JSON object a line each way, as the viewer's
// is; ggui's package devstate is the other end and documents the lines.
//
// The state is ggui dev's to hold between two instances, not the app's to
// write somewhere on its way out: a request to exit is a signal, which an
// app need not catch, or on Windows a console event a windowed app never
// receives, and an app that crashed or was killed writes nothing. Held
// here, a failed build in between, or an app that exited by itself,
// changes nothing: the next build that succeeds is handed the last state
// known.

// keepEnv is ggui's devstate.Env, which this command, importing nothing of
// ggui's, spells out. Its value is the instance and the channel's address,
// "3:/tmp/ggui-123/k.sock", or on Windows "3:\\.\pipe\ggui-dev-…": the
// app dials a pipe when the address is a pipe's name and a unix socket
// otherwise.
const keepEnv = "GGUI_DEV_STATE"

// handTimeout is how long the app has to hand over what it keeps before it
// is stopped: enough for a frame to end and the values to be encoded, short
// enough that an app that hangs does not hold up the restart much.
const handTimeout = 500 * time.Millisecond

// helloTimeout is how long a connection has to say hello.
const helloTimeout = 5 * time.Second

// errNoAnswer is take's when the app did not answer in time.
var errNoAnswer = errors.New("no answer")

// keepMsg is one line of the channel.
type keepMsg struct {
	Type     string                     `json:"type"`
	Instance string                     `json:"instance,omitempty"`
	State    map[string]json.RawMessage `json:"state,omitempty"`
}

// keepStream is one connection: a unix socket's, or a named pipe's file,
// which Go's poller gives deadlines and a Close that ends a pending read.
type keepStream interface {
	io.ReadWriteCloser
	SetReadDeadline(time.Time) error
	SetWriteDeadline(time.Time) error
}

// keepListener takes the apps' connections. Accept returns an error only
// once it is closed or broken for good.
type keepListener interface {
	Accept() (keepStream, error)
	Close() error
}

// keeper holds the state between instances and speaks to the running one.
type keeper struct {
	ln      keepListener
	addr    string
	cleanup func() // removes what listenKeep made, after ln is closed

	mu   sync.Mutex
	last map[string]json.RawMessage // what the next instance is handed
	want string                     // the instance it is handed to, once started
	next int
	cur  *keepConn // the running instance's channel, once it said hello
}

// keepConn is the running instance's channel.
type keepConn struct {
	conn  keepStream
	enc   *json.Encoder
	sent  chan struct{} // closed once the state was handed over
	saved chan map[string]json.RawMessage
	gone  chan struct{} // closed once the channel ends
}

// newKeeper listens for apps.
func newKeeper() (*keeper, error) {
	ln, addr, cleanup, err := listenKeep()
	if err != nil {
		return nil, err
	}
	k := &keeper{ln: ln, addr: addr, cleanup: cleanup}
	go k.accept()
	return k, nil
}

func (k *keeper) accept() {
	for {
		c, err := k.ln.Accept()
		if err != nil {
			return // closed
		}
		go k.serve(c)
	}
}

// serve answers a hello with the state, when it is from the instance the
// state is handed to, and with nothing otherwise, as for a process the app
// started, which has its environment, or an instance being replaced that
// kept its first value late. The instance's channel then carries its
// answers until it ends.
func (k *keeper) serve(c keepStream) {
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(helloTimeout))
	enc, dec := json.NewEncoder(c), json.NewDecoder(c)
	var m keepMsg
	if dec.Decode(&m) != nil || m.Type != "hello" {
		return
	}
	c.SetReadDeadline(time.Time{})
	k.mu.Lock()
	var kc *keepConn
	var st map[string]json.RawMessage
	if m.Instance != "" && m.Instance == k.want && k.cur == nil {
		kc = &keepConn{conn: c, enc: enc, sent: make(chan struct{}), saved: make(chan map[string]json.RawMessage, 1), gone: make(chan struct{})}
		k.cur, st = kc, k.last
	}
	k.mu.Unlock()
	// The map is replaced whole, never changed, so it is read unlocked.
	c.SetWriteDeadline(time.Now().Add(helloTimeout))
	err := enc.Encode(keepMsg{Type: "state", State: st})
	c.SetWriteDeadline(time.Time{})
	if kc == nil {
		return
	}
	defer close(kc.gone)
	if err != nil {
		return
	}
	close(kc.sent)
	for {
		var r keepMsg
		if dec.Decode(&r) != nil {
			return
		}
		if r.Type == "saved" {
			select {
			case kc.saved <- r.State:
			default:
			}
		}
	}
}

// handTo returns the environment entry of the next instance, the one the
// state is handed to.
func (k *keeper) handTo() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.next++
	k.want, k.cur = strconv.Itoa(k.next), nil
	return keepEnv + "=" + k.want + ":" + k.addr
}

// take asks the running instance for what it keeps, before it is stopped,
// and holds that for the next, returning how many values it holds. An
// instance that never said hello, because it keeps nothing or had not got
// that far, or whose channel ended, as when it crashed, changes nothing:
// it was handed the last state, and the next one is handed that. One that
// does not answer within timeout is not waited for, and the next starts
// with nothing, errNoAnswer says: a state that may have hung one instance
// is not handed to the next.
func (k *keeper) take(timeout time.Duration) (int, error) {
	k.mu.Lock()
	c := k.cur
	k.cur, k.want = nil, ""
	n := len(k.last)
	k.mu.Unlock()
	if c == nil {
		return n, nil
	}
	defer c.conn.Close()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	// The request follows the state on the channel, never overtakes it.
	select {
	case <-c.sent:
	case <-c.gone:
		return n, nil
	case <-timer.C:
		return n, nil
	}
	c.conn.SetWriteDeadline(time.Now().Add(timeout))
	if c.enc.Encode(keepMsg{Type: "save"}) != nil {
		return n, nil
	}
	select {
	case st := <-c.saved:
		k.mu.Lock()
		k.last = st
		k.mu.Unlock()
		return len(st), nil
	case <-c.gone:
		return n, nil
	case <-timer.C:
		k.mu.Lock()
		k.last = nil
		k.mu.Unlock()
		return 0, errNoAnswer
	}
}

// close stops listening, ends the running instance's channel and removes
// the socket.
func (k *keeper) close() {
	k.ln.Close()
	k.mu.Lock()
	if k.cur != nil {
		k.cur.conn.Close()
		k.cur = nil
	}
	k.mu.Unlock()
	k.cleanup()
}
