package widget

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/gem"
	"github.com/ironpark/ggui/property"
)

// StickyWidget is a section of scrolled content whose header pins to the
// start of the enclosing Scroll while the section is in view and is pushed
// out by the section after it. Build one with Sticky.
type StickyWidget struct {
	props      property.Owner
	header     ggui.Widget
	body       ggui.Widget
	port       *ScrollWidget
	horizontal bool
	headerSize ggui.Size
	bodySize   ggui.Size
	pinned     bool
}

// Sticky stacks header above body, both as wide as the section, and keeps
// the header at the top of the nearest Scroll while any of the body is
// still in view; as the section scrolls away, its end pushes the header
// out with it, so consecutive sections hand the top over from one header
// to the next. In a horizontal Scroll the header leads on the left and
// pins to the left edge. Outside a Scroll it is a plain header and body.
//
//	widget.Scroll(widget.Column(
//		widget.Sticky(ui.Title("A"), widget.List(as, row)),
//		widget.Sticky(ui.Title("B"), widget.List(bs, row)),
//	).Stretch())
func Sticky(header, body ggui.Widget) *StickyWidget {
	return &StickyWidget{header: header, body: body}
}

// Pinned reports whether the header was drawn pinned, away from its own
// place, at the last paint: what a header that grows a shadow while it
// floats over the content reads.
func (s *StickyWidget) Pinned() bool { return s.pinned }

func (s *StickyWidget) main(z ggui.Size) float64 { return gem.Pick(s.horizontal, z.W, z.H) }

// Layout implements Widget.
func (s *StickyWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	s.port = scrollPortOf(env)
	s.horizontal = false
	if vp, ok := ScrollViewportOf(env); ok {
		s.horizontal = vp.Horizontal
	}
	// Across the axis both stretch to the section, so a pinned header
	// covers the rows passing under it.
	cross := gem.Pick(s.horizontal, c.MaxH, c.MaxW)
	child := func(w ggui.Widget, crossMin float64) ggui.Size {
		if s.horizontal {
			return w.Layout(ggui.Constraints{MinH: crossMin, MaxW: ggui.Unbounded, MaxH: cross}, env)
		}
		return w.Layout(ggui.Constraints{MinW: crossMin, MaxW: cross, MaxH: ggui.Unbounded}, env)
	}
	stretch := gem.Pick(cross >= ggui.Unbounded, 0, cross)
	s.headerSize = child(s.header, stretch)
	s.bodySize = child(s.body, stretch)
	if stretch == 0 {
		// Unbounded across: stretch whichever child came out narrower to match the wider.
		hw := gem.Pick(s.horizontal, s.headerSize.H, s.headerSize.W)
		bw := gem.Pick(s.horizontal, s.bodySize.H, s.bodySize.W)
		w := max(hw, bw)
		if hw < w {
			s.headerSize = child(s.header, w)
		}
		if bw < w {
			s.bodySize = child(s.body, w)
		}
	}
	if s.horizontal {
		return c.Constrain(ggui.Sz(s.headerSize.W+s.bodySize.W, max(s.headerSize.H, s.bodySize.H)))
	}
	return c.Constrain(ggui.Sz(max(s.headerSize.W, s.bodySize.W), s.headerSize.H+s.bodySize.H))
}

// Paint implements Widget. The body paints first so the header, painted
// over it, takes the input where the two overlap.
func (s *StickyWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	lead := s.main(s.headerSize)
	body := ggui.Rct(r.Origin.Add(gem.Pick(s.horizontal, ggui.Pt(lead, 0), ggui.Pt(0, lead))), s.bodySize)
	dst.Paint(s.body, body)
	at := pinAt(s.port, s.horizontal, r, lead)
	s.pinned = at != 0
	header := ggui.Rct(r.Origin.Add(gem.Pick(s.horizontal, ggui.Pt(at, 0), ggui.Pt(0, at))), s.headerSize)
	dst.Paint(s.header, header)
}

// pinAt returns how far past its own place a header of length lead moves
// to stay at the start of port's window, staying inside the section r so
// the section's end pushes it out.
func pinAt(port *ScrollWidget, horizontal bool, r ggui.Rect, lead float64) float64 {
	edge, ok := portEdge(port, horizontal, r)
	if !ok {
		return 0
	}
	return pinPos(edge, 0, gem.Pick(horizontal, r.Size.W, r.Size.H), lead)
}

// portEdge returns how far past the start of r, along the axis, port's
// window starts, and false when there is no port scrolling along it.
func portEdge(port *ScrollWidget, horizontal bool, r ggui.Rect) (float64, bool) {
	if port == nil || port.horizontal != horizontal {
		return 0, false
	}
	return gem.Pick(horizontal, port.rect.Origin.X-r.Origin.X, port.rect.Origin.Y-r.Origin.Y), true
}

// pinPos returns where a header of length lead, in a section from start to
// end, sits when the window starts at edge: at the edge, but no earlier
// than its own place and no later than lets the section's end push it out.
func pinPos(edge, start, end, lead float64) float64 {
	return max(min(edge, end-lead), start)
}
