package notify

import (
	"path"
	"strings"
)

// The freedesktop notification service, as the Desktop Notifications
// Specification names it.
const (
	fdService = "org.freedesktop.Notifications"
	fdPath    = "/org/freedesktop/Notifications"
	fdNotify  = "susssasa{sv}i" // Notify's signature
)

// fdDefaultAction is the key of the action a click on the notification
// itself invokes; an action of the app's is keyed fdActionPrefix and its
// id, so that no id of the app's can be taken for it.
const (
	fdDefaultAction = "default"
	fdActionPrefix  = "ggui."
)

// fdOptions are what notifyArgs builds a call from beside the
// notification.
type fdOptions struct {
	appName  string
	appID    string // the desktop entry, whose icon the service shows
	icon     string // an icon theme name, or ""
	replaces uint32 // the service's id of the notification n replaces, or 0
	markup   bool   // the service reads markup in the body
}

// notifyArgs are the arguments of the Notify call that shows n:
// app_name, replaces_id, app_icon, summary, body, actions, hints and
// expire_timeout. The subtitle has no place of its own and goes above the
// body; the group has none at all.
func notifyArgs(n Notification, o fdOptions) []any {
	body := joinSubtitle(n)
	if o.markup {
		body = markupEscape(body)
	}
	// The default action makes the notification clickable; its label is
	// what a server that lists it shows, which most do not.
	actions := []any{fdDefaultAction, "Open"}
	for _, a := range n.Actions {
		actions = append(actions, fdActionPrefix+a.ID, a.Title)
	}
	var hints []any
	if o.appID != "" {
		hints = append(hints, dictEntry{"desktop-entry", variant{"s", o.appID}})
	}
	if n.Sound {
		hints = append(hints, dictEntry{"sound-name", variant{"s", "message-new-instant"}})
	} else {
		hints = append(hints, dictEntry{"suppress-sound", variant{"b", true}})
	}
	return []any{o.appName, o.replaces, o.icon, n.Title, body, actions, hints, int32(-1)}
}

// fdAction is the app's action an ActionInvoked key is for: "" for the
// notification itself, and false for a key the app did not give.
func fdAction(key string) (string, bool) {
	if key == fdDefaultAction {
		return "", true
	}
	if id, ok := strings.CutPrefix(key, fdActionPrefix); ok {
		return id, true
	}
	return "", false
}

// sessionBusPath is the socket of the user's session bus, given the
// environment: the first unix path or abstract name of
// DBUS_SESSION_BUS_ADDRESS, or else $XDG_RUNTIME_DIR/bus where that exists,
// where systemd and dbus-broker put it. An abstract name comes back with
// the "@" Go dials one by. It is "" where there is no bus to be found.
func sessionBusPath(getenv func(string) string, exists func(string) bool) string {
	if addr := getenv("DBUS_SESSION_BUS_ADDRESS"); addr != "" {
		for a := range strings.SplitSeq(addr, ";") {
			rest, ok := strings.CutPrefix(a, "unix:")
			if !ok {
				continue
			}
			for kv := range strings.SplitSeq(rest, ",") {
				k, v, _ := strings.Cut(kv, "=")
				v = unescapeAddress(v)
				switch k {
				case "path":
					return v
				case "abstract":
					return "@" + v
				}
			}
		}
		return ""
	}
	if dir := getenv("XDG_RUNTIME_DIR"); dir != "" {
		if p := path.Join(dir, "bus"); exists(p) {
			return p
		}
	}
	return ""
}

// unescapeAddress undoes the %xx escapes of a D-Bus address value.
func unescapeAddress(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if h, ok := unhex(s[i+1]); ok {
				if l, ok := unhex(s[i+2]); ok {
					b.WriteByte(h<<4 | l)
					i += 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func unhex(c byte) (byte, bool) {
	switch {
	case '0' <= c && c <= '9':
		return c - '0', true
	case 'a' <= c && c <= 'f':
		return c - 'a' + 10, true
	case 'A' <= c && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// markupEscape escapes s for the body of a freedesktop notification
// server that reads markup in it, which would otherwise take a "<" or an
// "&" of the text for the start of a tag or an entity.
func markupEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}
