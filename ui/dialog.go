package ui

import "github.com/ironpark/ggui"

// DialogWidget is a modal panel centered over the window: a scrim dims
// everything else and takes the clicks, Tab stays inside, Escape or a
// click on the scrim closes it, and focus returns to where it was. Build
// one with Dialog. It takes no space where it sits in the tree; put it
// anywhere.
type DialogWidget struct {
	modal
	width float64
}

// Dialog creates a dialog that shows content while open is true.
//
//	confirm := ggui.State(false)
//	ui.Dialog(confirm, ggui.Column(
//		ggui.Text("Delete the file?"),
//		ggui.Row(ui.Button("Delete", del), ui.Button("Cancel", func() { confirm.Set(false) }).Outline()),
//	)).Title("Confirm")
func Dialog(open ggui.Binding[bool], content ggui.Widget) *DialogWidget {
	d := &DialogWidget{width: 360}
	d.open, d.content, d.dismissible = open, content, true
	return d
}

// Title puts a heading above the content, which also names the dialog for
// Probe.Find.
func (d *DialogWidget) Title(s string) *DialogWidget { d.setTitle(s); return d }

// Named sets an accessible name without adding a visible heading.
func (d *DialogWidget) Named(name string) *DialogWidget { d.name = name; return d }

// Dismissible says whether a click on the scrim closes the dialog; it does
// by default. A confirmation that must be answered sets it false and closes
// itself from its own buttons, as AlertDialog does. Escape closes either
// way, so the keyboard is never shut in.
func (d *DialogWidget) Dismissible(v bool) *DialogWidget { d.dismissible = v; return d }

// Compact removes the outer padding for content with its own spacing.
func (d *DialogWidget) Compact() *DialogWidget { d.compact = true; return d }

// Width sets the panel's width; it shrinks to fit a narrower window.
func (d *DialogWidget) Width(w float64) *DialogWidget { d.width = w; return d }

// OnClose fires when the dialog closes, however it closed.
func (d *DialogWidget) OnClose(fn func()) *DialogWidget { d.onClose = fn; return d }

// Layout implements Widget: the dialog takes no room in the tree.
func (d *DialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	d.build(env).Border(t.BorderWidth, t.Border).Radius(t.RadiusLg)
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
	d.paint(dst, screen, ggui.Rct(at, size), t.Scrim, d)
}
