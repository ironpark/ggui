package ui

import (
	"image/color"
	"math"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// An editor's tabs, as a code editor or a terminal lays them along the top
// of a pane: one per open file or session, the pane's own tabs rather than
// pages of one control, which Tabs is. EditorTab is one tab, a rounded box;
// TabGroup holds the tabs of one project, or whatever groups them, under a
// label and a line of its color, as a browser's tab groups are. The bar
// itself, how the tabs scroll, reorder and split the panes, is the app's.
//
//	bar := widget.Row(
//		ui.EditorTab("main.go", show).Icon(goIcon).Selected(true).OnClose(close),
//		ui.EditorTab("go.mod", show).Dirty(true).OnClose(close),
//		ui.EditorTab("README.md", show).Preview(true).OnClose(close),
//	)

// The size of an editor tab's box, and the narrowest it is drawn unless
// MinWidth says otherwise.
const (
	EditorTabHeight   = 28
	EditorTabRadius   = 7
	EditorTabMinWidth = 92
)

const (
	tabStatusSize = 7  // the room of a tab's status mark, before its icon
	tabLabelFade  = 18 // how far a label cut short fades out at its end
)

// EditorTabWidget is one tab of an editor's tab bar. Build one with
// EditorTab.
type EditorTabWidget struct {
	label, name, closeName string
	onSelect, onClose      func()
	icon, status           ggui.Widget
	statused               bool // the tab has a status's room, set or not
	maxLabel, minWidth     float64
	selected, current      bool
	dirty, preview         bool
	tree                   ggui.Widget // built at the first layout, from the above
}

// EditorTab is a tab named label, which shows what it stands for with
// onSelect. It is a rounded box: raised while it is the tab its pane shows
// (Selected), the more brightly, and lifted, in the pane that has the
// focus (Current), and washed while the pointer is over it. Its close
// button (OnClose) shows while the pointer is over the tab or the focus is
// in it, and is there all the while, in the Tab order and to screen
// readers. A label longer than MaxLabelWidth fades out at its end, and
// shows whole in a tooltip.
func EditorTab(label string, onSelect func()) *EditorTabWidget {
	return &EditorTabWidget{label: label, onSelect: onSelect, maxLabel: 170, minWidth: EditorTabMinWidth}
}

// Icon puts w before the label, as a file's type or a session's program.
func (t *EditorTabWidget) Icon(w ggui.Widget) *EditorTabWidget { t.icon = w; return t }

// Status puts w before the icon, in a room of its own the size of a dot:
// what the tab's content is doing, as an agent at work or waiting on the
// user. The room stays with w nil, so a tab keeps its width, and its icon
// its place, as its status comes and goes.
func (t *EditorTabWidget) Status(w ggui.Widget) *EditorTabWidget {
	t.status, t.statused = w, true
	return t
}

// Selected raises the tab as the one its pane shows, and sets its label in
// the foreground color rather than the muted one.
func (t *EditorTabWidget) Selected(v bool) *EditorTabWidget { t.selected = v; return t }

// Current raises the selected tab more brightly, and lifts it, as the tab
// of the pane that has the focus.
func (t *EditorTabWidget) Current(v bool) *EditorTabWidget { t.current = v; return t }

// OnClose gives the tab a close button that calls fn.
func (t *EditorTabWidget) OnClose(fn func()) *EditorTabWidget { t.onClose = fn; return t }

// Dirty marks unsaved changes with a dot in the close button's place,
// which the button takes while it shows, as editors mark them.
func (t *EditorTabWidget) Dirty(v bool) *EditorTabWidget { t.dirty = v; return t }

// Preview sets the label in italics: a tab the next file opened takes the
// place of unless it is kept, as editors preview a file.
func (t *EditorTabWidget) Preview(v bool) *EditorTabWidget { t.preview = v; return t }

// MaxLabelWidth caps how wide the label is drawn before it fades out; 170
// unless set.
func (t *EditorTabWidget) MaxLabelWidth(w float64) *EditorTabWidget { t.maxLabel = w; return t }

// MinWidth is the narrowest the tab is drawn, its label taking the room
// left over; EditorTabMinWidth unless set.
func (t *EditorTabWidget) MinWidth(w float64) *EditorTabWidget { t.minWidth = w; return t }

// Name names the tab to screen readers and Probe; the label unless set.
func (t *EditorTabWidget) Name(s string) *EditorTabWidget { t.name = s; return t }

// CloseName names the close button; "Close " and the label unless set, as
// an app in another language sets it.
func (t *EditorTabWidget) CloseName(s string) *EditorTabWidget { t.closeName = s; return t }

func (t *EditorTabWidget) build() ggui.Widget {
	hover, focus := ggui.State(false), ggui.State(false)
	tone := tabRest
	if t.selected {
		tone = pick(t.current, tabCurrent, tabShown)
	}
	label := &editorTabLabel{text: widget.Text(t.label).Size(13).NoWrap(), max: t.maxLabel, tone: tone}
	if t.preview {
		label.text.Italic()
	}
	label.tip = Tooltip(label, t.label)
	var parts []ggui.Widget
	left := 10.0
	lead := t.icon
	if t.statused {
		mark := t.status
		if mark == nil {
			mark = widget.Box()
		}
		slot := widget.Box(widget.Center(mark)).Size(tabStatusSize, tabStatusSize)
		lead = slot
		if t.icon != nil {
			lead = widget.Row(slot, t.icon).Gap(4).Align(widget.AlignCenter)
		}
		left = 6
	}
	if lead != nil {
		parts = append(parts, lead)
	}
	parts = append(parts, label.tip)
	right := 10.0
	if t.onClose != nil {
		parts = append(parts, t.closeButton(hover, focus))
		right = 4
	}
	content := widget.Box(widget.Row(parts...).Gap(8).Align(widget.AlignCenter)).Height(EditorTabHeight)
	name := t.name
	if name == "" {
		name = t.label
	}
	// The tab draws its hover itself, over all of it, close button too: a
	// button's own would come and go as the pointer crossed to the close
	// button, over the tab's.
	btn := ButtonOf(content, t.onSelect).Variant(editorTabButton).Pad(0, right, 0, left).Radius(EditorTabRadius).Name(name)
	box := ggui.View(hover, func(h bool) ggui.Widget {
		return &editorTabBox{child: btn, button: btn, label: label, minWidth: t.minWidth, hovered: h, selected: t.selected, current: t.current}
	})
	return widget.Box(widget.Within(box).OnHover(hover.Set).OnFocus(focus.Set)).Pad(0, 2)
}

// closeButton is the tab's close button, shown while the pointer is over
// the tab or the focus is in it, and the dot of a dirty tab while not.
func (t *EditorTabWidget) closeButton(hover, focus *ggui.StateValue[bool]) ggui.Widget {
	shown := ggui.Combine(hover, focus, func(h, f bool) float64 {
		if h || f {
			return 1
		}
		return 0
	})
	name := t.closeName
	if name == "" {
		name = "Close " + t.label
	}
	btn := widget.Opacity(ButtonOf(Icon(icons.Close).Size(13), t.onClose).Ghost().Pad(3).Radius(4).Name(name), 0).
		BindOpacity(shown)
	if !t.dirty {
		return btn
	}
	// As large as the button, its icon and its padding.
	dot := widget.Box(widget.Center(widget.Box().Size(8, 8).Radius(4).Fill(theme.MutedFg))).Size(19, 19)
	hidden := ggui.Map(shown, func(v float64) float64 { return 1 - v })
	return widget.Stack(widget.Opacity(dot, 1).BindOpacity(hidden), btn)
}

// Layout implements Widget.
func (t *EditorTabWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if t.tree == nil {
		t.tree = t.build()
	}
	return t.tree.Layout(c, env)
}

// Paint implements Widget.
func (t *EditorTabWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(t.tree, r) }

// editorTabButton is the look of a tab's button, and of a group's label:
// its content alone, the tab or the group drawing the rest.
var editorTabButton = NewVariant("editor tab", func(t theme.Theme) ButtonStyle {
	return ButtonStyle{Label: t.Fg, HoverLabel: t.Fg}
})

// editorTabHoverSlot keeps how far a tab is into its hover, which eases in
// and out as a button's does; tabGroupHoverSlot a group's label's.
var (
	editorTabHoverSlot = ggui.NewSlot[*ggui.Motion]("editor tab hover")
	tabGroupHoverSlot  = ggui.NewSlot[*ggui.Motion]("tab group hover")
)

// editorTabBox paints a tab's box under its button: the raised fill of the
// selected tab, lifted in the pane with the focus, and the wash of one the
// pointer is over. It holds the tab to its MinWidth, the label taking the
// room.
type editorTabBox struct {
	child                      ggui.Widget
	button                     *ButtonWidget // what the hover's easing is kept by
	label                      *editorTabLabel
	minWidth                   float64
	hovered, selected, current bool
	fill, wash                 color.Color // under the theme the tab was laid out in
	theme                      *theme.Theme
	motion                     time.Duration
}

// The lift of the selected tab of the pane with the focus: a line of light
// along its top edge, and a shadow under it.
var (
	tabLiftLight  = color.NRGBA{0xff, 0xff, 0xff, 0x12}
	tabLiftShadow = ggui.ShadowStyle{Offset: ggui.Pt(0, 1), Blur: 3, Color: color.NRGBA{A: 0x73}}
)

func (b *editorTabBox) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := theme.Of(env)
	b.theme, b.motion = t, env.Motion(t.MotionFast)
	b.wash, b.fill = withAlpha(t.Fg, 0x0d), nil
	if b.selected {
		b.fill = t.Muted
		if b.current {
			b.fill = mix(t.Muted, t.Fg, .07)
		}
	}
	b.label.extra = 0
	size := b.child.Layout(c, env)
	if size.W < b.minWidth && c.MaxW >= b.minWidth {
		b.label.extra = b.minWidth - size.W
		size = b.child.Layout(c, env)
	}
	return size
}

