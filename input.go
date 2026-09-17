package ggui

import (
	"runtime"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
)

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
	PointerDrag                      // a button that went down inside the region is still held, wherever the cursor is now
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
//
// A region that consumed PointerDown captures the pointer: until the button
// comes up it receives PointerDrag every frame and then PointerUp, even when
// the cursor has left it. That is what a slider knob or a text selection
// needs.
type PointerHandler interface {
	HandlePointer(ev PointerEvent) bool
}

// Mods are the modifier keys held during a KeyEvent.
type Mods struct {
	Shift, Ctrl, Alt, Meta bool
}

// Cmd reports the platform's command modifier: Meta (⌘) on macOS, Ctrl
// elsewhere. Shortcuts such as select-all and paste check it.
func (m Mods) Cmd() bool {
	if runtime.GOOS == "darwin" {
		return m.Meta
	}
	return m.Ctrl
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
	Mods Mods       // for KeyPress
}

// KeyHandler receives keyboard events while its region has focus. A region
// gains focus when a button goes down inside it and loses it when the button
// goes down elsewhere or the region disappears. A held key repeats: after a
// short delay it delivers KeyPress again every few frames.
type KeyHandler interface {
	HandleKey(ev KeyEvent)
}

// TickHandler is a KeyHandler that also needs to run once per frame while it
// has focus, before that frame's keys are delivered: a text editor drives the
// platform IME this way. Returning true reports that the IME consumed this
// frame's input, and the KeyPress and KeyText events are then withheld.
type TickHandler interface {
	KeyHandler
	HandleTick() (consumed bool)
}

// frameInput is everything the runtime read from the platform this frame.
type frameInput struct {
	pos   Point
	down  []ebiten.MouseButton
	up    []ebiten.MouseButton
	wheel Point
	keys  []ebiten.Key // just pressed, plus repeats of held keys
	text  string
	mods  Mods
}

// inputState routes frameInput to the regions painted last frame. A pressed
// or focused region is matched across frames by its handler when the widget
// survived, or by Rect when a rebuild replaced it, so a widget that moves
// while dragged keeps the drag and a tap completes across a rebuild.

type inputState struct {
	regions []hitRegion

	// These are copies: regions is rebuilt every frame in a reused buffer,
	// so a pointer into it would soon describe a different region.
	hovered    *hitRegion
	pressed    *hitRegion
	pressedBtn ebiten.MouseButton
	focused    *hitRegion
	cursor     ebiten.CursorShapeType // what the hovered region asked for
}

