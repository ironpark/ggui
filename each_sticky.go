package ggui

import "math"

// eachPin is what an EachWidget keeps for sticky headers and for answering
// a ScrollController's ScrollToKey.
type eachPin[T any] struct {
	header func(T) bool
	port   *ScrollWidget
	row    int // virtual path: the header laid out for the window's start, or -1
	size   Size
	pitch  float64
	gap    float64
}

// StickyHeaders makes the rows isHeader picks section headers: inside a
// Scroll, the header of the section at the top of the window stays pinned
// there, over the rows passing under it, until the next header pushes it
// out. It works with ItemExtent too, where the pinned header is built even
// when its own row has scrolled far out of view. Configure it before
// mount.
//
//	ggui.EachKeyed(contacts, Contact.Key, row).ItemExtent(40).
//		StickyHeaders(func(c Contact) bool { return c.Letter })
func (f *EachWidget[T, K]) StickyHeaders(isHeader func(T) bool) *EachWidget[T, K] {
	f.checkConfig()
	f.pin.header = isHeader
	return f
}

func (f *EachWidget[T, K]) isHeader(i int) bool { return f.pin.header(f.items[i]) }

// headerBefore returns the last header at or before item i, or -1.
func (f *EachWidget[T, K]) headerBefore(i int) int {
	for ; i >= 0; i-- {
		if f.isHeader(i) {
			return i
		}
	}
	return -1
}

// windowStart returns how far into the list the window of its Scroll
// starts, along the list's axis, given where the list is painted; ok is
// false when the list's own start is in view.
func (f *EachWidget[T, K]) windowStart(r Rect) (float64, bool) {
	p := f.pin.port
	if p == nil || p.horizontal != f.horizontal {
		return 0, false
	}
	edge := pick(f.horizontal, p.rect.Origin.X-r.Origin.X, p.rect.Origin.Y-r.Origin.Y)
	return edge, edge > 0
}

// layoutPinned lays out, on the virtual path, the header of the section at
// the window's start when it is not among the rows in view. Virtualization
// takes the list to start where the Scroll's content does, and so does
// this.
func (f *EachWidget[T, K]) layoutPinned(pitch, gap, crossMin, crossMax float64, env Env) {
	f.pin.row, f.pin.pitch, f.pin.gap = -1, pitch, gap
	vp, ok := ScrollViewport(env)
	n := len(f.items)
	if f.pin.header == nil || !ok || vp.Horizontal != f.horizontal || n == 0 || pitch <= 0 {
		return
	}
	h := f.headerBefore(clamp(int(math.Floor(vp.Offset/pitch)), 0, n-1))
	if h < 0 {
		return
	}
	f.pin.row = h
	if h >= f.first && h < f.last {
		f.pin.size = f.sizes[h-f.first]
		return
	}
	e := f.entry(h)
	e.seen = f.frame
	f.pin.size = e.widget.Layout(f.constraints(f.extent, f.extent, crossMin, crossMax), env)
}

// pinned returns the header row to draw pinned at the window's start, its
// index and where it goes, or nil when no header has to move.
func (f *EachWidget[T, K]) pinned(r Rect) (*forEntry[T], int, Rect) {
	if f.pin.header == nil || len(f.items) == 0 {
		return nil, 0, Rect{}
	}
	edge, ok := f.windowStart(r)
	if !ok {
		return nil, 0, Rect{}
	}
	at := func(main, cross float64) Point {
		return r.Origin.Add(pick(f.horizontal, Pt(main, cross), Pt(cross, main)))
	}
	if f.extent > 0 {
		pitch, n := f.pin.pitch, len(f.items)
		h := f.headerBefore(clamp(int(math.Floor(edge/pitch)), 0, n-1))
		if h < 0 || h != f.pin.row {
			return nil, 0, Rect{}
		}
		start := float64(h) * pitch
		end := max(float64(n)*pitch-f.pin.gap, start)
		// Only a header that reaches the window's start can push.
		for i, last := h+1, min(n, int(math.Ceil((edge+f.extent)/pitch))+1); i < last; i++ {
			if f.isHeader(i) {
				end = float64(i)*pitch - f.pin.gap
				break
			}
		}
		pos := max(min(edge, end-f.extent), start)
		if pos == start {
			return nil, 0, Rect{}
		}
		cross := 0.0
		if h >= f.first && h < f.last {
			cross = f.cross(Size{W: f.offsets[h-f.first].X, H: f.offsets[h-f.first].Y})
		}
		return f.entries[f.keys[h]], h, Rct(at(pos, cross), f.pin.size)
	}
	// Every row is laid out: find the header above the window's start and
	// the end of its section, which is where the next header's row begins.
	head := -1
	for i, e := range f.childEntries {
		main := f.main(Size{W: f.offsets[i].X, H: f.offsets[i].Y})
		if main > edge {
			break
		}
		if f.entries[f.keys[clamp(Untrack(e.position.Get), 0, len(f.keys)-1)]] == e && f.pin.header(Untrack(e.item.Get)) {
			head = i
		}
	}
	if head < 0 {
		return nil, 0, Rect{}
	}
	start := f.main(Size{W: f.offsets[head].X, H: f.offsets[head].Y})
	lead := f.main(f.sizes[head])
	end := start + lead
	for i := head + 1; i < len(f.childEntries); i++ {
		e := f.childEntries[i]
		if f.pin.header(Untrack(e.item.Get)) {
			break
		}
		end = f.main(Size{W: f.offsets[i].X, H: f.offsets[i].Y}) + f.main(f.sizes[i])
	}
	pos := max(min(edge, end-lead), start)
	if pos == start {
		return nil, 0, Rect{}
	}
	e := f.childEntries[head]
	cross := f.cross(Size{W: f.offsets[head].X, H: f.offsets[head].Y})
	return e, Untrack(e.position.Get), Rct(at(pos, cross), f.sizes[head])
}

// placeKey answers the enclosing Scroll's ScrollToKey when the key is one
// of this list's rows.
func (f *EachWidget[T, K]) placeKey(r Rect) {
	p := f.pin.port
	if p == nil || p.horizontal != f.horizontal {
		return
	}
	p.placeKey(func(k any) (float64, bool) {
		key, ok := k.(K)
		if !ok {
			return 0, false
		}
		start := pick(f.horizontal, r.Origin.X, r.Origin.Y)
		if f.extent > 0 {
			for i, have := range f.keys {
				if have == key {
					return start + float64(i)*f.pin.pitch, true
				}
			}
			return 0, false
		}
		e := f.entries[key]
		if e == nil {
			return 0, false
		}
		for i, c := range f.childEntries {
			if c == e {
				return start + f.main(Size{W: f.offsets[i].X, H: f.offsets[i].Y}), true
			}
		}
		return 0, false
	})
}
