package ggui

import "github.com/hajimehoshi/ebiten/v2"

// PointerKind says what a PointerEvent reports.
type PointerKind int

const (
	PointerDown   PointerKind = iota // a button went down inside the region
	PointerUp                        // a button came up inside the region
	PointerTap                       // down and up happened inside the same region
	PointerMove                      // the cursor is inside the region this frame
	PointerEnter                     // the region became the hovered one
	PointerExit                      // the region stopped being the hovered one
	PointerScroll                    // the wheel moved over the region
)

// PointerEvent is a mouse or touch event delivered to a hit region.
type PointerEvent struct {
	Kind   PointerKind
	Pos    Point // in window coordinates
	Button ebiten.MouseButton
	Scroll Point // wheel delta, for PointerScroll
}

// PointerHandler receives pointer events whose position fell inside the
// region it was registered with. Returning true consumes the event; false
// lets the region beneath see it, so a tap target does not block scrolling.
type PointerHandler interface {
	HandlePointer(ev PointerEvent) bool
}

// KeyKind says what a KeyEvent reports.
type KeyKind int

const (
	KeyPress KeyKind = iota // Key was just pressed
	KeyText                 // Text was typed
	KeyFocus                // the region gained keyboard focus
	KeyBlur                 // the region lost keyboard focus
)

// KeyEvent is a keyboard event delivered to the focused region.
type KeyEvent struct {
	Kind KeyKind
	Key  ebiten.Key // for KeyPress
	Text string     // for KeyText
}

// KeyHandler receives keyboard events while its region has focus. A region
// gains focus when a button goes down inside it and loses it when the button
// goes down elsewhere or the region disappears.
type KeyHandler interface {
	HandleKey(ev KeyEvent)
}

// frameInput is everything the runtime read from the platform this frame.
type frameInput struct {
	pos   Point
	down  []ebiten.MouseButton
	up    []ebiten.MouseButton
	wheel Point
	keys  []ebiten.Key
	text  string
}

// inputState routes frameInput to the regions painted last frame. Regions
// are matched across frames by Rect, so a tree rebuilt between a press and
// a release still completes the tap.
type inputState struct {
	regions []hitRegion

	hovered    *hitRegion
	pressed    *hitRegion
	pressedBtn ebiten.MouseButton
	focused    *hitRegion
}

func (in *inputState) dispatch(f frameInput) {
	// Hover: the topmost region that claims PointerMove is the hovered one.
	move := PointerEvent{Kind: PointerMove, Pos: f.pos}
	now := in.send(move)
	if !sameRegion(now, in.hovered) {
		if in.hovered != nil {
			in.hovered.pointer.HandlePointer(PointerEvent{Kind: PointerExit, Pos: f.pos})
		}
		if now != nil {
			now.pointer.HandlePointer(PointerEvent{Kind: PointerEnter, Pos: f.pos})
		}
	}
	in.hovered = now

	for _, b := range f.down {
		ev := PointerEvent{Kind: PointerDown, Pos: f.pos, Button: b}
		in.pressed, in.pressedBtn = in.send(ev), b
		in.setFocus(in.findKey(f.pos))
	}
	for _, b := range f.up {
		ev := PointerEvent{Kind: PointerUp, Pos: f.pos, Button: b}
		in.send(ev)
		if p := in.pressed; p != nil && b == in.pressedBtn && p.rect.Contains(f.pos) {
			if cur := in.findRect(p.rect); cur != nil {
				cur.pointer.HandlePointer(PointerEvent{Kind: PointerTap, Pos: f.pos, Button: b})
			}
		}
		in.pressed = nil
	}
	if f.wheel != (Point{}) {
		in.send(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheel})
	}

	if in.focused != nil {
		cur := in.findRect(in.focused.rect)
		if cur == nil || cur.key == nil {
			in.setFocus(nil)
		} else {
			in.focused = cur
			for _, k := range f.keys {
				cur.key.HandleKey(KeyEvent{Kind: KeyPress, Key: k})
			}
			if f.text != "" {
				cur.key.HandleKey(KeyEvent{Kind: KeyText, Text: f.text})
			}
		}
	}
}

// topmost returns the last region painted that match accepts, or nil. Later
// regions sit on top of earlier ones, so the scan runs backwards.
func (in *inputState) topmost(match func(*hitRegion) bool) *hitRegion {
	for i := len(in.regions) - 1; i >= 0; i-- {
		if r := &in.regions[i]; match(r) {
			return r
		}
	}
	return nil
}

// send offers ev to regions under it from the top down until one consumes it
// and returns that region, or nil.
func (in *inputState) send(ev PointerEvent) *hitRegion {
	return in.topmost(func(r *hitRegion) bool {
		return r.pointer != nil && r.rect.Contains(ev.Pos) && r.pointer.HandlePointer(ev)
	})
}

// findKey returns the topmost region with a KeyHandler under p.
func (in *inputState) findKey(p Point) *hitRegion {
	return in.topmost(func(r *hitRegion) bool { return r.key != nil && r.rect.Contains(p) })
}

// findRect returns the topmost region painted exactly at rect.
func (in *inputState) findRect(rect Rect) *hitRegion {
	return in.topmost(func(r *hitRegion) bool { return r.rect == rect })
}

func (in *inputState) setFocus(r *hitRegion) {
	if sameRegion(r, in.focused) {
		return
	}
	if in.focused != nil && in.focused.key != nil {
		in.focused.key.HandleKey(KeyEvent{Kind: KeyBlur})
	}
	if r != nil && r.key != nil {
		r.key.HandleKey(KeyEvent{Kind: KeyFocus})
	}
	in.focused = r
}

func sameRegion(a, b *hitRegion) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.rect == b.rect
}

// PointerWidget makes its child react to the pointer. Build one with Pointer
// or Tap. It takes no space of its own: the child's Rect is the hit region.
type PointerWidget struct {
	child    Widget
	onDown   func(PointerEvent)
	onUp     func(PointerEvent)
	onTap    func()
	onMove   func(PointerEvent)
	onEnter  func()
	onExit   func()
	onScroll func(Point)
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

// HandlePointer implements PointerHandler.
func (p *PointerWidget) HandlePointer(ev PointerEvent) bool {
	switch ev.Kind {
	case PointerDown:
		if p.onDown != nil {
			p.onDown(ev)
		}
		return p.onDown != nil || p.onTap != nil
	case PointerUp:
		if p.onUp != nil {
			p.onUp(ev)
		}
		return p.onUp != nil || p.onTap != nil
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
func (p *PointerWidget) Layout(c Constraints) Size { return p.child.Layout(c) }

// Paint implements Widget.
func (p *PointerWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	p.child.Paint(dst, r)
}

// FocusWidget gives its child keyboard focus when clicked. Build one with
// Focus.
type FocusWidget struct {
	child   Widget
	onKey   func(ebiten.Key)
	onText  func(string)
	onFocus func(bool)
}

// Focus wraps child in a focusable region. Clicking inside focuses it; keys
// typed while focused reach the On callbacks.
func Focus(child Widget) *FocusWidget { return &FocusWidget{child: child} }

// OnKey fires once per key press while focused.
func (f *FocusWidget) OnKey(fn func(ebiten.Key)) *FocusWidget { f.onKey = fn; return f }

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
func (f *FocusWidget) Layout(c Constraints) Size { return f.child.Layout(c) }

// Paint implements Widget.
func (f *FocusWidget) Paint(dst *Canvas, r Rect) {
	dst.HitKey(r, f)
	f.child.Paint(dst, r)
}
