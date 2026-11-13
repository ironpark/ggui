package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

const (
	tabInset = 3.0 // strip padding all round the label row
	tabGap   = 4.0 // between the labels of a line strip
	tabBar   = 2.0 // the bar marking the shown label of a line strip
	// How far a horizontal strip's bar reaches past the strip's foot:
	// the trigger's padding box ends 4.5 above it and the bar 5 below that.
	tabBarDrop = 0.5
)

// TabsWidget shows one of several pages, picked by a row of labels above.
// Build one with Tabs.
type TabsWidget struct {
	props           property.Owner
	line            bool
	rtl             bool
	vertical        bool
	activateOnFocus bool
	active          int
	focusPending    bool
	stripH          float64
	selected        ggui.Binding[int]
	tabs            []TabPage
	onChange        func(int)

	ggui.Interactive                    // hovered is unused; hover holds the label instead
	boxes            []*ggui.BoxWidget  // each label, padded
	heads            []ggui.Widget      // each box, in its Tooltip if it has one
	labels           []*ggui.TextWidget // each label's own text, nil under a Header
	labelSize        []ggui.Size        // padding included
	hover            int                // the label under the pointer, or -1

	theme     *uitheme.Theme
	motion    time.Duration
	headerH   float64
	stripW    float64   // the strip's natural width, inset included
	labelX    []float64 // each label's offset from the strip's left inset
	shown     []float64 // how far each label is into being the shown one, as last painted
	pad       ggui.EdgeInsets
	bodySize  ggui.Size
	labelRect []ggui.Rect
}

// TabPage is one page of a Tabs widget; build it with Tab.
type TabPage struct {
	Label   string
	Content ggui.Widget

	header   ggui.Widget
	tooltip  string
	disabled bool
}

// Tab pairs a label with the page it shows.
func Tab(label string, content ggui.Widget) TabPage { return TabPage{Label: label, Content: content} }

// Header draws w in the strip in place of the label's text, for a tab with
// an icon, a count or a badge:
//
//	ui.Tab("Build", build).Header(ggui.Row(ggui.Text("Build"), ui.Badge("3")).Gap(6))
//
// The label still names the tab to screen readers and Probe. Text in w
// takes the strip's colors unless it sets its own.
func (p TabPage) Header(w ggui.Widget) TabPage { p.header = w; return p }

// Tooltip shows s when the pointer rests on the tab, as Tooltip does.
func (p TabPage) Tooltip(s string) TabPage { p.tooltip = s; return p }

// Tabs creates a tab strip bound to selected, the index of the page shown.
// Click selects a page. Arrows move focus, and Space or Enter activates it.
// ActivateOnFocus(true) enables automatic selection. Only the selected page is laid out.
func Tabs(selected ggui.Binding[int], tabs ...TabPage) *TabsWidget {
	t := &TabsWidget{selected: selected, tabs: tabs, hover: -1, active: -1}
	t.Role = ggui.RoleTabs
	t.AutoKey()
	for i, tab := range tabs {
		var head ggui.Widget
		if tab.header != nil {
			head = tab.header
			t.labels = append(t.labels, nil)
		} else {
			label := ggui.Text(tab.Label).NoWrap()
			head = label
			t.labels = append(t.labels, label)
		}
		box := ggui.Box(head)
		t.boxes = append(t.boxes, box)
		if tab.tooltip != "" {
			// Keyboard focus shows the roving tab's tooltip.
			tip := Tooltip(box, tab.tooltip)
			tip.focusOpens = func(dst *ggui.Canvas, r ggui.Rect) bool {
				return i == t.focusIndex() && dst.HasFocus(tabLabel{t, i}) && dst.FocusVisible()
			}
			t.heads = append(t.heads, tip)
		} else {
			t.heads = append(t.heads, box)
		}
	}
	return t
}

// Line uses the underline-style strip instead of the default segmented
// surface: bare labels a little apart, with a bar in the text color under
// the shown one, or beside it in a Vertical strip.
func (t *TabsWidget) Line() *TabsWidget {
	defer property.Watch(&t.props, &t.line)()
	t.line = true
	return t
}

// Vertical places the tab list beside the active page.
func (t *TabsWidget) Vertical() *TabsWidget {
	defer property.Watch(&t.props, &t.vertical)()
	t.vertical = true
	return t
}

// ActivateOnFocus selects pages while navigating with arrows. By default arrows
// move focus only; Space or Enter activates the focused tab, as in Base UI.
func (t *TabsWidget) ActivateOnFocus(v bool) *TabsWidget {
	t.activateOnFocus = v
	return t
}

