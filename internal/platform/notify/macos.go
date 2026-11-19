package notify

import (
	"crypto/sha256"
	"encoding/hex"
)

// The pure parts of macOS's notifications, kept out of backend_darwin.go
// so that they are tested on any machine, as toast.go and freedesktop.go
// are for Windows and Linux.

// categoryID is the identifier of the UNNotificationCategory a macOS
// notification with actions is shown under: one category for each
// distinct set of buttons, named for a hash of their ids, titles and
// options in order, so that the same buttons find the category registered
// for them and a change to any of them makes another.
func categoryID(actions []Action) string {
	h := sha256.New()
	for _, a := range actions {
		fg := "0"
		if a.Foreground {
			fg = "1"
		}
		// A NUL cannot be in an identifier or a title AppKit keeps, so it
		// separates them without ambiguity.
		h.Write([]byte(a.ID + "\x00" + a.Title + "\x00" + fg + "\x00"))
	}
	return "ggui." + hex.EncodeToString(h.Sum(nil)[:12])
}

// osascriptArgs are the arguments of the osascript that shows n for an app
// run outside a bundle. The texts go in as arguments of the script rather
// than in its source, so that nothing in them needs quoting.
func osascriptArgs(n Notification) []string {
	cmd := "display notification (item 3 of argv) with title (item 1 of argv)"
	if n.Subtitle != "" {
		cmd += " subtitle (item 2 of argv)"
	}
	if n.Sound {
		cmd += ` sound name "default"`
	}
	return []string{"-e", "on run argv", "-e", cmd, "-e", "end run", n.Title, n.Subtitle, n.Body}
}
