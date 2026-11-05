package ggui

import (
	"slices"
	"sort"

	"github.com/ironpark/ggui/pkg/virtual"
)

// The parts of EachWidget that work with an enclosing Scroll: section
// headers pinned at the top of its window, and the rows a
// ScrollController asks for by key.

// eachPin is what an EachWidget keeps for sticky headers and for answering
// a ScrollController's ScrollToKey.
type eachPin[T any] struct {
	header func(T) bool
	port   *ScrollWidget
	row    int // virtual path: the header laid out for the window's start, or -1
	size   Size
	pitch  float64
	gap    float64

	// headers are the indices of the header rows among the items of
	// generation gen, found once when the items change rather than by
	// asking isHeader of every row back from the window each frame.
	headers []int
	gen     uint64
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

// headerRows returns the indices of the header rows, in order, finding
// them again only when the items have changed since.
func (f *EachWidget[T, K]) headerRows() []int {
	p := &f.pin
	// The items are set before the first layout, so their generation is
	// never the zero that a pin which has found nothing starts at.
	if p.gen != f.itemsGen {
		p.headers = p.headers[:0]
		for i, item := range f.items {
			if p.header(item) {
				p.headers = append(p.headers, i)
			}
		}
		p.gen = f.itemsGen
	}
	return p.headers
}

// headerBefore returns the last header at or before item i, or -1.
func (f *EachWidget[T, K]) headerBefore(i int) int {
	hs := f.headerRows()
	if j := sort.SearchInts(hs, i+1) - 1; j >= 0 {
		return hs[j]
	}
	return -1
}

// headerAfter returns the first header after item i, or -1.
func (f *EachWidget[T, K]) headerAfter(i int) int {
	hs := f.headerRows()
	if j := sort.SearchInts(hs, i+1); j < len(hs) {
		return hs[j]
	}
	return -1
}

// windowStart returns how far into the list the window of its Scroll
// starts, along the list's axis, given where the list is painted; ok is
// false when the list's own start is in view.
func (f *EachWidget[T, K]) windowStart(r Rect) (float64, bool) {
	edge, ok := portEdge(f.pin.port, f.horizontal, r)
	return edge, ok && edge > 0
}

// mainAt and crossAt return where row i of those laid out starts, along
// and across the list's axis, from the list's corner.
func (f *EachWidget[T, K]) mainAt(i int) float64 {
	return pick(f.horizontal, f.rects[i].Origin.X, f.rects[i].Origin.Y)
}

func (f *EachWidget[T, K]) crossAt(i int) float64 {
	return pick(f.horizontal, f.rects[i].Origin.Y, f.rects[i].Origin.X)
}

// layoutPinned lays out, on the virtual path, the header of the section at
// the window's start when it is not among the rows in view.
func (f *EachWidget[T, K]) layoutPinned(pitch, gap, crossMin, crossMax float64, env Env) {
	f.pin.row, f.pin.pitch, f.pin.gap = -1, pitch, gap
	_, ok := viewportAlong(env, f.horizontal)
	n := len(f.items)
	if f.pin.header == nil || !ok || n == 0 || pitch <= 0 {
		return
	}
	// f.first is the row virtual.Fixed found the window starting on.
	h := f.headerBefore(min(f.first, n-1))
	if h < 0 {
		return
	}
	f.pin.row = h
	if h >= f.first && h < f.last {
		f.pin.size = f.rects[h-f.first].Size
		return
	}
	e := f.entry(h)
	e.seen = f.frame
	f.pin.size = e.widget.Layout(f.constraints(f.extent, f.extent, crossMin, crossMax), env)
}

// pinned returns the header row to draw pinned at the window's start, its
// index and where it goes, or nil when no header has to move.
func (f *EachWidget[T, K]) pinned(r Rect) (*forEntry[T], int, Rect) {
	if f.pin.header == nil || len(f.items) == 0 || f.estimated() {
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
		first, last := virtual.Fixed(n, pitch, edge, f.extent)
		h := f.headerBefore(min(first, n-1))
		if h < 0 || h != f.pin.row {
			return nil, 0, Rect{}
		}
		start := float64(h) * pitch
		end := max(float64(n)*pitch-f.pin.gap, start)
		// Only a header that reaches the window's start can push.
		if next := f.headerAfter(h); next >= 0 && next < min(n, last+1) {
			end = float64(next)*pitch - f.pin.gap
		}
		pos := pinPos(edge, start, end, f.extent)
		if pos == start {
			return nil, 0, Rect{}
		}
		cross := 0.0
		if h >= f.first && h < f.last {
			cross = f.crossAt(h - f.first)
		}
		return f.entries[f.keys[h]], h, Rct(at(pos, cross), f.pin.size)
	}
	// Every row is laid out, rows leaving among them: find the item the
	// window starts on, the header at or before it, and the end of its
	// section, the end of the child before the next header's, or of the
	// last child.
	n := min(len(f.childEntries), len(f.childItem), len(f.rects))
	row := sort.Search(n, func(i int) bool { return f.mainAt(i) > edge }) - 1
	if row < 0 || f.childItem[row] < 0 {
		return nil, 0, Rect{}
	}
	h := f.headerBefore(f.childItem[row])
	if h < 0 {
		return nil, 0, Rect{}
	}
	head := f.childOf(h, n)
	last := n - 1
	if next := f.headerAfter(h); next >= 0 {
		last = f.childOf(next, n) - 1
	}
	start := f.mainAt(head)
	lead := f.main(f.rects[head].Size)
	end := max(start+lead, f.mainAt(last)+f.main(f.rects[last].Size))
	pos := pinPos(edge, start, end, lead)
	if pos == start {
		return nil, 0, Rect{}
	}
	return f.childEntries[head], h, Rct(at(pos, f.crossAt(head)), f.rects[head].Size)
}

// childOf returns the child painting item i, of the first n, or n when it
// is past them: the first whose childItem reaches i, which is the item's
// own, a leaving row after it carrying its index too.
func (f *EachWidget[T, K]) childOf(i, n int) int {
	return sort.Search(n, func(c int) bool { return f.childItem[c] >= i })
}

// placeKey answers the enclosing Scroll's ScrollToKey when the key is one
// of this list's rows.
func (f *EachWidget[T, K]) placeKey(r Rect) {
	p := f.pin.port
	if p == nil || p.horizontal != f.horizontal || !p.keyPending() {
		return
	}
	p.placeKey(func(k any) (float64, bool) {
		key, ok := k.(K)
		if !ok {
			return 0, false
		}
		start := pick(f.horizontal, r.Origin.X, r.Origin.Y)
		if f.extent > 0 {
			i := slices.Index(f.keys, key)
			return start + float64(i)*f.pin.pitch, i >= 0
		}
		if f.estimated() {
			at, ok := f.est.Start(slices.Index(f.keys, key))
			return start + at, ok
		}
		e := f.entries[key]
		if e == nil {
			return 0, false
		}
		i := slices.Index(f.childEntries, e)
		if i < 0 {
			return 0, false
		}
		return start + f.mainAt(i), true
	})
}
