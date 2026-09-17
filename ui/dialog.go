package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// DialogWidget is a modal panel centered over the window: a scrim dims
// everything else and takes the clicks, Tab stays inside, Escape or a
// click on the scrim closes it, and focus returns to where it was. Build
// one with Dialog. It takes no space where it sits in the tree; put it
// anywhere.
type DialogWidget struct {
	compact bool
	open    ggui.Binding[bool]
	content ggui.Widget
	title   *ggui.TextWidget
	name    string
	panel   *ggui.BoxWidget
	width   float64
	onClose func()
	theme   ggui.Theme
	env     ggui.Env
	rect    ggui.Rect
}

// Dialog creates a dialog that shows content while open is true.
//
//	confirm := ggui.State(false)
//	ui.Dialog(confirm, ggui.Column(
//		ggui.Text("Delete the file?"),
//		ggui.Row(ui.Button("Delete", del), ui.Button("Cancel", func() { confirm.Set(false) }).Secondary()),
//	)).Title("Confirm")
func Dialog(open ggui.Binding[bool], content ggui.Widget) *DialogWidget {
	d := &DialogWidget{open: open, content: content, width: 360}
	return d
}

// Title puts a heading above the content, which also names the dialog for
// Probe.Find.
func (d *DialogWidget) Title(s string) *DialogWidget {
	d.title, d.name = ggui.Title(s).Size(18), s
	return d
}

// Named sets an accessible name without adding a visible heading.
func (d *DialogWidget) Named(name string) *DialogWidget { d.name = name; return d }

// Compact removes the outer padding for content with its own spacing.
func (d *DialogWidget) Compact() *DialogWidget { d.compact = true; return d }

// Width sets the panel's width; it shrinks to fit a narrower window.
func (d *DialogWidget) Width(w float64) *DialogWidget { d.width = w; return d }

// OnClose fires when the dialog closes, however it closed.
func (d *DialogWidget) OnClose(fn func()) *DialogWidget { d.onClose = fn; return d }

// Close closes the dialog.
func (d *DialogWidget) Close() {
	if !d.open.Peek() {
		return
	}
	d.open.Set(false)
	if d.onClose != nil {
		d.onClose()
	}
}

// Rect returns where the panel was last painted.
func (d *DialogWidget) Rect() ggui.Rect { return d.rect }

// Semantics implements ggui.Semantic.
func (d *DialogWidget) Semantics() (ggui.Role, string) {
	return ggui.RoleDialog, d.name
}

// Layout implements Widget: the dialog takes no room in the tree.
func (d *DialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	d.theme, d.env = t, env
	body := d.content
	if d.title != nil {
		body = ggui.Column(d.title, d.content).Gap(t.Space * 2).Align(ggui.AlignStretch)
	}
	d.panel = ggui.Box(body).Padding(t.CardPad).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius + 4).Shadow(overlayShadow(t))
	if d.compact {
		d.panel.Pad(0)
	}
	return c.Constrain(ggui.Size{})
}

// Paint implements Widget: an open dialog paints through Canvas.Overlay.
func (d *DialogWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	if !d.open.Peek() {
		return
	}
	dst.Overlay(d.paintPanel)
}

func (d *DialogWidget) paintPanel(dst *ggui.Canvas) {
	t := d.theme
	screen := dst.Size()
	if screen == (ggui.Size{}) {
		screen = ggui.Sz(d.width+t.Space*4, ggui.Unbounded)
	}
	margin := t.Space * 2
	w := min(d.width, max(screen.W-2*margin, 0))
	maxH := screen.H - 2*margin
	if screen.H == ggui.Unbounded {
		maxH = ggui.Unbounded
	}
	size := d.panel.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: max(maxH, 0)}, d.env)
	at := ggui.Pt((screen.W-size.W)/2, margin)
	if screen.H != ggui.Unbounded {
		at.Y = (screen.H - size.H) / 2
	}
	d.rect = ggui.Rct(at, size)

	dst.FillRect(ggui.Rect{Size: screen}, color.RGBA{0, 0, 0, 0x60})
	dst.HitPointer(ggui.Rect{Size: screen}, dialogScrim{d})
	// The panel is the node; the scrim is a way of taking clicks, not an
	// element, so it describes nothing.
	dst.DescribeNode(d.rect, d, func(dst *ggui.Canvas) {
		dst.FocusTrap(d, d.Close, func(dst *ggui.Canvas) {
			dst.HitPointer(d.rect, dialogSink{d})
			dst.Paint(d.panel, d.rect)
		})
	})
}

// dialogScrim covers the window under an open dialog: a press closes it
// and nothing reaches what is beneath.
type dialogScrim struct{ d *DialogWidget }

func (s dialogScrim) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ebiten.MouseButtonLeft {
		s.d.Close()
	}
	return true
}

// dialogSink keeps presses inside the panel from reaching the scrim and
// names the panel for Probe.Find.
type dialogSink struct{ d *DialogWidget }

func (s dialogSink) HandlePointer(ev ggui.PointerEvent) bool { return ev.Kind != ggui.PointerScroll }

func (s dialogSink) Semantics() (ggui.Role, string) { return s.d.Semantics() }
