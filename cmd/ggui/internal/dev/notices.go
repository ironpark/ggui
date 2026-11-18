package dev

import (
	"bytes"
	"io"
	"regexp"
	"sync"
)

// An app shown by the viewer says once on its stderr what does not show
// there, in a line ggui's remote.go writes:
//
//	ggui: the tray icon does not show while ggui dev's viewer shows the app; ggui dev --display=window or a release build does
//
// One line among the app's own output was read as the tray being broken,
// so ggui dev takes such a line out of the app's output and says it
// itself, as a warning with the flag that shows what is missing, once a
// session rather than once per restart.
var remoteNoticeLine = regexp.MustCompile(`^ggui: (.+) while ggui dev's viewer shows the app;`)

// noticePrefix is how every such line begins.
const noticePrefix = "ggui: "

// maxHeld is the most of a line that may be a notice held back before it
// is let through as it is: a notice is one short line.
const maxHeld = 4 << 10

// notices passes an app's stderr on to out, but for the viewer's notices,
// which it hands to say instead, each distinct one once in the life of
// seen. A line that may be a notice is held until it ends, the rest
// passed on as written.
type notices struct {
	out  io.Writer
	say  func(what string)
	seen *noticeSet

	mu      sync.Mutex
	held    []byte
	holding bool
	mid     bool // within a line, past its start
}

// noticeSet is the notices already said, shared by the app's runs.
type noticeSet struct {
	mu   sync.Mutex
	said map[string]bool
}

// first reports whether what has not been said before, and marks it.
func (s *noticeSet) first(what string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.said[what] {
		return false
	}
	if s.said == nil {
		s.said = map[string]bool{}
	}
	s.said[what] = true
	return true
}

func (n *notices) Write(p []byte) (int, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	written := len(p)
	for len(p) > 0 {
		end := bytes.IndexByte(p, '\n')
		line := p
		if end >= 0 {
			line = p[:end+1]
		}
		p = p[len(line):]
		switch {
		case n.holding:
			n.held = append(n.held, line...)
		case !n.mid && couldBeNotice(line):
			n.holding = true
			n.held = append(n.held[:0], line...)
		default:
			if _, err := n.out.Write(line); err != nil {
				return written - len(p) - len(line), err
			}
			n.mid = end < 0
			continue
		}
		// Held: a whole line to judge, or a part that has stopped
		// looking like a notice.
		whole := n.held[len(n.held)-1] == '\n'
		if !whole && couldBeNotice(n.held) && len(n.held) < maxHeld {
			continue
		}
		n.holding, n.mid = false, !whole
		if whole {
			if m := remoteNoticeLine.FindSubmatch(n.held); m != nil {
				if n.seen.first(string(m[1])) {
					n.say(string(m[1]))
				}
				continue
			}
		}
		if _, err := n.out.Write(n.held); err != nil {
			return written - len(p), err
		}
	}
	return written, nil
}

// couldBeNotice reports whether a line beginning with b may be a notice.
func couldBeNotice(b []byte) bool {
	if len(b) < len(noticePrefix) {
		return string(b) == noticePrefix[:len(b)]
	}
	return string(b[:len(noticePrefix)]) == noticePrefix
}

// restate says a notice of the viewer's as ggui dev's own: what does not
// show, why, and how to see it.
func restate(what string) string {
	if what == "the tray icon does not show" {
		return "the app's tray icon does not show while the viewer shows the app, which has no tray of its own; run ggui dev --display=window to see the tray"
	}
	return what + " while the viewer shows the app; run ggui dev --display=window to see it"
}
