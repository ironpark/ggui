package ui

import (
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// TabsWidget shows one of several pages, picked by a row of labels above.
// Build one with Tabs.
type TabsWidget struct {
	line     bool
	selected ggui.Binding[int]
	tabs     []TabPage
	onChange func(int)

	ggui.Interactive // hovered is unused; hover holds the label instead
	labels           []*ggui.TextWidget
	labelSize        []ggui.Size
	hover            int // the label under the pointer, or -1

	theme     ggui.Theme
	motion    time.Duration
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
func Tabs(selected ggui.Binding[int], tabs ...TabPage) *TabsWidget {
	t := &TabsWidget{selected: selected, tabs: tabs, hover: -1}
	t.Role = ggui.RoleTabs
	t.AutoKey()
	for _, tab := range tabs {
		t.labels = append(t.labels, ggui.Text(tab.Label).NoWrap())
	}
	return t
}

// Line uses the underline-style strip instead of the default segmented surface.
func (t *TabsWidget) Line() *TabsWidget { t.line = true; return t }

// Disabled greys the strip out and ignores input while v is true; the
// selected page stays.
func (t *TabsWidget) Disabled(v bool) *TabsWidget { t.Inert = v; return t }

// OnChange fires with the new index after the user picks a page.
func (t *TabsWidget) OnChange(fn func(int)) *TabsWidget { t.onChange = fn; return t }

func (t *TabsWidget) index() int {
	if len(t.tabs) == 0 {
		return -1
	}
	return int(clamp(float64(t.selected.Peek()), 0, float64(len(t.tabs)-1)))
}

func (t *TabsWidget) pick(i int) {
	if i >= 0 && i < len(t.tabs) {
		setChanged(t.selected, i, t.onChange)
	}
}

// Layout implements Widget.
func (t *TabsWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t.Sync()
	th := env.Theme()
	t.theme = th
	t.motion = env.Motion(knobDuration)
	t.pad = ggui.Insets(5, 10)
	t.labelSize = t.labelSize[:0]
	t.headerH = 0
	cur := t.index()
	for i, l := range t.labels {
		l.Color(pick(i == cur && !t.Inert, th.Fg, th.Muted))
		s := l.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env)
		t.labelSize = append(t.labelSize, s)
		t.headerH = max(t.headerH, s.H+t.pad.Top+t.pad.Bottom)
	}
	t.headerH += 14 // three-pixel inset on each side, then eight pixels to content
	t.bodySize = ggui.Size{}
	if cur >= 0 {
		body := ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MinH: max(c.MinH-t.headerH, 0), MaxH: max(c.MaxH-t.headerH, 0)}
		t.bodySize = t.tabs[cur].Content.Layout(body, env)
	}
	w := 6.0
	for _, s := range t.labelSize {
		w += s.W + t.pad.Left + t.pad.Right
	}
	return c.Constrain(ggui.Sz(max(w, t.bodySize.W), t.headerH+t.bodySize.H))
}

// Paint implements Widget. The strip is one semantics node holding a tab
// per label, which no amount of looking at the hit regions could tell:
// the header's key region and the labels' pointer regions are siblings.
func (t *TabsWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.DescribeNode(r, t, func(dst *ggui.Canvas) { t.paint(dst, r) })
}

