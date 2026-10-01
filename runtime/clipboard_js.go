package runtime

import (
	"strings"
	"sync"
	"syscall/js"
)

// browserClipboard keeps a copy in the page and keeps it in step with the
// system's. The browser hands its clipboard out only asynchronously, so a
// read cannot ask it. Instead a paste the user starts fires a paste event,
// which hands over the system's text before the frame that handles ⌘V or
// Ctrl+V reads it. A write goes to the system through
// navigator.clipboard, which the browser allows after a key press or click.
type browserClipboard struct {
	MemoryClipboard
	done js.Func // the writeText promise's handler, kept to stay callable
}

var (
	pageClipboard     *browserClipboard
	pageClipboardOnce sync.Once
)

func nativeClipboard() Clipboard {
	pageClipboardOnce.Do(func() {
		c := &browserClipboard{}
		c.done = js.FuncOf(func(js.Value, []js.Value) any { return nil })
		if doc := js.Global().Get("document"); doc.Truthy() {
			opts := js.Global().Get("Object").New()
			opts.Set("capture", true)
			doc.Call("addEventListener", "paste", js.FuncOf(func(_ js.Value, args []js.Value) any {
				c.paste(args[0])
				return nil
			}), opts)
		}
		pageClipboard = c
	})
	return pageClipboard
}

// paste takes the text of a paste aimed at the app: at its canvas or at the
// hidden text area its IME types into. The text area's own insertion is
// cancelled, since the focused field pastes what it reads from here.
func (c *browserClipboard) paste(e js.Value) {
	target := e.Get("target")
	if !target.Truthy() {
		return
	}
	if target.Get("tagName").String() != "CANVAS" && !strings.HasPrefix(target.Get("id").String(), "ebitengine-textinput") {
		return
	}
	data := e.Get("clipboardData")
	if !data.Truthy() {
		return
	}
	c.MemoryClipboard.Write(data.Call("getData", "text/plain").String())
	e.Call("preventDefault")
}

// Write implements Clipboard: the page's copy now, the system's when the
// browser allows it.
func (c *browserClipboard) Write(s string) {
	c.MemoryClipboard.Write(s)
	clip := js.Global().Get("navigator").Get("clipboard")
	if !clip.Truthy() || !clip.Get("writeText").Truthy() {
		return
	}
	// A refusal, without a user gesture or permission, leaves the copy in
	// the page; catching it keeps it out of the console.
	clip.Call("writeText", s).Call("catch", c.done)
}
