package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"strings"
	"unicode"
)

// The Windows toast's pure parts: its XML, its tag and the AppUserModelID
// it is shown under.

// maxToastActions is the most buttons a Windows toast shows.
const maxToastActions = 5

// toastTag is the tag of the toast for the notification id. Windows takes
// a tag of at most 64 characters; a longer id is replaced by a hash of it.
func toastTag(id string) string {
	if len([]rune(id)) <= 64 {
		return id
	}
	sum := sha256.Sum256([]byte(id))
	return "ggui." + hex.EncodeToString(sum[:20])
}

// toastXML is the toast content of n, in the ToastGeneric template: the
// title, the subtitle and the body as its lines of text, up to five
// buttons, and silence unless n asks for its sound. Each click hands back
// the arguments of what was clicked, the payload of n with the button's
// id.
func toastXML(n Notification) string {
	var b strings.Builder
	b.WriteString(`<toast launch="`)
	xmlEscape(&b, payload{ID: n.ID, Data: n.Data, Foreground: []string{"1"}}.encode())
	b.WriteString(`" activationType="foreground"><visual><binding template="ToastGeneric">`)
	texts := 0
	for _, t := range []string{n.Title, n.Subtitle, n.Body} {
		if t == "" {
			continue
		}
		b.WriteString("<text>")
		xmlEscape(&b, t)
		b.WriteString("</text>")
		texts++
	}
	if texts == 0 {
		b.WriteString("<text></text>")
	}
	b.WriteString("</binding></visual>")
	if len(n.Actions) > 0 {
		b.WriteString("<actions>")
		for i, a := range n.Actions {
			if i == maxToastActions {
				break
			}
			p := payload{ID: n.ID, Data: n.Data, Action: a.ID}
			if a.Foreground {
				p.Foreground = []string{"1"}
			}
			b.WriteString(`<action content="`)
			xmlEscape(&b, a.Title)
			b.WriteString(`" arguments="`)
			xmlEscape(&b, p.encode())
			// A button whose activation is in the background goes to a
			// COM activator, which an app that is not packaged has none
			// of; in the foreground it raises the toast's Activated
			// event in the running app, as a click on the toast does.
			b.WriteString(`" activationType="foreground"/>`)
		}
		b.WriteString("</actions>")
	}
	if !n.Sound {
		b.WriteString(`<audio silent="true"/>`)
	}
	b.WriteString("</toast>")
	return b.String()
}

// xmlEscape writes s to b escaped as XML text, quotes included, so that it
// may stand in an attribute too.
func xmlEscape(b *strings.Builder, s string) {
	// Writing to a strings.Builder does not fail.
	_ = xml.EscapeText(b, []byte(s))
}

// aumid is the AppUserModelID the toasts of the app with id are shown
// under: the id, which ggui's reverse-DNS convention makes a valid one,
// with what an AUMID cannot hold, a space or a control character, made a
// dot, and cut to the 128 characters it may have.
func aumid(id string) string {
	id = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return '.'
		}
		return r
	}, id)
	if r := []rune(id); len(r) > 128 {
		id = string(r[:128])
	}
	if id == "" {
		id = "ggui"
	}
	return id
}
