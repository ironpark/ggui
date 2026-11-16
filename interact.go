package ggui

import "github.com/ironpark/ggui/internal/property"

// Wrappers that make a child take input: the pointer, through Pointer and
// Tap, and the keyboard, through Focus.

// PointerWidget makes its child react to the pointer. Build one with Pointer
// or Tap. It takes no space of its own: the child's Rect is the hit region.
type PointerWidget struct {
	props       property.Owner
	child       Widget
	cursor      CursorShape
	shaped      bool // Cursor was called: the region asks for cursor
	onDown      func(PointerEvent)
	onUp        func(PointerEvent)
	onTap       func()
	onMove      func(PointerEvent)
	onDrag      func(PointerEvent)
	onEnter     func()
	onExit      func()
	onScroll    func(Point)
	onDrop      func(DropEvent)
	onDropHover func(bool)
	dropHovered bool
}

// Pointer wraps child in a hit region. Attach callbacks with the On methods;
// the widget consumes only the events it has a callback for.
func Pointer(child Widget) *PointerWidget { return &PointerWidget{child: child} }

// Tap is Pointer with an OnTap callback.
func Tap(child Widget, fn func()) *PointerWidget { return Pointer(child).OnTap(fn) }

// OnTap fires when a button goes down and comes up inside the child.
func (p *PointerWidget) OnTap(fn func()) *PointerWidget { p.onTap = fn; return p }

// OnDown fires when a button goes down inside the child.
func (p *PointerWidget) OnDown(fn func(PointerEvent)) *PointerWidget { p.onDown = fn; return p }

// OnUp fires when a button comes up inside the child.
func (p *PointerWidget) OnUp(fn func(PointerEvent)) *PointerWidget { p.onUp = fn; return p }

// OnMove fires every frame the cursor is inside the child.
func (p *PointerWidget) OnMove(fn func(PointerEvent)) *PointerWidget { p.onMove = fn; return p }

// OnDrag fires every frame a button pressed inside the child is still held,
// with the cursor's current position, inside or out. OnUp ends it.
func (p *PointerWidget) OnDrag(fn func(PointerEvent)) *PointerWidget { p.onDrag = fn; return p }

// Cursor sets the mouse cursor shown while the pointer is over the child,
// such as CursorShapePointer for something clickable. CursorShapeDefault
// asks for the arrow, over whatever a widget painted beneath asks for.
func (p *PointerWidget) Cursor(shape CursorShape) *PointerWidget {
	defer property.Watch(&p.props, &p.cursor)()
	defer property.Watch(&p.props, &p.shaped)()
	p.cursor, p.shaped = shape, true
	return p
}

// OnEnter fires when the cursor enters the child.
func (p *PointerWidget) OnEnter(fn func()) *PointerWidget { p.onEnter = fn; return p }

// OnExit fires when the cursor leaves the child.
func (p *PointerWidget) OnExit(fn func()) *PointerWidget { p.onExit = fn; return p }

// OnHover fires with true on enter and false on exit; hover.Set fits it.
func (p *PointerWidget) OnHover(fn func(bool)) *PointerWidget {
	return p.OnEnter(func() { fn(true) }).OnExit(func() { fn(false) })
}

// OnScroll fires with the wheel delta when the wheel moves over the child.
func (p *PointerWidget) OnScroll(fn func(delta Point)) *PointerWidget { p.onScroll = fn; return p }

// OnDrop fires when files are dropped onto the child from the desktop,
// which makes the child a drop zone: a drop over it goes here rather than
// to the handlers registered with Host.OnDrop.
func (p *PointerWidget) OnDrop(fn func(DropEvent)) *PointerWidget { p.onDrop = fn; return p }

// OnDropHover fires with true when files are dragged over the child and
// false when they leave it or are dropped, so the zone can highlight while
// a drop would land on it; hover.Set fits it. Only a platform that reports
// the drag delivers it, which is macOS today; see DragHandler.
func (p *PointerWidget) OnDropHover(fn func(bool)) *PointerWidget { p.onDropHover = fn; return p }

