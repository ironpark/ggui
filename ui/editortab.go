package ui

import (
	"image/color"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// An editor's tabs, as a code editor or a terminal lays them along the top
// of a pane: one per open file or session, the pane's own tabs rather than
// pages of one control, which Tabs is. EditorTab is one tab, a rounded box;
// TabGroup holds the tabs of one project, or whatever groups them, in a
// capsule of its color, as a browser's tab groups are. The bar itself, how
// the tabs scroll, reorder and split the panes, is the app's.
//
//	bar := widget.Row(
//		ui.EditorTab("main.go", show).Icon(goIcon).Selected(true).OnClose(close),
//		ui.EditorTab("go.mod", show).Dirty(true).OnClose(close),
//		ui.EditorTab("README.md", show).Preview(true).OnClose(close),
//	)

// The size of an editor tab's box.
const (
	EditorTabHeight = 28
	EditorTabRadius = 7
)

// EditorTabWidget is one tab of an editor's tab bar. Build one with
// EditorTab.
type EditorTabWidget struct {
	label     string
	onSelect  func()
	onClose   func()
	icon      ggui.Widget
	name      string
	closeName string
	tint      color.Color
	maxLabel  float64
	selected  bool
	current   bool
	dirty     bool
	preview   bool
	tree      ggui.Widget // built at the first layout, from the above
}

// EditorTab is a tab named label, which shows what it stands for with
// onSelect. It is a rounded box: raised and ringed while it is the tab its
// pane shows (Selected), the ring stronger in the pane that has the focus
// (Current), and tinted while the pointer is over it. Its close button
// (OnClose) shows while the pointer is over the tab or the focus is in it,
// and is there all the while, in the Tab order and to screen readers.
func EditorTab(label string, onSelect func()) *EditorTabWidget {
	return &EditorTabWidget{label: label, onSelect: onSelect, maxLabel: 170}
}

// Icon puts w before the label, as a file's type or a session's program.
func (t *EditorTabWidget) Icon(w ggui.Widget) *EditorTabWidget { t.icon = w; return t }

// Selected raises the tab as the one its pane shows, and sets its label in
// the foreground color rather than the muted one.
func (t *EditorTabWidget) Selected(v bool) *EditorTabWidget { t.selected = v; return t }

// Current rings the selected tab more strongly, as the tab of the pane
// that has the focus.
func (t *EditorTabWidget) Current(v bool) *EditorTabWidget { t.current = v; return t }

// OnClose gives the tab a close button that calls fn.
func (t *EditorTabWidget) OnClose(fn func()) *EditorTabWidget { t.onClose = fn; return t }

// Dirty marks unsaved changes with a dot in the close button's place,
// which the button takes while it shows, as editors mark them.
func (t *EditorTabWidget) Dirty(v bool) *EditorTabWidget { t.dirty = v; return t }

// Preview sets the label in italics: a tab the next file opened takes the
// place of unless it is kept, as editors preview a file.
func (t *EditorTabWidget) Preview(v bool) *EditorTabWidget { t.preview = v; return t }

// Tint is the color the selected tab is ringed in and one the pointer is
// over tinted with; unset, the TabGroup's the tab is in, else the theme's
// ring color.
func (t *EditorTabWidget) Tint(c color.Color) *EditorTabWidget { t.tint = c; return t }

// MaxLabelWidth caps how wide the label is drawn before it ends in an
// ellipsis; 170 unless set.
func (t *EditorTabWidget) MaxLabelWidth(w float64) *EditorTabWidget { t.maxLabel = w; return t }

// Name names the tab to screen readers and Probe; the label unless set.
func (t *EditorTabWidget) Name(s string) *EditorTabWidget { t.name = s; return t }

// CloseName names the close button; "Close " and the label unless set, as
// an app in another language sets it.
func (t *EditorTabWidget) CloseName(s string) *EditorTabWidget { t.closeName = s; return t }

func (t *EditorTabWidget) build() ggui.Widget {
	hover, focus := ggui.State(false), ggui.State(false)
	var fg color.Color = theme.MutedFg
	if t.selected {
		fg = theme.Fg
	}
	text := widget.Text(t.label).Size(13).Color(fg).NoWrap().Ellipsis()
	if t.preview {
		text = text.Italic()
	}
	var parts []ggui.Widget
	if t.icon != nil {
		parts = append(parts, t.icon)
	}
	parts = append(parts, widget.Box(text).MaxWidth(t.maxLabel))
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
	btn := ButtonOf(content, t.onSelect).Ghost().Pad(0, right, 0, 10).Radius(EditorTabRadius).Name(name)
	box := ggui.View(hover, func(h bool) ggui.Widget {
		return &editorTabBox{child: btn, tint: t.tint, hovered: h, selected: t.selected, current: t.current}
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

// tabTintKey is the tint of the TabGroup the tabs under it are in.
var tabTintKey = ggui.NewEnvKey[color.Color]("editor tab tint")

// editorTabBox paints a tab's box under its button: the raised fill of the
// selected tab and its ring, or the tint of one the pointer is over.
type editorTabBox struct {
	child                      ggui.Widget
	tint                       color.Color
	hovered, selected, current bool
	fill, ring, wash           color.Color // under the theme the tab was laid out in
}

func (b *editorTabBox) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	tint := b.tint
	if tint == nil {
		tint, _ = env.Get(tabTintKey)
	}
	if tint == nil {
		tint = theme.Ring
	}
	tint = ggui.ResolveColor(tint, env)
	b.wash, b.fill, b.ring = withAlpha(tint, 0x44), nil, nil
	if b.selected {
		b.fill = ggui.ResolveColor(theme.Muted, env)
		ring := uint8(0x38)
		if b.current {
			ring = 0x99
		}
		b.ring = withAlpha(tint, ring)
	}
	return b.child.Layout(c, env)
}

func (b *editorTabBox) Paint(dst *ggui.Canvas, r ggui.Rect) {
	switch {
	case b.fill != nil:
		dst.FillRoundRect(r, EditorTabRadius, b.fill)
		if b.hovered {
			dst.FillRoundRect(r, EditorTabRadius, withAlpha(b.wash, 0x80))
		}
		dst.StrokeRoundRect(r.Grow(-0.5), EditorTabRadius-0.5, 1, b.ring)
	case b.hovered:
		dst.FillRoundRect(r, EditorTabRadius, b.wash)
	}
	dst.Paint(b.child, r)
}

// TabGroupWidget is a run of editor tabs in a capsule. Build one with
// TabGroup.
type TabGroupWidget struct {
	cells []ggui.Widget
	tint  color.Color
	tree  ggui.Widget
}

// The room round a TabGroup's tabs inside its capsule, and on each side of
// it.
const (
	tabGroupPad    = 3
	tabGroupMargin = 3
)

// TabGroup holds cells, editor tabs and whatever heads them, as a label
// naming the group, side by side in a capsule tinted and ringed in its
// Tint, as a browser shows a tab group. The tabs in it take the tint too.
func TabGroup(cells ...ggui.Widget) *TabGroupWidget { return &TabGroupWidget{cells: cells} }

// Tint is the group's color; the theme's ring color unless set.
func (g *TabGroupWidget) Tint(c color.Color) *TabGroupWidget { g.tint = c; return g }

// Layout implements Widget.
func (g *TabGroupWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	if g.tree == nil {
		tint := g.tint
		if tint == nil {
			tint = theme.Ring
		}
		row := widget.Box(widget.Row(g.cells...).Align(widget.AlignCenter)).Pad(tabGroupPad)
		capsule := &tabCapsule{child: ggui.Provide(tabTintKey, tint, row), tint: tint}
		g.tree = widget.Box(capsule).Pad(0, tabGroupMargin)
	}
	return g.tree.Layout(c, env)
}

// Paint implements Widget.
func (g *TabGroupWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(g.tree, r) }

// tabCapsule paints a TabGroup's capsule under its tabs.
type tabCapsule struct {
	child      ggui.Widget
	tint       color.Color
	fill, ring color.Color // under the theme the group was laid out in
}

func (c *tabCapsule) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	tint := ggui.ResolveColor(c.tint, env)
	c.fill, c.ring = withAlpha(tint, 0x17), withAlpha(tint, 0x52)
	return c.child.Layout(cs, env)
}

func (c *tabCapsule) Paint(dst *ggui.Canvas, r ggui.Rect) {
	const radius = EditorTabRadius + tabGroupPad
	dst.FillRoundRect(r, radius, c.fill)
	dst.StrokeRoundRect(r.Grow(-0.5), radius-0.5, 1, c.ring)
	dst.Paint(c.child, r)
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