func (b *editorTabBox) Paint(dst *ggui.Canvas, r ggui.Rect) {
	hover := ease(dst, b.theme, b.button.Anchor(r), editorTabHoverSlot, pick(b.hovered, 1.0, 0.0), b.motion)
	if b.fill != nil {
		if b.current {
			dst.Shadow(r, EditorTabRadius, tabLiftShadow)
			// The light shows only along the top: the fill covers the rest
			// of it, a pixel lower.
			dst.FillRoundRect(r, EditorTabRadius, b.fill)
			dst.FillRoundRect(r, EditorTabRadius, tabLiftLight)
			dst.FillRoundRect(ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+1), ggui.Sz(r.Size.W, r.Size.H-1)), EditorTabRadius, b.fill)
		} else {
			dst.FillRoundRect(r, EditorTabRadius, b.fill)
		}
	}
	if hover > 0 {
		dst.FillRoundRect(r, EditorTabRadius, withAlpha(b.wash, uint8(math.Round(hover*pick(b.fill != nil, 128.0, 255.0)))))
	}
	dst.Paint(b.child, r)
}

// editorTabTone is how a tab's label is set: muted at rest, in the foreground
// color in the pane with the focus, and between the two in another pane.
type editorTabTone int

const (
	tabRest editorTabTone = iota
	tabShown
	tabCurrent
)

