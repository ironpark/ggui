//go:build js && wasm

package router

import (
	"strings"
	"syscall/js"
)

// Browser history state keys. Entries keep any other state a page stored.
const (
	stateID    = "gguiRouterID"
	stateIndex = "gguiRouterIndex"
)

// HashHistory keeps the application URL in the browser's fragment, such as
// /#/settings/profile, so static hosting needs no server fallback.
type HashHistory struct {
	history js.Value
	current Entry
	index   int
	length  int
	nextID  uint64
	post    func(func())
	subs    map[int]func(Entry)
	nextSub int
	listen  js.Func
	active  bool
}

// Hash returns a history backed by the browser URL's fragment. The first
// entry is the current fragment, or "/" when it is empty.
func Hash() *HashHistory {
	h := &HashHistory{history: js.Global().Get("history"), subs: map[int]func(Entry){}}
	h.adopt(js.Global().Get("history").Get("state"))
	return h
}

// fragmentURL reads the application URL from location.hash.
func fragmentURL() string {
	s := strings.TrimPrefix(js.Global().Get("location").Get("hash").String(), "#")
	if !strings.HasPrefix(s, "/") {
		return "/" + s
	}
	return s
}

// adopt records the entry the browser is showing. An entry this document
// tagged keeps its ID and index; an untagged one (the first load, or a
// fragment the user typed) is tagged now as the entry after the current.
func (h *HashHistory) adopt(state js.Value) {
	url := fragmentURL()
	if state.Type() == js.TypeObject && state.Get(stateID).Type() == js.TypeNumber {
		h.index = state.Get(stateIndex).Int()
		h.current = Entry{URL: url, ID: uint64(state.Get(stateID).Float())}
		if h.current.ID > h.nextID {
			h.nextID = h.current.ID
		}
		if h.index >= h.length {
			h.length = h.index + 1
		}
		return
	}
	if h.active {
		h.index++
	}
	h.length = h.index + 1
	h.current = h.tag(url, "replaceState", state)
}

// tag writes an entry with our keys merged into the page's state.
func (h *HashHistory) tag(url, method string, prev js.Value) Entry {
	h.nextID++
	state := js.Global().Get("Object").New()
	if prev.Type() == js.TypeObject {
		js.Global().Get("Object").Call("assign", state, prev)
	}
	state.Set(stateID, float64(h.nextID))
	state.Set(stateIndex, h.index)
	h.history.Call(method, state, "", "#"+url)
	return Entry{URL: url, ID: h.nextID}
}

func (h *HashHistory) Current() Entry { return h.current }

func (h *HashHistory) Push(url string) (Entry, error) {
	h.index++
	h.length = h.index + 1
	h.current = h.tag(url, "pushState", js.Null())
	return h.current, nil
}

func (h *HashHistory) Replace(url string) (Entry, error) {
	h.current = h.tag(url, "replaceState", h.history.Get("state"))
	return h.current, nil
}

// Go asks the browser to traverse; the result arrives as a popstate event.
func (h *HashHistory) Go(delta int) {
	if delta != 0 {
		h.history.Call("go", delta)
	}
}

// Position counts only entries this document created or visited; the
// browser does not reveal the others.
func (h *HashHistory) Position() (index, length int) { return h.index, h.length }

// Subscribe listens for popstate. Events are handed to the dispatcher the
// router installs, so subscribers run on the UI thread.
func (h *HashHistory) Subscribe(fn func(Entry)) (stop func()) {
	id := h.nextSub
	h.nextSub++
	h.subs[id] = fn
	if len(h.subs) == 1 {
		h.listen = js.FuncOf(func(this js.Value, args []js.Value) any {
			state := args[0].Get("state")
			deliver := func() {
				h.active = true
				h.adopt(state)
				for _, fn := range h.subs {
					fn(h.current)
				}
			}
			if h.post != nil {
				h.post(deliver)
			} else {
				deliver()
			}
			return nil
		})
		js.Global().Call("addEventListener", "popstate", h.listen)
	}
	return func() {
		delete(h.subs, id)
		if len(h.subs) == 0 {
			js.Global().Call("removeEventListener", "popstate", h.listen)
			h.listen.Release()
		}
	}
}

func (h *HashHistory) setDispatch(post func(func())) { h.post = post }

func defaultHistory() History { return Hash() }
