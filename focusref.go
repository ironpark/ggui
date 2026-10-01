package ggui

// FocusRef focuses a widget from code: a form that failed validation
// focuses the first bad field, a search panel its input when it opens.
// Attach it around the widget, then call Focus from a handler, an effect or
// posted work; the zero value is ready to use.
//
//	var name ggui.FocusRef
//	form := ggui.Column(name.Attach(ui.TextField(value)), save)
//	...
//	name.Focus()
type FocusRef struct {
	pending bool
	focused bool
}

// Focus moves keyboard focus to the first focusable widget inside the
// attached one when it is next painted, which for a call from a handler or
// posted work is this frame. Focus inside an open dialog's trap stays
// there; a widget outside it is not focused. It does nothing to an attached
// widget with nothing focusable, such as a disabled field.
func (f *FocusRef) Focus() { f.pending = true }

// Focused reports whether focus was inside the attached widget when it
// was last painted.
func (f *FocusRef) Focused() bool { return f.focused }

// Attach wraps child as the widget Focus focuses.
func (f *FocusRef) Attach(child Widget) Widget { return &focusRefWidget{ref: f, child: child} }

type focusRefWidget struct {
	ref   *FocusRef
	child Widget
}

// Layout implements Widget.
func (w *focusRefWidget) Layout(c Constraints, env Env) Size { return w.child.Layout(c, env) }

// Baseline implements Baseliner: the child's.
func (w *focusRefWidget) Baseline() (float64, bool) { return baselineOf(w.child) }

// Paint implements Widget: while a Focus is pending, the first key region
// the child registers claims it.
func (w *focusRefWidget) Paint(dst *Canvas, r Rect) {
	if dst == nil {
		w.child.Paint(nil, r)
		return
	}
	w.ref.focused = dst.FocusWithin(r)
	if !w.ref.pending {
		dst.Paint(w.child, r)
		return
	}
	w.ref.pending = false
	root := dst.root()
	prev := root.focusClaim
	root.focusClaim = w.ref
	defer func() { root.focusClaim = prev }()
	dst.Paint(w.child, r)
}
