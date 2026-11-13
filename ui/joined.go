package ui

import (
	"image/color"
	"math"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// JoinedSides are the sides on which a control meets a neighbour it is
// joined to, as the children of a ButtonGroup are. A control squares the
// corners of a joined side, and leaves out its border on a joined Left or
// Top side, where the neighbour's border already draws the seam.
type JoinedSides struct{ Left, Top, Right, Bottom bool }

// Any reports whether any side is joined.
func (j JoinedSides) Any() bool { return j.Left || j.Top || j.Right || j.Bottom }

// JoinedSidesKey holds the JoinedSides of the control being laid out. A
// ButtonGroup sets it for each child; Button, Toggle, ButtonGroupText, the
// text fields and Select read it, and a control of your own may.
//
// The sides are the ones in the window, so a control need not look at the
// group's orientation or the reading direction. A control keeps what it
// read at layout and, when it paints, drops it unless it is painted by the
// group itself: the content of a popup opened from a child inherits the
// environment but is not part of the strip.
var JoinedSidesKey = ggui.NewEnvKey[JoinedSides]("joined sides")

// joinedSidesOf is the JoinedSides a ButtonGroup set for the control laid
// out under env.
func joinedSidesOf(env ggui.Env) JoinedSides {
	j, _ := env.Get(JoinedSidesKey)
	return j
}

// takeJoinedSides is joinedSidesOf, and env without them for the control's
// own parts: the sides are the control's, and a button inside a field the
// group joins is not itself joined.
func takeJoinedSides(env ggui.Env) (JoinedSides, ggui.Env) {
	j, ok := env.Get(JoinedSidesKey)
	if !ok || !j.Any() {
		return j, env
	}
	return j, env.With(JoinedSidesKey, JoinedSides{})
}

// within is j while dst paints a child of a ButtonGroup, and no sides
// anywhere else: in a popup, which is laid out under the environment of
// the child that opened it.
func (j JoinedSides) within(dst *ggui.Canvas) JoinedSides {
	if !j.Any() || dst == nil {
		return JoinedSides{}
	}
	if _, ok := dst.Enclosing().(*ButtonGroupWidget); ok {
		return j
	}
	return JoinedSides{}
}

// joinedPart is one part of a rounded box whose corners share a radius.
type joinedPart struct {
	clip   ggui.Rect
	radius float64
}

// joinedParts cuts r into the fewest parts within which every corner has
// the same radius: one where nothing is joined or everything is, two
// halves for a control at the end of a strip, four quarters otherwise. The
// core rounds every corner of a box alike, so a box with mixed corners is
// drawn once per part, clipped to it. The cuts fall on whole pixels of a
// canvas at scale, so no pixel is drawn by two parts and a translucent
// fill shows no seam.
func joinedParts(r ggui.Rect, radius float64, j JoinedSides, scale float64) (parts [4]joinedPart, n int) {
	tl, tr := joinedCorner(radius, j.Left, j.Top), joinedCorner(radius, j.Right, j.Top)
	bl, br := joinedCorner(radius, j.Left, j.Bottom), joinedCorner(radius, j.Right, j.Bottom)
	if tl == tr && tl == bl && tl == br {
		parts[0] = joinedPart{r, tl}
		return parts, 1
	}
	midX := math.Round((r.Origin.X+r.Size.W/2)*scale) / scale
	midY := math.Round((r.Origin.Y+r.Size.H/2)*scale) / scale
	left, right := midX-r.Origin.X, r.Origin.X+r.Size.W-midX
	top, bottom := midY-r.Origin.Y, r.Origin.Y+r.Size.H-midY
	switch {
	case tl == bl && tr == br:
		parts[0] = joinedPart{ggui.Rct(r.Origin, ggui.Sz(left, r.Size.H)), tl}
		parts[1] = joinedPart{ggui.Rct(ggui.Pt(midX, r.Origin.Y), ggui.Sz(right, r.Size.H)), tr}
		return parts, 2
	case tl == tr && bl == br:
		parts[0] = joinedPart{ggui.Rct(r.Origin, ggui.Sz(r.Size.W, top)), tl}
		parts[1] = joinedPart{ggui.Rct(ggui.Pt(r.Origin.X, midY), ggui.Sz(r.Size.W, bottom)), bl}
		return parts, 2
	}
	parts[0] = joinedPart{ggui.Rct(r.Origin, ggui.Sz(left, top)), tl}
	parts[1] = joinedPart{ggui.Rct(ggui.Pt(midX, r.Origin.Y), ggui.Sz(right, top)), tr}
	parts[2] = joinedPart{ggui.Rct(ggui.Pt(r.Origin.X, midY), ggui.Sz(left, bottom)), bl}
	parts[3] = joinedPart{ggui.Rct(ggui.Pt(midX, midY), ggui.Sz(right, bottom)), br}
	return parts, 4
}

// joinedCorner is the radius of a corner between two sides: none where
// either is joined.
func joinedCorner(radius float64, a, b bool) float64 {
	if a || b {
		return 0
	}
	return radius
}

// fillJoined fills r with col, its corners rounded by radius except on the
// joined sides.
func fillJoined(dst *ggui.Canvas, r ggui.Rect, radius float64, j JoinedSides, col color.Color) {
	if col == nil {
		return
	}
	parts, n := joinedParts(r, radius, j, dst.Scale())
	if n == 1 {
		dst.FillRoundRect(r, parts[0].radius, col)
		return
	}
	for _, p := range parts[:n] {
		dst.Clip(p.clip).FillRoundRect(r, p.radius, col)
	}
}

// strokeJoined draws a line of width w just inside r, its corners rounded
// by radius except on the joined sides. With seam false it leaves the line
// out on a joined Left or Top side, as a control's own border does; a
// focus ring passes true and is drawn all the way round.
func strokeJoined(dst *ggui.Canvas, r ggui.Rect, radius float64, j JoinedSides, w float64, col color.Color, seam bool) {
	if col == nil || w <= 0 {
		return
	}
	parts, n := joinedParts(r, radius, j, dst.Scale())
	// The line on a side left out is drawn beyond r, where the clip drops
	// it. Its corners are square, so the two beside it run to the edge.
	line := r
	if !seam && j.Left {
		line.Origin.X -= w
		line.Size.W += w
	}
	if !seam && j.Top {
		line.Origin.Y -= w
		line.Size.H += w
	}
	if n == 1 && line == r {
		dst.StrokeRoundRect(r, parts[0].radius, w, col)
		return
	}
	for _, p := range parts[:n] {
		dst.Clip(p.clip).StrokeRoundRect(line, p.radius, w, col)
	}
}

// paintJoinedBox draws a control's surface and border in r, following the
// joined sides: what a Box with a fill, a border and a radius draws, with
// the corners squared and the border left out where the control meets its
// neighbour.
func paintJoinedBox(dst *ggui.Canvas, r ggui.Rect, radius float64, j JoinedSides, fill color.Color, borderWidth float64, border color.Color) {
	fillJoined(dst, r, radius, j, fill)
	strokeJoined(dst, r, radius, j, borderWidth, border, false)
}

// joinedFocus is a focus ring: the line at the edge of a control and the
// soft halo around it, following its joined sides.
type joinedFocus struct {
	on         bool
	r          ggui.Rect
	radius     float64
	sides      JoinedSides
	width      float64
	line, halo color.Color
}

func (f *joinedFocus) paint(dst *ggui.Canvas) {
	strokeJoined(dst, f.r, f.radius, f.sides, f.width, f.line, true)
	strokeJoined(dst, f.r.Grow(2), f.radius+2, f.sides, 3, f.halo, true)
}

// joinedFocusRing draws the focus ring of a control: line at its edge and
// ring, at the theme's halo opacity, around it. Inside a ButtonGroup the
// ring is handed to the group, which draws it over the children painted
// after this one; they would otherwise cover the half of it on their side.
func joinedFocusRing(dst *ggui.Canvas, t *uitheme.Theme, r ggui.Rect, radius float64, j JoinedSides, line, ring color.Color) {
	f := joinedFocus{on: true, r: r, radius: radius, sides: j, width: t.BorderWidth, line: line, halo: t.Halo(ring)}
	if g, ok := dst.Enclosing().(*ButtonGroupWidget); ok && j.Any() {
		g.focus = f
		return
	}
	f.paint(dst)
}
