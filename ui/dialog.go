package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

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
	top    float64 // the panel's distance from the window's top; 0 centres it
	topAt  float64 // the share of the window's height above the panel, when top is 0
	effect *ggui.TransitionWidget
	reveal ggui.Motion
}

// dialogWidth is a dialog's width until Width says otherwise: shadcn's
// sm:max-w-sm.
const dialogWidth = 384

// Dialog creates a dialog that shows content while open is true.
//
//	confirm := ggui.State(false)
//	ui.Dialog(confirm, ggui.Column(
//		ggui.Text("Delete the file?"),
//		ggui.Row(ui.Button("Delete", del), ui.Button("Cancel", func() { confirm.Set(false) }).Outline()),
//	)).Title("Confirm")
func Dialog(open ggui.Binding[bool], content ggui.Widget) *DialogWidget {
	d := &DialogWidget{width: dialogWidth}
	d.open, d.content, d.dismissible = open, content, true
	return d
}

// Title puts a heading above the content, which also names the dialog for
// Probe.Find.
func (d *DialogWidget) Title(s string) *DialogWidget { d.setTitle(s); return d }

// Description puts a line of muted text under the title, saying what the
// dialog is for. An empty string removes it.
func (d *DialogWidget) Description(s string) *DialogWidget {
	d.setDescription(s)
	d.props.Changed()
	return d
}

// Footer puts actions in a band along the dialog's bottom edge, in a row at
// its end: the buttons that answer it. The band is tinted and runs from
// edge to edge, whatever Compact says about the content. Calling it with no
// actions removes the band.
//
//	ui.Dialog(open, form).Title("Edit profile").
//		Footer(ui.Button("Cancel", cancel).Outline(), ui.Button("Save", save))
func (d *DialogWidget) Footer(actions ...ggui.Widget) *DialogWidget {
	d.setFooter(actions)
	d.props.Changed()
	return d
}

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

// Top holds the panel's top y pixels below the window's, rather than
// centring it, so a panel whose height follows its content, such as a
// command palette's, stays put as it grows and shrinks. It keeps the
// margin of a centred panel at least; 0 centres it again.
func (d *DialogWidget) Top(y float64) *DialogWidget {
	defer property.Watch(&d.props, &d.top)()
	d.top = max(y, 0)
	return d
}

// TopAt holds the panel's top at share of the window's height below its
// top, as shadcn's command dialog does at a third, rather than centring
// it; like Top it keeps the panel in place as its content grows. Top, when
// set, wins; 0 centres the panel again.
func (d *DialogWidget) TopAt(share float64) *DialogWidget {
	defer property.Watch(&d.props, &d.topAt)()
	d.topAt = clamp(share, 0, 1)
	return d
}

// OnClose fires when the dialog closes, however it closed.
func (d *DialogWidget) OnClose(fn func()) *DialogWidget { d.onClose = fn; return d }

// Layout implements Widget: the dialog takes no room in the tree.
func (d *DialogWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer d.props.Layout().EndLayout()
	if d.nameReader != nil {
		d.name = d.nameReader.Get()
	}
	t := uitheme.Of(env)
	d.build(env, modalShape{
		headerGap: t.Space, titleLeading: 1, closeInset: t.Space, band: true, fillColor: t.Popover,
		borderWidth: t.BorderWidth, border: fade(t.Fg, .1), radius: t.RadiusLg,
	})
	if d.effect == nil {
		d.effect = ggui.PopIn(ggui.FromFuncs(
			func(c ggui.Constraints, e ggui.Env) ggui.Size { return d.panel.Layout(c, e) },
			d.paintSurface,
		))
	}
	return c.Constrain(ggui.Size{})
}

// Paint implements Widget: an open dialog paints through Canvas.Overlay.
func (d *DialogWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	open := ggui.Untrack(d.open.Get)
	now := ggui.FrameTime()
	v := d.reveal.Toggle(open, now, d.env.Motion(d.theme.MotionOverlay))
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
	fixed := d.top
	if fixed == 0 && d.topAt > 0 && screen.H != ggui.Unbounded {
		fixed = screen.H * d.topAt
	}
	top := max(fixed, margin)
	w := min(d.width, max(screen.W-2*margin, 0))
	maxH := screen.H - 2*margin
	if fixed > 0 {
		maxH = screen.H - top - margin
	}
	if screen.H == ggui.Unbounded {
		maxH = ggui.Unbounded
	}
	size := d.effect.Layout(ggui.Constraints{MinW: w, MaxW: w, MaxH: max(maxH, 0)}, d.env)
	at := ggui.Pt((screen.W-size.W)/2, margin)
	switch {
	case fixed > 0 && screen.H != ggui.Unbounded:
		at.Y = min(top, max(screen.H-margin-size.H, margin))
	case screen.H != ggui.Unbounded:
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

// CloseButton controls the top-right close action, visible by default.
func (d *DialogWidget) CloseButton(show bool) *DialogWidget {
	defer property.Watch(&d.props, &d.hideClose)()
	d.hideClose = !show
	return d
}