func (t *TabsWidget) paint(dst *ggui.Canvas, r ggui.Rect) {
	dst = dst.Clip(r)
	th := t.theme
	header := ggui.Rct(r.Origin, ggui.Sz(r.Size.W, t.headerH-8))
	if !t.Inert {
		dst.HitKey(header, t)
	}
	t.labelRect = t.labelRect[:0]
	x := r.Origin.X + 3
	cur := t.index()
	hover := pick(t.Inert, -1, t.hover)
	width := 6.0
	for _, s := range t.labelSize {
		width += s.W + t.pad.Left + t.pad.Right
	}
	if !t.line {
		dst.FillRoundRect(ggui.Rct(r.Origin, ggui.Sz(min(width, r.Size.W), header.Size.H)), th.Radius, mutedSurface(th))
		if cur >= 0 {
			target := r.Origin.X + 3
			for i := 0; i < cur; i++ {
				target += t.labelSize[i].W + t.pad.Left + t.pad.Right
			}
			sx := dst.Ease(ggui.Anchor{Rect: header}, underlineSlot, target, t.motion)
			sw := dst.Ease(ggui.Anchor{Rect: header}, widthSlot, t.labelSize[cur].W+t.pad.Left+t.pad.Right, t.motion)
			active := ggui.Rct(ggui.Pt(sx, r.Origin.Y+3), ggui.Sz(sw, header.Size.H-6))
			dst.Shadow(active, max(th.Radius-2, 0), cardShadow(th))
			dst.FillRoundRect(active, max(th.Radius-2, 0), th.Surface)
		}
	}

	for i, s := range t.labelSize {
		lr := ggui.Rct(ggui.Pt(x, r.Origin.Y+3), ggui.Sz(s.W+t.pad.Left+t.pad.Right, header.Size.H-6))
		t.labelRect = append(t.labelRect, lr)
		dst.Describe(lr, tabLabel{t, i})
		if !t.Inert {
			dst.HitPointer(lr, tabLabel{t, i})
			dst.HitCursor(lr, ebiten.CursorShapePointer)
		}
		if i == hover && i != cur {
			dst.FillRoundRect(lr, th.Radius, subtle(th))
		}
		dst.Paint(t.labels[i], ggui.Rct(ggui.Pt(x+t.pad.Left, r.Origin.Y+3+t.pad.Top), s))
		x += lr.Size.W
	}
	lineY := r.Origin.Y + header.Size.H - 1
	if t.line {
		dst.FillRect(ggui.Rct(ggui.Pt(r.Origin.X, lineY), ggui.Sz(r.Size.W, 1)), th.Border)
	}
	if cur >= 0 {
		lr := t.labelRect[cur]
		if t.line {
			x := dst.Ease(ggui.Anchor{Rect: header}, underlineSlot, lr.Origin.X, t.motion)
			w := dst.Ease(ggui.Anchor{Rect: header}, widthSlot, lr.Size.W, t.motion)
			dst.FillRoundRect(ggui.Rct(ggui.Pt(x, lineY-1), ggui.Sz(w, 2)), 1, pick(t.Inert, th.Muted, th.Accent))
		}
		t.FocusRing(dst, lr, th.Radius, focusColor(th))
		dst.Paint(t.tabs[cur].Content, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+t.headerH), t.bodySize))
	}
}

// HandleKey implements KeyHandler: Left and Right move between pages.
func (t *TabsWidget) HandleKey(ev ggui.KeyEvent) {
	t.Keyboard(ev, nil)
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

// ConsumesKey implements ggui.KeyConsumer: the arrows, Home and End move.
func (t *TabsWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	switch ev.Key {
	case ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyHome, ebiten.KeyEnd:
		return ev.Kind == ggui.KeyPress
	}
	return false
}

// Adopt implements ggui.Adopter.
func (t *TabsWidget) Adopt(prev any) {
	t.Interactive.Adopt(prev)
	if p, ok := prev.(*TabsWidget); ok {
		t.hover = p.hover
	}
}

var (
	underlineSlot = ggui.NewSlot[*ggui.Motion]("underlineSlot")
	widthSlot     = ggui.NewSlot[*ggui.Motion]("widthSlot")
)

// tabLabel is the pointer handler for one label.
type tabLabel struct {
	t *TabsWidget
	i int
}

// Semantics implements ggui.Semantic: each label is a tab.
func (l tabLabel) Semantics() (ggui.Role, string) { return ggui.RoleTab, l.t.tabs[l.i].Label }

// Describe implements ggui.Describer: which tab is the shown one.
func (l tabLabel) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleTab,
		Name:     l.t.tabs[l.i].Label,
		Selected: l.i == l.t.index(),
		Disabled: l.t.Inert,
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

// Act implements ggui.Actor: showing this tab's page.
func (l tabLabel) Act(a ggui.Action) bool {
	if l.t.Inert || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	l.t.pick(l.i)
	return true
}

func (l tabLabel) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		l.t.hover = l.i
	case ggui.PointerExit:
		if l.t.hover == l.i {
			l.t.hover = -1
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
	if p, ok := prev.(tabLabel); ok && p.t.hover == p.i {
		l.t.hover = l.i
	}
}
