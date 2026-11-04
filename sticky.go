package ggui

import (
	"math"
	"slices"

	"github.com/ironpark/ggui/internal/property"
)

// StickyWidget is a section of scrolled content whose header pins to the
// start of the enclosing Scroll while the section is in view and is pushed
// out by the section after it. Build one with Sticky.
type StickyWidget struct {
	props      property.Owner
	header     Widget
	body       Widget
	port       *ScrollWidget
	horizontal bool
	headerSize Size
	bodySize   Size
	pinned     bool
}

// Sticky stacks header above body, both as wide as the section, and keeps
// the header at the top of the nearest Scroll while any of the body is
// still in view; as the section scrolls away, its end pushes the header
// out with it, so consecutive sections hand the top over from one header
// to the next. In a horizontal Scroll the header leads on the left and
// pins to the left edge. Outside a Scroll it is a plain header and body.
//
//	ggui.Scroll(ggui.Column(
//		ggui.Sticky(ui.Title("A"), ggui.List(as, row)),
//		ggui.Sticky(ui.Title("B"), ggui.List(bs, row)),
//	).Stretch())
func Sticky(header, body Widget) *StickyWidget {
	return &StickyWidget{header: header, body: body}
}

// Pinned reports whether the header was drawn pinned, away from its own
// place, at the last paint: what a header that grows a shadow while it
// floats over the content reads.
func (s *StickyWidget) Pinned() bool { return s.pinned }

func (s *StickyWidget) main(z Size) float64 { return pick(s.horizontal, z.W, z.H) }

// Layout implements Widget.
func (s *StickyWidget) Layout(c Constraints, env Env) Size {
	defer s.props.Layout().EndLayout()
	s.port = scrollPortOf(env)
	s.horizontal = false
	if vp, ok := ScrollViewport(env); ok {
		s.horizontal = vp.Horizontal
	}
	// Across the axis both stretch to the section, so a pinned header
	// covers the rows passing under it.
	cross := pick(s.horizontal, c.MaxH, c.MaxW)
	child := func(w Widget, crossMin float64) Size {
		if s.horizontal {
			return w.Layout(Constraints{MinH: crossMin, MaxW: Unbounded, MaxH: cross}, env)
		}
		return w.Layout(Constraints{MinW: crossMin, MaxW: cross, MaxH: Unbounded}, env)
	}
	stretch := pick(cross >= Unbounded, 0, cross)
	s.headerSize = child(s.header, stretch)
	s.bodySize = child(s.body, stretch)
	if stretch == 0 {
		// Unbounded across: both take the wider of the two.
		w := max(pick(s.horizontal, s.headerSize.H, s.headerSize.W), pick(s.horizontal, s.bodySize.H, s.bodySize.W))
		s.headerSize = child(s.header, w)
		s.bodySize = child(s.body, w)
	}
	if s.horizontal {
		return c.Constrain(Sz(s.headerSize.W+s.bodySize.W, max(s.headerSize.H, s.bodySize.H)))
	}
	return c.Constrain(Sz(max(s.headerSize.W, s.bodySize.W), s.headerSize.H+s.bodySize.H))
}

// Paint implements Widget. The body paints first so the header, painted
// over it, takes the input where the two overlap.
func (s *StickyWidget) Paint(dst *Canvas, r Rect) {
	lead := s.main(s.headerSize)
	body := Rct(r.Origin.Add(pick(s.horizontal, Pt(lead, 0), Pt(0, lead))), s.bodySize)
	dst.Paint(s.body, body)
	at := pinAt(s.port, s.horizontal, r, lead)
	s.pinned = at != 0
	header := Rct(r.Origin.Add(pick(s.horizontal, Pt(at, 0), Pt(0, at))), s.headerSize)
	dst.Paint(s.header, header)
}

// pinAt returns how far past its own place a header of length lead moves
// to stay at the start of port's window, staying inside the section r so
// the section's end pushes it out.
func pinAt(port *ScrollWidget, horizontal bool, r Rect, lead float64) float64 {
	edge, ok := portEdge(port, horizontal, r)
	if !ok {
		return 0
	}
	return pinPos(edge, 0, pick(horizontal, r.Size.W, r.Size.H), lead)
}

// portEdge returns how far past the start of r, along the axis, port's
// window starts, and false when there is no port scrolling along it.
func portEdge(port *ScrollWidget, horizontal bool, r Rect) (float64, bool) {
	if port == nil || port.horizontal != horizontal {
		return 0, false
	}
	return pick(horizontal, port.rect.Origin.X-r.Origin.X, port.rect.Origin.Y-r.Origin.Y), true
}

// pinPos returns where a header of length lead, in a section from start to
// end, sits when the window starts at edge: at the edge, but no earlier
// than its own place and no later than lets the section's end push it out.
func pinPos(edge, start, end, lead float64) float64 {
	return max(min(edge, end-lead), start)
}

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
	vp, ok := viewportAlong(env, f.horizontal)
	n := len(f.items)
	if f.pin.header == nil || !ok || n == 0 || pitch <= 0 {
		return
	}
	h := f.headerBefore(clamp(int(math.Floor((vp.Offset-f.lead)/pitch)), 0, n-1))
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
	// Every row is laid out: find the header above the window's start and
	// the end of its section, which is where the next header's row begins.
	head := -1
	for i, e := range f.childEntries {
		if f.mainAt(i) > edge {
			break
		}
		if f.entries[f.keys[clamp(Untrack(e.position.Get), 0, len(f.keys)-1)]] == e && f.pin.header(Untrack(e.item.Get)) {
			head = i
		}
	}
	if head < 0 {
		return nil, 0, Rect{}
	}
	start := f.mainAt(head)
	lead := f.main(f.rects[head].Size)
	end := start + lead
	for i := head + 1; i < len(f.childEntries); i++ {
		e := f.childEntries[i]
		if f.pin.header(Untrack(e.item.Get)) {
			break
		}
		end = f.mainAt(i) + f.main(f.rects[i].Size)
	}
	pos := pinPos(edge, start, end, lead)
	if pos == start {
		return nil, 0, Rect{}
	}
	e := f.childEntries[head]
	cross := f.crossAt(head)
	return e, Untrack(e.position.Get), Rct(at(pos, cross), f.rects[head].Size)
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