// editorTabLabel is a tab's label: as wide as its text, up to max, and then cut
// and faded out at its end, the tooltip around it showing it whole; and
// extra wider, to fill a tab held to its MinWidth.
type editorTabLabel struct {
	text       *widget.TextWidget
	tip        *TooltipWidget
	max, extra float64
	tone       editorTabTone
	natural    ggui.Size
	cut        bool
}

func (l *editorTabLabel) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := theme.Of(env)
	switch l.tone {
	case tabCurrent:
		l.text.Color(t.Fg)
	case tabShown:
		l.text.Color(mix(t.Fg, t.MutedFg, .35))
	default:
		l.text.Color(t.MutedFg)
	}
	l.natural = l.text.Layout(ggui.Constraints{MaxW: ggui.Unbounded, MaxH: c.MaxH}, env)
	l.cut = l.natural.W > l.max
	l.tip.disabled = !l.cut // only a label cut short has more to show
	return c.Constrain(ggui.Sz(min(l.natural.W, l.max)+l.extra, l.natural.H))
}

func (l *editorTabLabel) Paint(dst *ggui.Canvas, r ggui.Rect) {
	at := ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-l.natural.H)/2), l.natural)
	if !l.cut {
		dst.Paint(l.text, at)
		return
	}
	dst.Clip(r).FadeEdges(r, true, 0, tabLabelFade, func(dst *ggui.Canvas) { dst.Paint(l.text, at) })
}