// Disabled makes this page unavailable to pointer, keyboard and accessibility actions.
func (p TabPage) Disabled(v bool) TabPage { p.disabled = v; return p }

func (t *TabsWidget) enabled(i int) bool { return i >= 0 && i < len(t.tabs) && !t.tabs[i].disabled }
func (t *TabsWidget) focusIndex() int {
	if t.enabled(t.active) {
		return t.active
	}
	if t.enabled(t.index()) {
		return t.index()
	}
	return stepIndex(-1, 1, len(t.tabs), t.enabled)
}

// Disabled greys the strip out and ignores input while v is true; the
// selected page stays.
func (t *TabsWidget) Disabled(v bool) *TabsWidget { t.SetInert(v); return t }

// OnChange fires with the new index after the user picks a page.
func (t *TabsWidget) OnChange(fn func(int)) *TabsWidget { t.onChange = fn; return t }

func (t *TabsWidget) index() int {
	if len(t.tabs) == 0 {
		return -1
	}
	return int(clamp(float64(ggui.Untrack(t.selected.Get)), 0, float64(len(t.tabs)-1)))
}

func (t *TabsWidget) pick(i int) {
	if t.enabled(i) && !t.IsInert() {
		t.active = i
		t.focusPending = true
		setChanged(t.selected, i, t.onChange)
	}
}

// Layout implements Widget.
func (t *TabsWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer t.props.Layout().EndLayout()
	t.Sync()
	th := uitheme.Of(env)
	t.theme, t.rtl = th, env.Direction() == ggui.DirRTL
	t.motion, t.pad = env.Motion(th.MotionFast), th.TabPad
	t.labelSize, t.labelX = t.labelSize[:0], t.labelX[:0]
	t.headerH, t.stripW, t.stripH = 0, 0, 0
	cur := t.index()
	gap := pick(t.line, tabGap, 0.0)
	for i, head := range t.heads {
		t.boxes[i].Padding(t.pad)
		// A label of the strip's own is recolored in Paint as the pointer
		// crosses it; what a Header draws takes the resting color from here.
		fg := t.labelColor(i, pick(i == cur, 1.0, 0.0))
		if t.labels[i] != nil {
			t.labels[i].Color(fg)
		}
		s := head.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env.WithText(th.Label.Merge(ggui.TextStyle{Color: fg})))
		t.labelSize = append(t.labelSize, s)
		if t.vertical {
			t.stripH += pick(i > 0, gap, 0.0)
			t.labelX = append(t.labelX, t.stripH)
			t.stripH += s.H
			t.stripW = max(t.stripW, s.W)
		} else {
			t.stripW += pick(i > 0, gap, 0.0)
			t.labelX = append(t.labelX, t.stripW)
			t.stripW += s.W
			t.stripH = max(t.stripH, s.H)
		}
	}
	t.stripW += 2 * tabInset
	t.stripH += 2 * tabInset
	t.headerH = t.stripH + th.Space
	t.bodySize = ggui.Size{}
	if cur >= 0 && t.tabs[cur].Content != nil {
		body := ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MinH: max(c.MinH-t.headerH, 0), MaxH: max(c.MaxH-t.headerH, 0)}
		if t.vertical {
			body = ggui.Constraints{MinW: max(c.MinW-t.stripW-th.Space, 0), MaxW: max(c.MaxW-t.stripW-th.Space, 0), MinH: c.MinH, MaxH: c.MaxH}
		}
		t.bodySize = t.tabs[cur].Content.Layout(body, env)
	}
	if t.vertical {
		return c.Constrain(ggui.Sz(t.stripW+th.Space+t.bodySize.W, max(t.stripH, t.bodySize.H)))
	}
	return c.Constrain(ggui.Sz(max(t.stripW, t.bodySize.W), t.headerH+t.bodySize.H))
}

// barRect is where a line strip marks the label at lr. shadcn hangs the
// bar off the trigger's padding box, the label less its one-pixel border:
// as long as that box, and after:bottom-[-5px] h-0.5 under a trigger that
// is a pixel shorter than the strip's inside and centered in it, which
// lands half a pixel past the strip's foot; after:-right-1 w-0.5 beside a
// vertical one, which lands on the strip's edge.
func (t *TabsWidget) barRect(strip, lr ggui.Rect) ggui.Rect {
	b := t.theme.BorderWidth
	if t.vertical {
		x := pick(t.rtl, strip.Origin.X, strip.Origin.X+strip.Size.W-tabBar)
		return ggui.Rct(ggui.Pt(x, lr.Origin.Y+b), ggui.Sz(tabBar, max(0, lr.Size.H-2*b)))
	}
	return ggui.Rct(ggui.Pt(lr.Origin.X+b, strip.Origin.Y+strip.Size.H-tabBar+tabBarDrop), ggui.Sz(max(0, lr.Size.W-2*b), tabBar))
}

