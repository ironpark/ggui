//go:build (linux && !android) || freebsd || netbsd || openbsd || dragonfly

package notify

import (
	"reflect"
	"testing"
	"time"
)

// fakeService is a notification service on the session bus, played by
// this test: it records the calls made to it, answers Notify with a
// number, and emits what the test asks.
type fakeService struct {
	conn *busConn
	got  chan *message
}

func startFakeService(t *testing.T) *fakeService {
	t.Helper()
	f := &fakeService{got: make(chan *message, 16)}
	conn, err := dialBus(busPath(), f.handle)
	if err != nil {
		t.Fatalf("dialling the bus: %v", err)
	}
	f.conn = conn
	t.Cleanup(func() { conn.close(errClosedByTest) })
	// DBUS_NAME_FLAG_DO_NOT_QUEUE; 1 is the primary owner.
	r, err := conn.call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "RequestName", "su", fdService, uint32(4))
	if err != nil || r.body[0] != uint32(1) {
		t.Skipf("cannot own %s (a notification service runs?): %v %v", fdService, r, err)
	}
	return f
}

type closedByTest struct{}

func (closedByTest) Error() string { return "closed by the test" }

var errClosedByTest error = closedByTest{}

var nextNumber uint32 = 6

func (f *fakeService) handle(m *message) {
	if m.typ != msgCall || m.iface != fdService {
		return
	}
	reply := &message{typ: msgReturn, replySerial: m.serial, dest: m.sender}
	switch m.member {
	case "GetCapabilities":
		reply.sig, reply.body = "as", []any{[]string{"actions", "body", "body-markup"}}
	case "Notify":
		num := m.body[1].(uint32)
		if num == 0 {
			nextNumber++
			num = nextNumber
		}
		reply.sig, reply.body = "u", []any{num}
	}
	_ = f.conn.send(reply, nil)
	f.got <- m
}

func (f *fakeService) emit(member, sig string, body ...any) {
	_ = f.conn.send(&message{typ: msgSignal, path: fdPath, iface: fdService, member: member, sig: sig, body: body}, nil)
}

func (f *fakeService) next(t *testing.T, member string) *message {
	t.Helper()
	for {
		select {
		case m := <-f.got:
			if m.member == member {
				return m
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no %s call", member)
		}
	}
}

// Against a real bus, and a service the test plays, the backend posts,
// replaces by ID, hears a click on a button and closes a notification.
// It runs where there is a session bus, as in a desktop session or under
// dbus-run-session, and skips elsewhere.
func TestFreedesktopAgainstABus(t *testing.T) {
	if busPath() == "" {
		t.Skip("no D-Bus session bus")
	}
	f := startFakeService(t)
	clicks := make(chan Response, 4)
	Start(Options{AppName: "Editor", AppID: "com.example.editor", Respond: func(r Response) { clicks <- r }})

	n := Notification{ID: "chat", Title: "Ada", Subtitle: "#engines", Body: "ready <yet>?", Data: "chat/42",
		Actions: []Action{{ID: "reply", Title: "Reply", Foreground: true}, {ID: "read", Title: "Mark as read"}}}
	if err := Post(n); err != nil {
		t.Fatal(err)
	}
	m := f.next(t, "Notify")
	want := notifyArgs(n, fdOptions{appName: "Editor", appID: "com.example.editor", markup: true})
	want[5] = []any{"default", "Open", "ggui.reply", "Reply", "ggui.read", "Mark as read"}
	if !reflect.DeepEqual(m.body, want) {
		t.Errorf("Notify\n%#v\nwant\n%#v", m.body, want)
	}
	if m.body[4] != "#engines\nready &lt;yet&gt;?" {
		t.Errorf("body %q", m.body[4])
	}
	first := nextNumber

	n.Body = "now"
	if err := Post(n); err != nil {
		t.Fatal(err)
	}
	if m := f.next(t, "Notify"); m.body[1] != first {
		t.Errorf("the second post replaces %v, want %d", m.body[1], first)
	}

	f.emit("ActionInvoked", "us", first, "ggui.read")
	select {
	case r := <-clicks:
		if r != (Response{ID: "chat", Data: "chat/42", Action: "read"}) {
			t.Errorf("click %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no response to ActionInvoked")
	}
	f.emit("ActionInvoked", "us", first, "default")
	if r := <-clicks; r != (Response{ID: "chat", Data: "chat/42", Foreground: true}) {
		t.Errorf("click on the notification %+v", r)
	}

	Remove("chat")
	if m := f.next(t, "CloseNotification"); m.body[0] != first {
		t.Errorf("CloseNotification(%v), want %d", m.body[0], first)
	}
	f.emit("NotificationClosed", "uu", first, uint32(3))
	f.emit("ActionInvoked", "us", first, "default")
	select {
	case r := <-clicks:
		t.Errorf("a click on a closed notification was answered: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}

	done := make(chan bool, 1)
	RequestPermission(func(ok bool) { done <- ok })
	if !<-done {
		t.Error("RequestPermission said no with a bus")
	}
}
