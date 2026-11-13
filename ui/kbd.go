package ui

import (
	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// KbdWidget displays a keyboard shortcut without registering an input handler.
type KbdWidget struct {
	text *ggui.TextWidget
	box  *ggui.BoxWidget
}

// Kbd creates a key cap: a small label on the Muted surface, at least as
// wide as it is tall. Put the caps of a chord in a KbdGroup.
func Kbd(label string) *KbdWidget {
	text := ggui.Text(label).NoWrap()
	return &KbdWidget{text: text, box: ggui.Box(&buttonContent{child: text})}
}

// KbdInverseKey marks a subtree drawn on the inverse of the page, as a
// tooltip's is. A Kbd under it is drawn in the page's background color on
// a translucent wash of it, where its usual Muted surface would not show.
var KbdInverseKey = ggui.NewEnvKey[bool]("kbd on an inverse surface")

// Layout implements ggui.Widget.
func (k *KbdWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	fill, ink := t.Muted, t.MutedFg
	if inverse, _ := env.Get(KbdInverseKey); inverse {
		fill, ink = fade(t.Bg, pick(isDark(t.Bg), .1, .2)), t.Bg
	}
	k.text.Style(t.Caption.Merge(t.Label)).Color(ink)
	// shadcn's h-5 min-w-5 px-1 rounded-sm: two and a half Spaces square at
	// the least, and taller only when the text is scaled up.
	side := t.Space * 2.5
	k.box.Pad(0, t.Space/2).Fill(fill).Radius(t.Radius * .6)
	c.MinW, c.MinH = min(c.MaxW, max(c.MinW, side)), min(c.MaxH, max(c.MinH, side))
	return k.box.Layout(c, env)
}

// Baseline implements ggui.Baseliner: the label's.
func (k *KbdWidget) Baseline() (float64, bool) { return k.box.Baseline() }

// Paint implements ggui.Widget.
func (k *KbdWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(k.box, r) }

// KbdGroupWidget is a row of key caps and whatever joins them. Build one
// with KbdGroup.
type KbdGroupWidget struct {
	children []ggui.Widget
	sizes    []ggui.Size
	gap      float64
	rtl      bool
}

// KbdGroup lines up the caps of a chord, or several chords, half a Space
// apart and centred on one line.
//
//	ui.KbdGroup(ui.Kbd("⌘"), ui.Kbd("K"))
//	ui.KbdGroup(ui.Kbd("Ctrl"), ggui.Text("+"), ui.Kbd("B"))
func KbdGroup(children ...ggui.Widget) *KbdGroupWidget { return &KbdGroupWidget{children: children} }

// Layout implements ggui.Widget.
func (g *KbdGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	g.gap = uitheme.Of(env).Space / 2
	g.rtl = env.Direction() == ggui.DirRTL
	g.sizes = g.sizes[:0]
	var w, h float64
	for i, child := range g.children {
		s := child.Layout(c.Loosen(), env)
		g.sizes = append(g.sizes, s)
		w, h = w+s.W+pick(i > 0, g.gap, 0), max(h, s.H)
	}
	return c.Constrain(ggui.Sz(w, h))
}

// Paint implements ggui.Widget.
func (g *KbdGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	x := r.Origin.X
	for i, child := range g.children {
		s := g.sizes[i]
		at := ggui.Rct(ggui.Pt(x, r.Origin.Y+(r.Size.H-s.H)/2), s)
		dst.Paint(child, mirrorHorizontal(at, r, g.rtl))
		x += s.W + g.gap
	}
}