// labelColor is the color of label i when it is k of the way to the text
// color, which the shown page's label and one under the pointer have at
// one: from the quieter color shadcn gives a tab at rest, 60% of the text
// color on a light page and the muted one on a dark page. A tab that cannot
// be picked is at half strength.
func (t *TabsWidget) labelColor(i int, k float64) color.Color {
	th := t.theme
	fg := th.Fg
	if k < 1 {
		rest := pick(isDark(th.Bg), th.MutedFg, fade(th.Fg, .6))
		fg = rest
		if k > 0 {
			fg = mix(rest, th.Fg, k)
		}
	}
	if t.IsInert() || !t.enabled(i) {
		fg = th.Disabled(fg)
	}
	return fg
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
	strip := mirrorHorizontal(ggui.Rct(r.Origin, ggui.Sz(min(t.stripW, r.Size.W), min(t.stripH, r.Size.H))), r, t.rtl)
	header := strip
	if !t.vertical {
		header = ggui.Rct(r.Origin, ggui.Sz(r.Size.W, strip.Size.H))
	}

	t.labelRect = t.labelRect[:0]
	for i, s := range t.labelSize {
		var lr ggui.Rect
		if t.vertical {
			lr = ggui.Rct(ggui.Pt(strip.Origin.X+tabInset, strip.Origin.Y+tabInset+t.labelX[i]), ggui.Sz(max(strip.Size.W-2*tabInset, 0), s.H))
		} else {
			lr = mirrorHorizontal(ggui.Rct(ggui.Pt(r.Origin.X+tabInset+t.labelX[i], r.Origin.Y+tabInset), ggui.Sz(s.W, max(strip.Size.H-2*tabInset, 0))), r, t.rtl)
		}
		t.labelRect = append(t.labelRect, lr)
	}
	cur := t.index()
	// Each label eases into and out of being the shown one where it stands,
	// as shadcn's triggers do under transition-all: the surface of the one
	// left fades out while that of the one picked fades in, and nothing
	// slides between them.
	t.shown = t.shown[:0]
	for i := range t.labelRect {
		t.shown = append(t.shown, dst.Ease(ggui.Anchor{Rect: header, ID: i}, tabShownSlot, pick(i == cur, 1.0, 0.0), t.motion))
	}
	if !t.line {
		dst.FillRoundRect(strip, th.Radius, th.Muted)
		dark := isDark(th.Bg)
		for i, lr := range t.labelRect {
			k := t.shown[i]
			if k <= 0 {
				continue
			}
			if dark {
				dst.FillRoundRect(lr, th.RadiusSm, fade(fade(th.InputBorder, .3), k))
				dst.StrokeRoundRect(lr, th.RadiusSm, th.BorderWidth, fade(th.InputBorder, k))
				continue
			}
			shadow := th.CardShadow
			shadow.Color = fade(shadow.Color, k)
			dst.Shadow(lr, th.RadiusSm, shadow)
			dst.FillRoundRect(lr, th.RadiusSm, fade(th.Bg, k))
		}
	}
	for i, lr := range t.labelRect {
		dst.Describe(lr, tabLabel{t, i})
		if !t.IsInert() && t.enabled(i) {
			dst.HitPointer(lr, tabLabel{t, i})
			dst.HitKey(lr, tabLabel{t, i})
			dst.HitCursor(lr, ggui.CursorShapeDefault)
		}
		if label := t.labels[i]; label != nil {
			over := i == t.hover && i != cur && !t.IsInert() && t.enabled(i)
			hover := dst.Ease(ggui.Anchor{Rect: header, ID: i}, tabHoverSlot, pick(over, 1.0, 0.0), t.motion)
			label.Color(t.labelColor(i, max(hover, t.shown[i])))
		}
		dst.Paint(t.heads[i], lr)
	}
	for i, lr := range t.labelRect {
		k := t.shown[i]
		if !t.line || k <= 0 {
			continue
		}
		// The bar by the shown label is the text color whatever the accent,
		// and nothing is ruled along the rest of the strip. It fades in
		// and out with its label, shadcn's after:transition-opacity.
		bar := fade(pick(t.IsInert() || !t.enabled(i), th.Disabled(th.Fg), th.Fg), k)
		at := t.barRect(strip, lr)
		if s := dst.Scale(); s > 0 && !t.vertical {
			// On whole device pixels, as a browser snaps a box it paints:
			// a pixel past the strip at 1x rather than a blurred edge.
			at.Origin.Y = math.Round(at.Origin.Y*s) / s
		}
		dst.FillRect(at, bar)
	}
	if t.focusPending && t.focusIndex() >= 0 {
		dst.RequestFocus(tabLabel{t, t.focusIndex()})
		t.focusPending = false
	}
	if focus := t.focusIndex(); focus >= 0 {
		controlFocusRing(dst, &t.Interactive, th, t.labelRect[focus], th.RadiusSm, th.Ring)
	}
	if cur >= 0 && t.tabs[cur].Content != nil {
		at := ggui.Pt(r.Origin.X, r.Origin.Y+t.headerH)
		if t.vertical {
			at = ggui.Pt(r.Origin.X+t.stripW+th.Space, r.Origin.Y)
			if t.rtl {
				at.X = r.Origin.X
			}
		}
		dst.Paint(t.tabs[cur].Content, ggui.Rct(at, t.bodySize))
	}
}