// keep returns a copy of r that outlives the regions buffer.
func keep(r *hitRegion) *hitRegion {
	if r == nil {
		return nil
	}
	c := *r
	return &c
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
	in.hovered = keep(now)

	in.cursor = ebiten.CursorShapeDefault
	if r := in.topmost(func(r *hitRegion) bool { return r.cursor != 0 && r.rect.Contains(f.pos) }); r != nil {
		in.cursor = r.cursor
	}

	for _, b := range f.down {
		ev := PointerEvent{Kind: PointerDown, Pos: f.pos, Button: b}
		in.pressed, in.pressedBtn = keep(in.send(ev)), b
		in.setFocus(in.findKey(f.pos))
	}
	if in.pressed != nil && len(f.down) == 0 && len(f.up) == 0 {
		if cur := in.findPointer(in.pressed); cur != nil {
			cur.pointer.HandlePointer(PointerEvent{Kind: PointerDrag, Pos: f.pos, Button: in.pressedBtn})
		}
	}
	for _, b := range f.up {
		ev := PointerEvent{Kind: PointerUp, Pos: f.pos, Button: b}
		p := in.pressed
		if p == nil || b != in.pressedBtn {
			in.send(ev)
			continue
		}
		// The region that took the press gets the release, wherever the
		// cursor is, and a tap if it is still inside.
		if cur := in.findPointer(p); cur != nil {
			cur.pointer.HandlePointer(ev)
			if cur.rect.Contains(f.pos) {
				cur.pointer.HandlePointer(PointerEvent{Kind: PointerTap, Pos: f.pos, Button: b})
			}
		}
		in.pressed = nil
	}
	if f.wheel != (Point{}) {
		in.send(PointerEvent{Kind: PointerScroll, Pos: f.pos, Scroll: f.wheel})
	}

	if i := slices.Index(f.keys, ebiten.KeyTab); i >= 0 {
		f.keys = slices.Delete(slices.Clone(f.keys), i, i+1)
		in.moveFocus(pick(f.mods.Shift, -1, 1))
	}

	if in.focused != nil {
		cur := in.findKeyRegion(in.focused)
		if cur == nil {
			in.setFocus(nil)
			return
		}
		if prev := in.focused.key; !sameAny(prev, cur.key) {
			// The tree was rebuilt: the region is the same, the widget new.
			// An Adopter already took the old one's state over during paint;
			// anything else is told it has focus now.
			if _, ok := cur.key.(Adopter); !ok {
				cur.key.HandleKey(KeyEvent{Kind: KeyFocus})
			}
		}
		in.focused = keep(cur)
		if th, ok := cur.key.(TickHandler); ok && th.HandleTick() {
			return
		}
		for _, k := range f.keys {
			cur.key.HandleKey(KeyEvent{Kind: KeyPress, Key: k, Mods: f.mods})
		}
		if f.text != "" {
			cur.key.HandleKey(KeyEvent{Kind: KeyText, Text: f.text})
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

// findPointer returns the region that continues p this frame: the one
// with the same handler, wherever it moved to, or else the topmost pointer
// region painted exactly where p was, which is a rebuilt widget.
func (in *inputState) findPointer(p *hitRegion) *hitRegion {
	if r := in.topmost(func(r *hitRegion) bool { return r.pointer != nil && sameAny(r.pointer, p.pointer) }); r != nil {
		return r
	}
	return in.topmost(func(r *hitRegion) bool { return r.pointer != nil && r.rect == p.rect })
}

// findKeyRegion is findPointer for a key region.
func (in *inputState) findKeyRegion(p *hitRegion) *hitRegion {
	if r := in.topmost(func(r *hitRegion) bool { return r.key != nil && sameAny(r.key, p.key) }); r != nil {
		return r
	}
	return in.topmost(func(r *hitRegion) bool { return r.key != nil && r.rect == p.rect })
}

func (in *inputState) setFocus(r *hitRegion) { in.focus(r, false) }

// focus moves keyboard focus to r. A move made with the keyboard is
// reported with Key set to KeyTab, so a control can show a focus ring only
// then, as browsers do with :focus-visible.
func (in *inputState) focus(r *hitRegion, keyboard bool) {
	if sameRegion(r, in.focused) {
		return
	}
	if in.focused != nil && in.focused.key != nil {
		in.focused.key.HandleKey(KeyEvent{Kind: KeyBlur})
	}
	if r != nil && r.key != nil {
		ev := KeyEvent{Kind: KeyFocus}
		if keyboard {
			ev.Key = ebiten.KeyTab
		}
		r.key.HandleKey(ev)
	}
	in.focused = keep(r)
}

// moveFocus steps focus through the key regions in paint order, wrapping
// at the ends: Tab is dir 1, Shift+Tab is -1.
func (in *inputState) moveFocus(dir int) {
	var keyed []*hitRegion
	cur := -1
	for i := range in.regions {
		r := &in.regions[i]
		if r.key == nil {
			continue
		}
		if in.focused != nil && r.rect == in.focused.rect {
			cur = len(keyed)
		}
		keyed = append(keyed, r)
	}
	if len(keyed) == 0 {
		return
	}
	next := 0
	if dir < 0 {
		next = len(keyed) - 1
	}
	if cur >= 0 {
		next = (cur + dir + len(keyed)) % len(keyed)
	}
	in.focus(keyed[next], true)
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
	cursor   ebiten.CursorShapeType
	onDown   func(PointerEvent)
	onUp     func(PointerEvent)
	onTap    func()
	onMove   func(PointerEvent)
	onDrag   func(PointerEvent)
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

// OnDrag fires every frame a button pressed inside the child is still held,
// with the cursor's current position, inside or out. OnUp ends it.
func (p *PointerWidget) OnDrag(fn func(PointerEvent)) *PointerWidget { p.onDrag = fn; return p }

// Cursor sets the mouse cursor shown while the pointer is over the child,
// such as ebiten.CursorShapePointer for something clickable.
func (p *PointerWidget) Cursor(shape ebiten.CursorShapeType) *PointerWidget {
	p.cursor = shape
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
func (p *PointerWidget) Layout(c Constraints, env Env) Size { return p.child.Layout(c, env) }

// Paint implements Widget.
func (p *PointerWidget) Paint(dst *Canvas, r Rect) {
	dst.HitPointer(r, p)
	if p.cursor != 0 {
		dst.HitCursor(r, p.cursor)
	}
	dst.Paint(p.child, r)
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
func (f *FocusWidget) Layout(c Constraints, env Env) Size { return f.child.Layout(c, env) }

// Paint implements Widget.
func (f *FocusWidget) Paint(dst *Canvas, r Rect) {
	dst.HitKey(r, f)
	dst.Paint(f.child, r)
}