// Baseline implements Baseliner: the text's.
func (l *editorTabLabel) Baseline() (float64, bool) { return ggui.BaselineOf(l.text) }

// TabGroupWidget is a run of editor tabs under a label and a line of its
// color. Build one with TabGroup.
type TabGroupWidget struct {
	cells     []ggui.Widget
	label     ggui.Widget
	onLabel   func()
	labelName string
	tint      color.Color

	row, chip ggui.Widget // built at the first layout
	button    *ButtonWidget
	rowSize   ggui.Size
	chipSize  ggui.Size
	fill      color.Color // the label's, at rest and under the pointer
	hoverFill color.Color
	line      color.Color
	theme     *theme.Theme
	motion    time.Duration
}

const (
	tabGroupMargin = 3  // the room on each side of a group
	tabGroupGap    = 2  // between its label and its first tab
	tabGroupPadY   = 5  // above and below its tabs; the line is in the lower
	tabGroupLine   = 2  // the line's thickness
	tabGroupInset  = 6  // how far in from the group's ends the line stops
	tabGroupStick  = 4  // how far in from a scrolled bar's start a label sticks
	tabGroupFadeIn = 22 // how far tabs passing behind a stuck label fade in
)

// TabGroup holds cells, editor tabs and what goes between them, side by
// side over a line of its Tint, as a browser shows a tab group, headed by
// its Label. In a Scroll that has carried the group's start out of view,
// the label stays at the start of what shows, as far as the group goes,
// the tabs passing behind it hidden and fading in after it.
func TabGroup(cells ...ggui.Widget) *TabGroupWidget { return &TabGroupWidget{cells: cells} }

// Label heads the group with w, as its name, on a chip of its color as
// tall as a tab.
func (g *TabGroupWidget) Label(w ggui.Widget) *TabGroupWidget { g.label = w; return g }

// OnLabel makes the label a button that calls fn, as one folding the group
// away does.
func (g *TabGroupWidget) OnLabel(fn func()) *TabGroupWidget { g.onLabel = fn; return g }

// LabelName names the label's button to screen readers and Probe.
func (g *TabGroupWidget) LabelName(s string) *TabGroupWidget { g.labelName = s; return g }

// Tint is the group's color; the theme's ring color unless set.
func (g *TabGroupWidget) Tint(c color.Color) *TabGroupWidget { g.tint = c; return g }

