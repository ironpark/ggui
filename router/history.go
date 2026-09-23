package router

import (
	"fmt"
	"strings"
)

// Entry is one history entry.
type Entry struct {
	URL string
	ID  uint64 // unique per entry, stable across traversal
}

// History stores locations for a Router. Implementations deliver traversal
// events on the UI thread; Push and Replace do not notify subscribers.
type History interface {
	Current() Entry
	Push(url string) (Entry, error)
	Replace(url string) (Entry, error)
	Go(delta int)                        // traversal; may complete asynchronously
	Position() (index, length int)       // entries this adapter knows about
	Subscribe(func(Entry)) (stop func()) // traversal events
}

// MemoryHistory keeps entries in memory, for native applications and
// deterministic tests.
type MemoryHistory struct {
	entries []Entry
	index   int
	nextID  uint64
	subs    map[int]func(Entry)
	nextSub int
}

// Memory returns an in-memory history whose first entry is initialURL.
func Memory(initialURL string) *MemoryHistory {
	h := &MemoryHistory{subs: map[int]func(Entry){}}
	h.entries = []Entry{h.entry(initialURL)}
	return h
}

func (h *MemoryHistory) entry(url string) Entry {
	h.nextID++
	return Entry{URL: url, ID: h.nextID}
}

func (h *MemoryHistory) Current() Entry { return h.entries[h.index] }

func (h *MemoryHistory) Push(url string) (Entry, error) {
	e := h.entry(url)
	h.entries = append(h.entries[:h.index+1], e)
	h.index++
	return e, nil
}

func (h *MemoryHistory) Replace(url string) (Entry, error) {
	e := h.entry(url)
	h.entries[h.index] = e
	return e, nil
}

// Go moves delta entries; it is a no-op past either end.
func (h *MemoryHistory) Go(delta int) {
	next := h.index + delta
	if delta == 0 || next < 0 || next >= len(h.entries) {
		return
	}
	h.index = next
	e := h.entries[next]
	for _, fn := range h.subs {
		fn(e)
	}
}

func (h *MemoryHistory) Position() (index, length int) { return h.index, len(h.entries) }

func (h *MemoryHistory) Subscribe(fn func(Entry)) (stop func()) {
	id := h.nextSub
	h.nextSub++
	h.subs[id] = fn
	return func() { delete(h.subs, id) }
}

// cleanBase validates a Browser base path and returns it without a trailing
// slash, so the root is "".
func cleanBase(basePath string) string {
	if basePath == "" {
		return ""
	}
	u, err := parseURL(basePath)
	if err != nil || u.rawQuery != "" || u.fragment != "" || strings.ContainsAny(basePath, "?#") {
		panic(fmt.Sprintf("router: base path %q must be a path such as /app", basePath))
	}
	if len(u.raw) == 0 {
		return ""
	}
	return u.path()
}

// appURL strips base from a browser path. The base itself, with or without
// its trailing slash, is the application root; a path outside base is kept
// whole.
func appURL(base, pathname string) string {
	switch {
	case base == "":
		return pathname
	case pathname == base:
		return "/"
	case strings.HasPrefix(pathname, base+"/"):
		return pathname[len(base):]
	}
	return pathname
}
