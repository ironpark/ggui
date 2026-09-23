package router

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