// HandleKey implements KeyHandler: Left and Right move between pages.
func (t *TabsWidget) HandleKey(ev ggui.KeyEvent) {
	t.Keyboard(ev, nil)

	if t.IsInert() || ev.Kind != ggui.KeyPress || len(t.tabs) == 0 {
		return
	}
	if ggui.Activates(ev) {
		t.pick(t.focusIndex())
		return
	}
	axis := pick(t.vertical, navVertical, navBoth)
	if i, ok := navIndex(directionalKey(ev, t.rtl && !t.vertical), axis, t.focusIndex(), len(t.tabs), t.enabled); ok {
		t.active = i
		t.focusPending = true
		if t.activateOnFocus {
			t.pick(i)
		}
	}
}

// ConsumesKey keeps navigation and activation keys within the tab list.
func (t *TabsWidget) ConsumesKey(ev ggui.KeyEvent) bool {
	return navConsumes(ev, pick(t.vertical, navVertical, navBoth)) || ggui.Activates(ev)
}

// Adopt implements ggui.Adopter.
func (t *TabsWidget) Adopt(prev any) {
	t.Interactive.Adopt(prev)
	if p, ok := prev.(*TabsWidget); ok {
		t.hover = p.hover
		t.active = p.active
	}
}

var (
	tabShownSlot = ggui.NewSlot[*ggui.Motion]("tab shown")
	tabHoverSlot = ggui.NewSlot[*ggui.Motion]("tab hover")
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
		Disabled: l.t.IsInert() || !l.t.enabled(l.i),
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

// Act implements ggui.Actor: showing this tab's page.
func (l tabLabel) Act(a ggui.Action) bool {
	if l.t.IsInert() || !l.t.enabled(l.i) || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	l.t.pick(l.i)
	return true
}

func (l tabLabel) HandlePointer(ev ggui.PointerEvent) bool {
	if l.t.IsInert() || !l.t.enabled(l.i) {
		return false
	}
	return hoverPick(ev, l.i, &l.t.hover, func() { l.t.pick(l.i) })
}

func (l tabLabel) Adopt(prev any) {
	if p, ok := prev.(tabLabel); ok {
		l.t.Interactive.Adopt(p.t)
		l.t.active = p.t.active
		if p.t.hover == p.i {
			l.t.hover = l.i
		}
	}
}

// BindDisabled follows r for Disabled without rebuilding the control.
func (t *TabsWidget) BindDisabled(r ggui.Readable[bool]) *TabsWidget { t.BindInert(r); return t }

// Each tab is an accessibility focus target; only the roving tab is a Tab stop.
func (l tabLabel) TabStop() bool { return l.t.enabled(l.i) && l.i == l.t.focusIndex() }
func (l tabLabel) HitID() any {
	if owner := l.t.HitID(); owner != nil {
		return childKey{owner: owner, part: l.i}
	}
	return nil
}
func (l tabLabel) ConsumesKey(ev ggui.KeyEvent) bool { return l.t.ConsumesKey(ev) }
func (l tabLabel) HandleKey(ev ggui.KeyEvent) {
	if ev.Kind == ggui.KeyFocus {
		l.t.active = l.i
	}
	l.t.HandleKey(ev)
}

// Describe keeps the named list available as a focus shortcut; focus itself
// belongs to its roving child so assistive technology announces the active tab.
func (t *TabsWidget) Describe() ggui.Node {
	return ggui.Node{Role: ggui.RoleTabs, Name: t.SemanticName(), Disabled: t.IsInert(), Actions: ggui.ActionFocus}
}

// Act forwards a request to focus the list to its roving tab.
func (t *TabsWidget) Act(a ggui.Action) bool {
	if a.Kind != ggui.ActionFocus || t.IsInert() || t.focusIndex() < 0 {
		return false
	}
	t.focusPending = true
	return true
}
