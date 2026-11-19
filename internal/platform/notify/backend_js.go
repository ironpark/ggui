//go:build js

package notify

import (
	"fmt"
	"sync"
	"syscall/js"
)

// The browser shows a Notification, once the user has allowed the page
// to: Post asks the first time. A page's notifications have no buttons,
// which only a service worker's have, and their subtitle goes above the
// body. A notification with a tag replaces the one with the same tag.

// shown is what the app keeps of a notification up: the object, to close
// it by, and its handlers, released once it closes.
type shown struct {
	n                js.Value
	onclick, onclose js.Func
	released         sync.Once
}

var (
	shownMu sync.Mutex
	byID    = map[string]*shown{}
)

func setup() {}

// api is the page's Notification constructor, or undefined.
func api() js.Value { return js.Global().Get("Notification") }

func check() error {
	n := api()
	if n.IsUndefined() || n.IsNull() {
		return fmt.Errorf("%w: the browser has no Notification API", ErrUnavailable)
	}
	if n.Get("permission").String() == "denied" {
		return ErrDenied
	}
	return nil
}

// allowed asks the user to allow notifications, unless they have answered
// already, and reports whether they did.
func allowed() bool {
	n := api()
	switch n.Get("permission").String() {
	case "granted":
		return true
	case "denied":
		return false
	}
	answer := make(chan string, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		s := "default"
		if len(args) > 0 && args[0].Type() == js.TypeString {
			s = args[0].String()
		}
		answer <- s
		return nil
	})
	defer then.Release()
	fail := js.FuncOf(func(js.Value, []js.Value) any {
		answer <- "default"
		return nil
	})
	defer fail.Release()
	n.Call("requestPermission").Call("then", then, fail)
	return <-answer == "granted"
}

func post(n Notification) error {
	if !allowed() {
		return ErrDenied
	}
	o := js.Global().Get("Object").New()
	o.Set("body", joinSubtitle(n))
	o.Set("silent", !n.Sound)
	if n.ID != "" {
		o.Set("tag", n.ID)
		// A replacement alerts as the first did.
		o.Set("renotify", true)
	}
	s := &shown{n: api().New(n.Title, o)}
	id, data := n.ID, n.Data
	s.onclick = js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Call("focus")
		s.n.Call("close")
		respond(Response{ID: id, Data: data, Foreground: true})
		return nil
	})
	s.onclose = js.FuncOf(func(js.Value, []js.Value) any {
		shownMu.Lock()
		if byID[id] == s {
			delete(byID, id)
		}
		shownMu.Unlock()
		s.release()
		return nil
	})
	s.n.Set("onclick", s.onclick)
	s.n.Set("onclose", s.onclose)
	if id != "" {
		shownMu.Lock()
		old := byID[id]
		byID[id] = s
		shownMu.Unlock()
		if old != nil {
			// A tag's replacement closes the old one without a close
			// event in some browsers.
			old.release()
		}
	}
	return nil
}

// release lets go of s's handlers; the notification no longer calls them.
func (s *shown) release() {
	s.released.Do(func() {
		s.n.Set("onclick", js.Null())
		s.n.Set("onclose", js.Null())
		s.onclick.Release()
		s.onclose.Release()
	})
}

func remove(id string) {
	shownMu.Lock()
	s := byID[id]
	shownMu.Unlock()
	if s != nil {
		s.n.Call("close")
	}
}

func permission(fn func(bool)) { go fn(check() == nil && allowed()) }
