//go:build (linux && !android) || freebsd || netbsd || openbsd || dragonfly

package notify

import (
	"fmt"
	"os"
	"slices"
	"sync"
)

// Linux and the BSDs post to the freedesktop notification service, which
// the desktop runs on the user's session bus, through a connection of the
// package's own: see dbus_wire.go for why not a library. The service
// knows a notification by a number it hands back, which a later one
// replaces by and which its signals name.

// fdSent is what the app keeps of a notification the service shows: what
// a click on it is answered with.
type fdSent struct {
	id, data   string
	foreground []string // the actions that bring the app forward
}

var fd struct {
	sync.Mutex
	conn   *busConn
	failed error             // the reason the bus could not be reached
	markup bool              // the service reads markup in the body
	ids    map[string]uint32 // a notification's ID to the service's number
	sent   map[uint32]fdSent // the service's number to the notification
}

func setup() {}

// busPath is the session bus's socket, or "".
func busPath() string { return sessionBusPath(os.Getenv, fileExists) }

func check() error {
	if busPath() == "" {
		return fmt.Errorf("%w: there is no D-Bus session bus", ErrUnavailable)
	}
	fd.Lock()
	defer fd.Unlock()
	if fd.failed != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, fd.failed)
	}
	return nil
}

// connect connects to the session bus, once it is first needed and again
// after a connection ended, and asks for the service's signals. A bus that
// cannot be reached is not tried again: Post says so from then on.
func connect() (*busConn, error) {
	fd.Lock()
	conn := fd.conn
	fd.Unlock()
	if conn != nil && conn.closedErr() == nil {
		return conn, nil
	}
	conn, err := dialBus(busPath(), onSignal)
	if err != nil {
		fd.Lock()
		fd.failed = err
		fd.Unlock()
		return nil, fmt.Errorf("%w: the D-Bus session bus: %v", ErrUnavailable, err)
	}
	rule := "type='signal',sender='" + fdService + "',path='" + fdPath + "',interface='" + fdService + "'"
	if _, err := conn.call("org.freedesktop.DBus", "/org/freedesktop/DBus", "org.freedesktop.DBus", "AddMatch", "s", rule); err != nil {
		conn.close(err)
		return nil, err
	}
	markup := false
	if r, err := conn.call(fdService, fdPath, fdService, "GetCapabilities", ""); err == nil && len(r.body) == 1 {
		caps, _ := r.body[0].([]any)
		markup = slices.Contains(caps, any("body-markup"))
	}
	fd.Lock()
	fd.conn, fd.markup = conn, markup
	if fd.ids == nil {
		fd.ids, fd.sent = map[string]uint32{}, map[uint32]fdSent{}
	}
	fd.Unlock()
	return conn, nil
}

func post(n Notification) error {
	conn, err := connect()
	if err != nil {
		return err
	}
	o := options()
	name := o.AppName
	if name == "" {
		name = o.AppID
	}
	fd.Lock()
	fo := fdOptions{appName: name, appID: o.AppID, icon: o.Icon, markup: fd.markup}
	if n.ID != "" {
		fo.replaces = fd.ids[n.ID]
	}
	fd.Unlock()
	r, err := conn.call(fdService, fdPath, fdService, "Notify", fdNotify, notifyArgs(n, fo)...)
	if err != nil {
		return err
	}
	var num uint32
	if len(r.body) == 1 {
		num, _ = r.body[0].(uint32)
	}
	fd.Lock()
	defer fd.Unlock()
	if fo.replaces != 0 && fo.replaces != num {
		delete(fd.sent, fo.replaces)
	}
	if n.ID != "" {
		fd.ids[n.ID] = num
	}
	fd.sent[num] = fdSent{id: n.ID, data: n.Data, foreground: foregroundIDs(n.Actions)}
	return nil
}

func remove(id string) {
	fd.Lock()
	num, ok := fd.ids[id]
	conn := fd.conn
	fd.Unlock()
	if !ok || conn == nil {
		return
	}
	if _, err := conn.call(fdService, fdPath, fdService, "CloseNotification", "u", num); err != nil {
		Report(err)
	}
}

// permission: the service asks nobody.
func permission(fn func(bool)) {
	_, err := connect()
	go fn(err == nil)
}

// onSignal follows the service's signals: ActionInvoked for a click,
// NotificationClosed for a notification that went, which this app then
// forgets.
func onSignal(m *message) {
	if m.typ != msgSignal || m.path != fdPath || m.iface != fdService || len(m.body) != 2 {
		return
	}
	num, _ := m.body[0].(uint32)
	fd.Lock()
	s, ok := fd.sent[num]
	if m.member == "NotificationClosed" && ok {
		delete(fd.sent, num)
		if fd.ids[s.id] == num {
			delete(fd.ids, s.id)
		}
	}
	fd.Unlock()
	if m.member != "ActionInvoked" || !ok {
		return
	}
	key, _ := m.body[1].(string)
	action, ok := fdAction(key)
	if !ok {
		return
	}
	respond(Response{ID: s.id, Data: s.data, Action: action, Foreground: action == "" || slices.Contains(s.foreground, action)})
}

// fileExists reports whether path names something.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
