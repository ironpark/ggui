package ggui

import "github.com/ironpark/ggui/gem"

// activeScope returns the topmost focus scope painted this frame.
func (in *inputState) activeScope() *focusScope {
	if i := in.index().scopeAt; i >= 0 {
		return in.regions[i].scope
	}
	return nil
}

// updateTrap notices a focus scope appearing or going. When one appears,
// focus that is outside it moves to its first key region and where focus
// was is remembered; when it goes, focus that went with it returns there.
func (in *inputState) updateTrap() {
	var owner any
	if s := in.activeScope(); s != nil {
		owner = s.owner
	}
	if gem.Same(owner, in.trap) {
		return
	}
	if owner == nil {
		in.trap = nil
		if in.focused == nil || in.findKeyRegion(in.focused) == nil {
			if r := in.trapReturn; r != nil {
				in.focus(in.findKeyRegion(r), false)
			}
		}
		in.trapReturn = nil
		return
	}
	if in.trap == nil {
		in.trapReturn = keep(in.focused)
	}
	in.trap = owner
	inside := func(r *hitRegion) bool { return r.scope != nil && gem.Same(r.scope.owner, owner) }
	if in.focused != nil {
		if cur := in.findKeyRegion(in.focused); cur != nil && inside(cur) {
			return
		}
	}
	for i := range in.regions {
		if r := &in.regions[i]; r.key != nil && inside(r) {
			in.focus(r, false)
			return
		}
	}
}

// refreshFocused moves the focused region to where its widget painted this
// frame, so the next paint's FocusWithin sees it there: a region focused
// while scrolled out of view was kept with no area. Only the same handler
// counts; one rebuilt in its place is left for dispatchKeys to tell it has
// focus.
func (in *inputState) refreshFocused() {
	if in.focused == nil {
		return
	}
	if r := in.regionOf(in.focused.key, true); r != nil {
		in.focused = in.focusSlot.keep(r)
	}
}

func (in *inputState) setFocus(r *hitRegion) { in.focus(r, false) }

// focus moves keyboard focus to r. A move made with the keyboard is
// reported with Key set to KeyTab, so a control can show a focus ring only
// then, as browsers do with :focus-visible.
func (in *inputState) focus(r *hitRegion, keyboard bool) {
	if r == nil && in.focused == nil {
		return
	}
	if r != nil && in.focused != nil && (gem.Same(r.key, in.focused.key) || in.continues(r, in.focused)) {
		// The same widget, perhaps in another region, such as a dropdown's
		// list under its field: focus stays, without a blur, and becomes
		// visible if the keyboard moved it there.
		in.focused = in.focusSlot.keep(r)
		in.focusVisible = in.focusVisible || keyboard
		in.updateFocusWithin(r)
		return
	}
	if in.focused != nil && in.focused.key != nil {
		RepaintAll()
		in.focused.key.HandleKey(KeyEvent{Kind: KeyBlur})
	}
	if r != nil && r.key != nil {
		RepaintAll()
		ev := KeyEvent{Kind: KeyFocus}
		if keyboard {
			ev.Key = KeyTab
			in.reveal(r.full)
		}
		r.key.HandleKey(ev)
	}
	in.focused = in.focusSlot.keep(r)
	in.focusVisible = keyboard
	in.updateFocusWithin(r)
}

// reveal asks every Revealer painted to scroll target into view; each
// decides whether target is inside its content.
func (in *inputState) reveal(target Rect) {
	for i := range in.regions {
		if v, ok := in.regions[i].pointer.(Revealer); ok {
			v.Reveal(target)
		}
	}
}

// moveFocus steps focus through the key regions in paint order, wrapping
// at the ends: Tab is dir 1, Shift+Tab is -1. While a focus scope is
// showing, only its regions take part.
func (in *inputState) moveFocus(dir int) {
	var keyed []*hitRegion
	cur := -1
	scope := in.activeScope()
	// The focused region may have moved since it was recorded, as after a
	// Reveal, so find it the way a frame does, handler before Rect.
	var focused *hitRegion
	if in.focused != nil {
		focused = in.findKeyRegion(in.focused)
	}
	for i := range in.regions {
		r := &in.regions[i]
		if r.key == nil || !r.inScope(scope) {
			continue
		}
		if r == focused || focused != nil && gem.Same(r.key, focused.key) {
			// The focused widget is one stop, at its first region, however
			// many it painted: a combobox's input also takes the keyboard
			// over its list.
			if cur < 0 {
				cur = len(keyed)
				keyed = append(keyed, r)
			}
			continue
		}
		if t, ok := r.key.(TabStopper); ok && !t.TabStop() {
			continue
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

// releasePress takes the press away from the widget under it when a drag
// or a pan takes over: it gets a PointerUp without a tap.
func (in *inputState) releasePress(pos Point, mods Mods) {
	if in.pressed == nil {
		return
	}
	if cur := in.findPointer(in.pressed); cur != nil {
		in.deliver(cur, PointerEvent{Kind: PointerUp, Pos: pos, Button: in.pressedBtn, Mods: mods})
	}
	in.pressed = nil
}

// applyRevealRequests runs the Canvas.RequestReveal calls this paint made.
func (in *inputState) applyRevealRequests(f *frameState) {
	for _, r := range f.revealRequests {
		in.reveal(r)
	}
	f.revealRequests = f.revealRequests[:0]
}

func (in *inputState) applyFocusRequest(f *frameState) {
	h := f.focusRequest
	f.focusRequest = nil
	if h == nil {
		return
	}
	for i := range in.regions {
		r := &in.regions[i]
		if gem.Same(r.key, h) {
			if scope := in.activeScope(); scope != nil && r.scope != scope {
				return
			}
			in.focus(r, true)
			return
		}
	}
}
