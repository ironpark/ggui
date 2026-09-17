package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// TabsWidget shows one of several pages, picked by a row of labels above.
// Build one with Tabs.
type TabsWidget struct {
	selected *ggui.Signal[int]
	tabs     []TabPage
	onChange func(int)

	labels    []*ggui.TextWidget
	labelSize []ggui.Size
	hovered   int // the label under the pointer, or -1
	focus     focusState
	disabled  bool

	theme     ggui.Theme
	headerH   float64
	pad       ggui.EdgeInsets
	bodySize  ggui.Size
	labelRect []ggui.Rect
}

// TabPage is one page of a Tabs widget; build it with Tab.
type TabPage struct {
	Label   string
	Content ggui.Widget
}

// Tab pairs a label with the page it shows.
func Tab(label string, content ggui.Widget) TabPage { return TabPage{Label: label, Content: content} }

// Tabs creates a tab strip bound to selected, the index of the page shown.
// Click or Space picks the label under the pointer; Left and Right move
// while the strip has focus. Only the selected page is laid out.
func Tabs(selected *ggui.Signal[int], tabs ...TabPage) *TabsWidget {
	t := &TabsWidget{selected: selected, tabs: tabs, hovered: -1}
	for _, tab := range tabs {
		t.labels = append(t.labels, ggui.Text(tab.Label).NoWrap())
	}
	return t
}

// Disabled greys the strip out and ignores input while v is true; the
// selected page stays.
func (t *TabsWidget) Disabled(v bool) *TabsWidget { t.disabled = v; return t }

// OnChange fires with the new index after the user picks a page.
func (t *TabsWidget) OnChange(fn func(int)) *TabsWidget { t.onChange = fn; return t }

func (t *TabsWidget) index() int {
	if len(t.tabs) == 0 {
		return -1
	}
	return int(clamp(float64(t.selected.Peek()), 0, float64(len(t.tabs)-1)))
}

func (t *TabsWidget) pick(i int) {
	if i < 0 || i >= len(t.tabs) || i == t.index() {
		return
	}
	t.selected.Set(i)
	if t.onChange != nil {
		t.onChange(i)
	}
}

// Layout implements Widget.
func (t *TabsWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	th := env.Theme()
	t.theme = th
	t.pad = th.ButtonPad
	t.labelSize = t.labelSize[:0]
	t.headerH = 0
	cur := t.index()
	for i, l := range t.labels {
		l.Color(pick(i == cur && !t.disabled, th.Fg, th.Muted))
		s := l.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
		t.labelSize = append(t.labelSize, s)
		t.headerH = max(t.headerH, s.H+t.pad.Top+t.pad.Bottom)
	}
	t.headerH++ // the divider
	t.bodySize = ggui.Size{}
	if cur >= 0 {
		body := ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MinH: max(c.MinH-t.headerH, 0), MaxH: max(c.MaxH-t.headerH, 0)}
		t.bodySize = t.tabs[cur].Content.Layout(body, env)
	}
	var w float64
	for _, s := range t.labelSize {
		w += s.W + t.pad.Left + t.pad.Right
	}
	return c.Constrain(ggui.Sz(max(w, t.bodySize.W), t.headerH+t.bodySize.H))
}

// Paint implements Widget.
func (t *TabsWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	th := t.theme
	header := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, t.headerH))
	if !t.disabled {
		dst.HitKey(header, t)
	}
	t.labelRect = t.labelRect[:0]
	x := r.Origin.X
	cur := t.index()
	for i, s := range t.labelSize {
		lr := ggui.Rct(ggui.Pt(x, r.Origin.Y), ggui.Sz(s.W+t.pad.Left+t.pad.Right, t.headerH-1))
		t.labelRect = append(t.labelRect, lr)
		if !t.disabled {
			dst.HitPointer(lr, tabLabel{t, i})
			dst.HitCursor(lr, ebiten.CursorShapePointer)
		}
		if i == t.hovered && i != cur && !t.disabled {
			dst.FillRoundRect(lr, th.Radius, th.Surface)
		}
		dst.Paint(t.labels[i], ggui.Rct(ggui.Pt(x+t.pad.Left, r.Origin.Y+t.pad.Top), s))
		x += lr.Size.W
	}
	lineY := r.Origin.Y + t.headerH - 1
	dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, lineY), ggui.Sz(r.Size.W, 1)), th.Border)
	if cur >= 0 {
		lr := t.labelRect[cur]
		x := motion(dst, header, underlineSlot, lr.Origin.X)
		w := motion(dst, header, widthSlot, lr.Size.W)
		dst.FillRoundRect(ggui.Rct(ggui.Pt(x, lineY-1), ggui.Sz(w, 2)), 1, pick(t.disabled, th.Muted, th.Accent))
		if t.focus.focused && t.focus.ring {
			t.focus.paintRing(dst, lr, th.Radius, th)
		}
		dst.Paint(t.tabs[cur].Content, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+t.headerH), t.bodySize))
	}
}

// HandleKey implements KeyHandler: Left and Right move between pages.
func (t *TabsWidget) HandleKey(ev ggui.KeyEvent) {
	t.focus.handle(ev)
	if ev.Kind != ggui.KeyPress || len(t.tabs) == 0 {
		return
	}
	switch ev.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowUp:
		t.pick((t.index() + len(t.tabs) - 1) % len(t.tabs))
	case ebiten.KeyArrowRight, ebiten.KeyArrowDown:
		t.pick((t.index() + 1) % len(t.tabs))
	case ebiten.KeyHome:
		t.pick(0)
	case ebiten.KeyEnd:
		t.pick(len(t.tabs) - 1)
	}
}

// Adopt implements ggui.Adopter.
func (t *TabsWidget) Adopt(prev any) {
	if p, ok := prev.(*TabsWidget); ok {
		t.hovered, t.focus = p.hovered, p.focus
	}
}

// tabLabel is the pointer handler for one label.
type tabLabel struct {
	t *TabsWidget
	i int
}

func (l tabLabel) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		l.t.hovered = l.i
	case ggui.PointerExit:
		if l.t.hovered == l.i {
			l.t.hovered = -1
		}
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft {
			l.t.pick(l.i)
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

func (l tabLabel) Adopt(prev any) {
	if p, ok := prev.(tabLabel); ok && p.t.hovered == p.i {
		l.t.hovered = l.i
	}
}