// Layout implements Widget.
func (g *TabGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if g.row == nil {
		g.row = widget.Row(g.cells...).Align(widget.AlignCenter)
		if g.label != nil {
			content := widget.Box(widget.Row(g.label).Align(widget.AlignCenter)).Height(EditorTabHeight)
			if g.onLabel != nil {
				g.button = ButtonOf(content, g.onLabel).Variant(editorTabButton).Pad(0, 10).Radius(EditorTabRadius)
				if g.labelName != "" {
					g.button.Name(g.labelName)
				}
				g.chip = g.button
			} else {
				g.chip = widget.Box(content).Pad(0, 10)
			}
		}
	}
	t := theme.Of(env)
	g.theme, g.motion = t, env.Motion(t.MotionFast)
	tint := g.tint
	if tint == nil {
		tint = theme.Ring
	}
	tint = ggui.ResolveColor(tint, env)
	g.fill, g.hoverFill, g.line = withAlpha(tint, 0x29), withAlpha(tint, 0x42), withAlpha(tint, 0xbf)
	loose := ggui.Constraints{MaxW: ggui.Unbounded, MaxH: c.MaxH}
	g.rowSize = g.row.Layout(loose, env)
	w, h := 2*tabGroupMargin+g.rowSize.W, max(EditorTabHeight, g.rowSize.H)
	g.chipSize = ggui.Size{}
	if g.chip != nil {
		g.chipSize = g.chip.Layout(loose, env)
		w += g.chipSize.W + tabGroupGap
		h = max(h, g.chipSize.H)
	}
	return c.Constrain(ggui.Sz(w, h+2*tabGroupPadY))
}

// Paint implements Widget.
func (g *TabGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	x0, mid := r.Origin.X+tabGroupMargin, r.Origin.Y+r.Size.H/2
	line := ggui.Rct(ggui.Pt(x0+tabGroupInset, r.Origin.Y+r.Size.H-tabGroupLine),
		ggui.Sz(max(r.Size.W-2*(tabGroupMargin+tabGroupInset), 0), tabGroupLine))
	dst.FillRoundRect(line, tabGroupLine/2, g.line)
	rowX := x0
	if g.chip != nil {
		rowX += g.chipSize.W + tabGroupGap
	}
	rowAt := ggui.Rct(ggui.Pt(rowX, mid-g.rowSize.H/2), g.rowSize)
	if g.chip == nil {
		dst.Paint(g.row, rowAt)
		return
	}
	// Scrolled past the group's start, the label sticks at the start of
	// what shows, as far as the group's end lets it.
	chipX := x0
	if clip, ok := dst.ClipBounds(); ok {
		chipX = max(x0, min(clip.Origin.X+tabGroupStick, r.Origin.X+r.Size.W-tabGroupMargin-g.chipSize.W))
	}
	if chipX > x0+.5 {
		// The tabs behind it are hidden, and fade in after it.
		from := chipX + g.chipSize.W + tabGroupGap
		if after := ggui.Rct(ggui.Pt(from, r.Origin.Y), ggui.Sz(max(r.Origin.X+r.Size.W-from, 0), r.Size.H)); after.Size.W > 0 {
			dst.Clip(after).FadeEdges(after, true, tabGroupFadeIn, 0, func(dst *ggui.Canvas) { dst.Paint(g.row, rowAt) })
		}
	} else {
		dst.Paint(g.row, rowAt)
	}
	chipAt := ggui.Rct(ggui.Pt(chipX, mid-g.chipSize.H/2), g.chipSize)
	fill := g.fill
	if g.button != nil {
		if h := ease(dst, g.theme, g.button.Anchor(chipAt), tabGroupHoverSlot, pick(g.button.Hovered, 1.0, 0.0), g.motion); h > 0 {
			fill = mix(g.fill, g.hoverFill, h)
		}
	}
	dst.FillRoundRect(chipAt, EditorTabRadius, fill)
	dst.Paint(g.chip, chipAt)
}

// withAlpha is c at alpha a out of 0xff of its own; nothing for no color.
func withAlpha(c color.Color, a uint8) color.NRGBA {
	if c == nil {
		return color.NRGBA{}
	}
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	n.A = uint8(uint16(n.A) * uint16(a) / 0xff)
	return n
}
