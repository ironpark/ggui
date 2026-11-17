package widget

import "github.com/ironpark/ggui"

// WithinWidget tells what goes on inside its child without taking any of
// it: the pointer over it, focus in it, a button going down in it, the keys
// of what has focus in it. Build one with Within. It takes no space of its
// own: the child's Rect is its region, and what the child paints is inside
// it wherever that shows.
type WithinWidget struct {
	child   ggui.Widget
	onHover func(bool)
	onFocus func(bool)
	onDown  func(ggui.PointerEvent)
	onKey   func(ggui.KeyEvent) bool
}

// Within wraps child in a region that hears what goes on inside it. Attach
// callbacks with the On methods, before the widget is mounted.
//
// A tab that shows its close button while the pointer or the focus is in
// it builds the button always and shows it by its signals, so that it
// stays in the Tab order and the accessibility tree:
//
//	hover, focus := ggui.State(false), ggui.State(false)
//	tab := widget.Within(widget.Row(label, closeButton)).
//		OnHover(hover.Set).OnFocus(focus.Set)
func Within(child ggui.Widget) *WithinWidget { return &WithinWidget{child: child} }

// OnHover fires with true when the pointer comes over the child or anything
// painted inside it, and with false when it has left all of it; hover.Set
// fits it. A Within with OnHover claims the pointer's hover over the child,
// as Pointer's OnHover does: a widget beneath that is not around it is not
// hovered through it.
func (w *WithinWidget) OnHover(fn func(bool)) *WithinWidget { w.onHover = fn; return w }

// OnFocus fires with true when keyboard focus comes inside the child and
// with false when it has left; focused.Set fits it. It is not called as
// focus moves from one widget inside to another.
func (w *WithinWidget) OnFocus(fn func(bool)) *WithinWidget { w.onFocus = fn; return w }

// OnDown fires when a button goes down over the child or anything painted
// inside it, before whatever it landed on hears of it, and whether or not
// that takes it. A press on something painted over the child that is not
// part of it, such as a popup, is not heard.
func (w *WithinWidget) OnDown(fn func(ggui.PointerEvent)) *WithinWidget { w.onDown = fn; return w }

// OnKey is offered the key presses of the widget that has focus inside the
// child, where a shortcut would run: a chord with a modifier before that
// widget, a bare key only if the widget left it. Return true to take the
// press.
func (w *WithinWidget) OnKey(fn func(ggui.KeyEvent) bool) *WithinWidget { w.onKey = fn; return w }

// HandlePointer implements ggui.PointerHandler, for the hover alone: it
// claims the move, so that it is on the hover path, and takes nothing else.
func (w *WithinWidget) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerMove:
		return w.onHover != nil
	case ggui.PointerEnter:
		if w.onHover != nil {
			w.onHover(true)
		}
	case ggui.PointerExit:
		if w.onHover != nil {
			w.onHover(false)
		}
	}
	return false
}

// HandlePointerWithin implements ggui.PointerWithinHandler: it hears a
// press and lets it go on.
func (w *WithinWidget) HandlePointerWithin(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && w.onDown != nil {
		w.onDown(ev)
	}
	return false
}

// HandleFocusWithin implements ggui.FocusWithinHandler.
func (w *WithinWidget) HandleFocusWithin(within bool) {
	if w.onFocus != nil {
		w.onFocus(within)
	}
}

// HandleKeyWithin implements ggui.KeyWithinHandler.
func (w *WithinWidget) HandleKeyWithin(ev ggui.KeyEvent) bool {
	return w.onKey != nil && w.onKey(ev)
}

// Layout implements ggui.Widget.
func (w *WithinWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	return w.child.Layout(c, env)
}

// Baseline implements ggui.Baseliner: the child's.
func (w *WithinWidget) Baseline() (float64, bool) { return ggui.BaselineOf(w.child) }

// Paint implements ggui.Widget.
func (w *WithinWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.HitWithin(r, w)
	if w.onHover != nil {
		// The same Rect: one region with the within handler.
		dst.HitPointer(r, w)
	}
	dst.Paint(w.child, r)
}
