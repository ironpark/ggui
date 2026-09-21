package ui

import "github.com/ironpark/ggui/internal/property"

import "github.com/ironpark/ggui"

// DialogWidget is a modal panel centered over the window: a scrim dims
// everything else and takes the clicks, Tab stays inside, Escape or a
// click on the scrim closes it, and focus returns to where it was. Build
// one with Dialog. It takes no space where it sits in the tree; put it
// anywhere. Keep the widget alive across rebuilds to preserve its animation.
type DialogWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	modal
	width  float64
	effect *ggui.TransitionWidget
	reveal ggui.Motion
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

// Name sets an accessible name without adding a visible heading.
func (d *DialogWidget) Name(name string) *DialogWidget {
	if d.nameReader != nil {
		d.nameReader = nil
		d.props.Changed()
	}
	defer property.Watch(&d.props, &d.name)()
	d.name = name
	return d
}

// Dismissible says whether a click on the scrim closes the dialog; it does
// by default. A confirmation that must be answered sets it false and closes
// itself from its own buttons, as AlertDialog does. Escape closes either
// way, so the keyboard is never shut in.
func (d *DialogWidget) Dismissible(v bool) *DialogWidget {
	defer property.Watch(&d.props, &d.dismissible)()
	d.dismissible = v
	return d
}

// Compact removes the outer padding for content with its own spacing.
func (d *DialogWidget) Compact() *DialogWidget {
	defer property.Watch(&d.props, &d.compact)()
	d.compact = true
	return d
}

// Width sets the panel's width; it shrinks to fit a narrower window.
func (d *DialogWidget) Width(w float64) *DialogWidget {
	defer property.Watch(&d.props, &d.width)()
	d.width = w
	return d
}

// OnClose fires when the dialog closes, however it closed.
func (d *DialogWidget) OnClose(fn func()) *DialogWidget { d.onClose = fn; return d }

// Layout implements Widget: the dialog takes no room in the tree.
func (d *DialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer d.props.Layout()()
	if d.nameReader != nil {
		d.name = d.nameReader.Get()
	}
	t := env.Theme()
	d.build(env).Border(t.BorderWidth, t.Border).Radius(t.RadiusLg)
	if d.effect == nil {
		d.effect = ggui.PopIn(ggui.FromFuncs(
			func(c ggui.Constraints, e ggui.Env) ggui.Size { return d.panel.Layout(c, e) },
			func(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.panel, r) },
		))
	}
	return c.Constrain(ggui.Size{})
}

// Paint implements Widget: an open dialog paints through Canvas.Overlay.
func (d *DialogWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	open := ggui.Untrack(d.open.Get)
	now := ggui.Now()
	v := d.reveal.Toggle(open, now, d.env.Motion(d.theme.MotionFast))
	if !open && v <= 0 {
		d.rect = ggui.Rect{}
		return
	}
	dst.Overlay(func(dst *ggui.Canvas) { d.paintPanel(dst, v, open) })
}

func (d *DialogWidget) paintPanel(dst *ggui.Canvas, progress float64, open bool) {
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
	size := d.effect.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: max(maxH, 0)}, d.env)
	at := ggui.Pt((screen.W-size.W)/2, margin)
	if screen.H != ggui.Unbounded {
		at.Y = (screen.H - size.H) / 2
	}
	if !open {
		dst = dst.Inert()
	}
	d.effect.Progress(progress, !open)
	d.paintContent(dst, screen, ggui.Rct(at, size), fade(t.Scrim, progress), d, d.effect)
}

// BindName follows a non-nil accessible-name reader.
func (d *DialogWidget) BindName(r ggui.Readable[string]) *DialogWidget {
	property.Require(r, "BindName")
	if !property.Same(d.nameReader, r) {
		d.nameReader = r
		d.props.Changed()
	}
	return d
}