// HandleDrag implements DragHandler. A drop zone claims the drag whether or
// not it listens to it, so that the zone which lights up is the one the
// drop will reach.
func (p *PointerWidget) HandleDrag(ev DragEvent) bool {
	if p.onDrop == nil {
		return false
	}
	if p.onDropHover == nil {
		return true
	}
	switch ev.Kind {
	case DragOver:
		if !p.dropHovered {
			p.dropHovered = true
			p.onDropHover(true)
		}
	case DragExit:
		if p.dropHovered {
			p.dropHovered = false
			p.onDropHover(false)
		}
	}
	return true
}

// HandleDrop implements DropHandler.
func (p *PointerWidget) HandleDrop(ev DropEvent) bool {
	if p.onDrop == nil {
		return false
	}
	p.onDrop(ev)
	return true
}

// HandlePointer implements PointerHandler.
func (p *PointerWidget) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if p.onDown != nil {
			p.onDown(ev)
		}
		return p.onDown != nil || p.onTap != nil || p.onDrag != nil
	case PointerUp:
		if p.onUp != nil {
			p.onUp(ev)
		}
		return p.onUp != nil || p.onTap != nil || p.onDrag != nil
	case PointerDrag:
		if p.onDrag != nil {
			p.onDrag(ev)
		}
		return p.onDrag != nil
	case PointerTap:
		if p.onTap != nil {
			p.onTap()
		}
		return p.onTap != nil
	case PointerMove:
		if p.onMove != nil {
			p.onMove(ev)
		}
		return p.onMove != nil || p.onEnter != nil || p.onExit != nil
	case PointerEnter:
		if p.onEnter != nil {
			p.onEnter()
		}
		return true
	case PointerExit:
		if p.onExit != nil {
			p.onExit()
		}
		return true
	case PointerScroll:
		if p.onScroll != nil {
			p.onScroll(ev.Scroll)
		}
		return p.onScroll != nil
	}
	return false
}

// Layout implements Widget.
func (p *PointerWidget) Layout(c Constraints, env Env) Size {
	defer p.props.Layout().EndLayout()
	return p.child.Layout(c, env)
}

// Baseline implements Baseliner: the child's.
func (p *PointerWidget) Baseline() (float64, bool) { return BaselineOf(p.child) }

// Paint implements Widget.
func (p *PointerWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	if p.shaped {
		dst.HitCursor(r, p.cursor)
	}
	dst.Paint(p.child, r)
}

// FocusWidget gives its child keyboard focus when clicked. Build one with
// Focus.
type FocusWidget struct {
	child   Widget
	onKey   func(KeyboardKey)
	onText  func(string)
	onFocus func(bool)
}

// Focus wraps child in a focusable region. Clicking inside focuses it; keys
// typed while focused reach the On callbacks.
func Focus(child Widget) *FocusWidget { return &FocusWidget{child: child} }

// OnKey fires once per key press while focused.
func (f *FocusWidget) OnKey(fn func(KeyboardKey)) *FocusWidget { f.onKey = fn; return f }

// OnText fires with the characters typed this frame while focused.
func (f *FocusWidget) OnText(fn func(string)) *FocusWidget { f.onText = fn; return f }

// OnFocus fires with true on focus and false on blur; focused.Set fits it.
func (f *FocusWidget) OnFocus(fn func(bool)) *FocusWidget { f.onFocus = fn; return f }

// HandleKey implements KeyHandler.
func (f *FocusWidget) HandleKey(ev KeyEvent) {
	switch ev.Kind {
	case KeyPress:
		if f.onKey != nil {
			f.onKey(ev.Key)
		}
	case KeyText:
		if f.onText != nil {
			f.onText(ev.Text)
		}
	case KeyFocus:
		if f.onFocus != nil {
			f.onFocus(true)
		}
	case KeyBlur:
		if f.onFocus != nil {
			f.onFocus(false)
		}
	}
}

// Layout implements Widget.
func (f *FocusWidget) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

// Baseline implements Baseliner: the child's.
func (f *FocusWidget) Baseline() (float64, bool) { return BaselineOf(f.child) }

// Paint implements Widget.
func (f *FocusWidget) Paint(dst *Canvas, r Rect) {
	dst.HitKey(r, f)
	dst.Paint(f.child, r)
}
