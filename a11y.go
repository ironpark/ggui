package ggui

import (
	"sync"
	"sync/atomic"
)

// A platform accessibility bridge is the far side of the tree semantics.go
// builds: VoiceOver, and every other assistive technology, does not read
// widgets, it reads a tree of platform objects that answer questions about
// themselves. This file is the half of that bridge which is plain Go --
// which node an object stands for, what role it claims, where it sits on
// screen, and which objects a frame created or retired -- so that all of it
// can be tested without an assistive technology running, or indeed a
// window. The platform half lives behind axPlatform, one file per platform,
// and does nothing but make objects and hand them notifications.
//
// Two rules shape everything here. The first is that queries arrive on the
// operating system's main thread, synchronously, while Ebitengine runs
// frames on a goroutine of its own, so an answer must come from the last
// published SemTree and never from work posted to the frame. The second is
// that an assistive technology holds onto the objects it is given and
// compares them: a node that is still there must hand back the same object
// it did last frame, or focus and the reading position jump. Hence the
// element cache below, which is keyed by identity and swept once a frame.

// AccessibilityMode says when an App talks to the platform's accessibility
// API. The zero value waits for an assistive technology to attach and
// builds nothing at all until one does, so a frame costs nothing in the
// usual case where none is running.
type AccessibilityMode uint8

const (
	// AccessibilityAuto turns the bridge on while an assistive technology
	// is attached, and off again when it detaches.
	AccessibilityAuto AccessibilityMode = iota
	// AccessibilityOff never speaks to the platform, whatever is running.
	AccessibilityOff
	// AccessibilityAlways keeps the bridge on, so that a tool which does
	// not announce itself -- Accessibility Inspector is one -- still sees
	// the tree. It costs a little work every frame.
	AccessibilityAlways
)

// axKey identifies a node from frame to frame, as NodeID does, but as
// something a map will take: NodeID.ID is whatever a widget passed to Key
// and may be a slice, which would panic a lookup. A node with an identity
// is keyed by it alone, so that moving does not make it a different
// element; one without is keyed by the bounds it painted, which is the only
// handle on it there is.
type axKey struct {
	id   any
	rect Rect
	role Role
}

// axKeyOf reduces a NodeID to a cache key.
func axKeyOf(id NodeID) axKey {
	if k := semKey(id.ID); k != nil {
		return axKey{id: k, role: id.Role}
	}
	return axKey{rect: id.Rect, role: id.Role}
}

// axFrame is one published tree together with the lookup the bridge needs
// to answer a query about it: element to node, in one step rather than a
// scan. Like the tree, it is finished when it is stored and never written
// again, so the main thread may read it without a lock.
type axFrame struct {
	tree  *SemTree
	index map[axKey]int32
}

// newAXFrame indexes a tree by cache key. Two nodes can collide -- an
// unidentified node keys on its bounds, and a container exactly filling its
// one child shares them -- in which case the first, which is the outer one,
// wins; the loser is simply not reachable as an element of its own.
func newAXFrame(t *SemTree) *axFrame {
	f := &axFrame{tree: t, index: make(map[axKey]int32, t.Len())}
	for i := range t.Len() {
		k := axKeyOf(t.At(i).ID)
		if _, dup := f.index[k]; !dup {
			f.index[k] = int32(i)
		}
	}
	return f
}

// at returns the node a key names, if the frame still holds one.
func (f *axFrame) at(k axKey) (SemNode, bool) {
	i, ok := f.index[k]
	if !ok {
		return SemNode{}, false
	}
	return f.tree.At(int(i)), true
}

// axPlatform is everything the bridge needs from the operating system.
// Nothing above this line knows what an Objective-C object is, and nothing
// below it knows what a widget is.
type axPlatform interface {
	// active reports whether an assistive technology is attached. It is
	// asked once a second rather than once a frame, since it is the one
	// call the bridge makes while nothing is listening.
	active() bool
	// element makes a fresh accessibility object standing for handle, and
	// returns it retained. It is only ever called from the platform's own
	// thread, in the middle of answering a query, so that no object is
	// created off it.
	element(handle int64) uintptr
	// release drops the bridge's reference to elements whose nodes are
	// gone. It may be called from the frame goroutine and must marshal
	// itself to wherever the platform requires.
	release(elems []uintptr)
	// notify delivers a frame's worth of notifications. It is called from
	// the frame goroutine, at most once a frame and never with an empty
	// list, so a platform that must hop to a particular thread to post
	// them pays for exactly one hop per frame that had something to say.
	notify(notes []axNote)
}

// axPollFrames is how often AccessibilityAuto asks whether an assistive
// technology has attached: once a second at sixty frames, which is far
// cheaper than asking every frame and far quicker than the user can notice.
const axPollFrames = 60

// axBridge holds the published tree and the element cache between the frame
// goroutine, which writes them, and the platform's thread, which reads
// them. The tree goes through an atomic pointer because a query must never
// wait for a frame; the cache goes behind a mutex because both sides change
// it, and the critical sections are short and call nothing that could block.
type axBridge struct {
	plat axPlatform
	mode AccessibilityMode
	on   bool
	poll int

	cur atomic.Pointer[axFrame]

	// prev is last frame's, for the diff. Only the frame goroutine
	// touches it, so it needs neither the lock nor the atomic.
	prev *axFrame
	act  func(NodeID, Action)

	mu    sync.Mutex
	elems axElems
}

// start binds the bridge to its platform. It is called once, from the first
// frame, because the platform half needs a window to attach to and there is
// none before Run. act is what an assistive technology's request to press or
// focus something ends up calling; the bridge takes the function rather than
// the App so that nothing here has to know what an App is.
func (b *axBridge) start(act func(NodeID, Action), mode AccessibilityMode) {
	b.mode = mode
	if mode == AccessibilityOff {
		return
	}
	b.plat = newAXPlatform()
	if b.plat != nil {
		b.act = act
		axAttach(b)
	}
}

// publish makes t the tree every query is answered from, retires the
// elements of the nodes that are no longer in it, and hands the platform
// what changed along with whatever Announce queued. It runs at the end of a
// frame, on the frame goroutine, and returns without waiting for anything.
//
// notices is drained by the caller whether or not the bridge is listening,
// so that an app nobody is reading does not accumulate a queue of things it
// will never say.
func (b *axBridge) publish(t *SemTree, notices []Announcement) {
	if b.plat == nil || !b.enabled() {
		b.prev = nil
		return
	}
	// Still poll for attach/detach above and deliver announcements below:
	// neither depends on whether the semantic description changed.
	if b.prev != nil && b.prev.tree == t {
		if notes := axSpeak(notices); len(notes) > 0 {
			b.plat.notify(notes)
		}
		return
	}
	f := newAXFrame(t)
	notes := append(axDiff(b.prev, f), axSpeak(notices)...)
	b.cur.Store(f)
	b.prev = f
	b.mu.Lock()
	gone := b.elems.sweep(f.index)
	b.mu.Unlock()
	if len(gone) > 0 {
		b.plat.release(gone)
	}
	if len(notes) > 0 {
		b.plat.notify(notes)
	}
}

// enabled reports whether the bridge should do the frame's work, asking the
// platform every axPollFrames frames rather than every frame. Turning off
// drops the tree and every element with it, so that nothing is held once
// the last assistive technology detaches.
func (b *axBridge) enabled() bool {
	if b.mode == AccessibilityOff {
		return false
	}
	if b.poll > 0 {
		b.poll--
		return b.on
	}
	// The poll runs in AccessibilityAlways too, and its answer is thrown
	// away: asking is also what gives the platform half its chance to
	// attach itself to the window, which it can only do from the thread
	// that owns one.
	b.poll = axPollFrames
	was := b.on
	b.on = b.plat.active() || b.mode == AccessibilityAlways
	axWantsDetail.Store(b.on)
	if was && !b.on {
		b.clear()
	}
	return b.on
}

// clear retires every element and forgets the tree, for when the bridge
// goes quiet.
func (b *axBridge) clear() {
	b.cur.Store(nil)
	b.prev = nil
	b.mu.Lock()
	gone := b.elems.sweep(nil)
	b.mu.Unlock()
	if len(gone) > 0 {
		b.plat.release(gone)
	}
}

// frame returns the tree queries are being answered from, or nil before the
// first one.
func (b *axBridge) frame() *axFrame { return b.cur.Load() }

// element returns the platform object standing for the node key names,
// making it on the spot if this is the first time it has been asked for,
// and 0 when the node is no longer in the tree. It is called from the
// platform's thread, which is where an object may be made.
func (b *axBridge) element(k axKey) uintptr {
	f := b.frame()
	if f == nil || b.plat == nil {
		return 0
	}
	if _, ok := f.index[k]; !ok {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.elems.intern(k, b.plat.element)
}

// node returns the node an element handle stands for, as the tree last
// published it. An element an assistive technology kept past the life of
// its node resolves to nothing, which is the honest answer.
func (b *axBridge) node(handle int64) (SemNode, bool) {
	f := b.frame()
	if f == nil {
		return SemNode{}, false
	}
	b.mu.Lock()
	k, ok := b.elems.keyOf(handle)
	b.mu.Unlock()
	if !ok {
		return SemNode{}, false
	}
	return f.at(k)
}

// perform hands an action to the app, which queues it onto the frame
// goroutine and returns at once. Nothing here waits for it: see the note at
// the top of actions.go for why waiting is the one thing that deadlocks.
func (b *axBridge) perform(id NodeID, a Action) {
	if b.act != nil {
		b.act(id, a)
	}
}

// axElems is the element cache: one platform object per live node, held
// across frames so that an unchanged node hands back the object it handed
// back last time. Objects are addressed by a handle rather than a pointer,
// so that the platform side can carry an integer in an instance variable
// and nothing has to pin a Go value for the operating system.
//
// A handle is a slot index and a generation. Slots are reused, because a
// long-lived window builds and discards elements without end, and the
// generation is what keeps a handle from an element the bridge released --
// which the assistive technology may still be holding -- from resolving to
// whatever node took the slot afterwards.
type axElems struct {
	slots []axSlot
	free  []int32
	byKey map[axKey]int64
}

type axSlot struct {
	gen  uint32
	key  axKey
	elem uintptr
	live bool
}

// axPack and axUnpack convert between a handle and the slot and generation
// inside it.
func axPack(slot int32, gen uint32) int64 { return int64(gen)<<32 | int64(uint32(slot)) }

func axUnpack(handle int64) (slot int32, gen uint32) {
	return int32(uint32(handle)), uint32(handle >> 32)
}

// intern returns the element for key, calling build to make one the first
// time. build is given the handle the element must carry, so that the
// object can be born knowing which node it is.
func (e *axElems) intern(key axKey, build func(handle int64) uintptr) uintptr {
	if h, ok := e.byKey[key]; ok {
		slot, _ := axUnpack(h)
		return e.slots[slot].elem
	}
	var slot int32
	if n := len(e.free); n > 0 {
		slot, e.free = e.free[n-1], e.free[:n-1]
		e.slots[slot].gen++
	} else {
		slot = int32(len(e.slots))
		e.slots = append(e.slots, axSlot{gen: 1})
	}
	s := &e.slots[slot]
	s.key, s.live = key, true
	h := axPack(slot, s.gen)
	s.elem = build(h)
	if e.byKey == nil {
		e.byKey = make(map[axKey]int64)
	}
	e.byKey[key] = h
	return s.elem
}

// keyOf returns the node key a handle names, or false when the element has
// been retired or the handle was never one of ours.
func (e *axElems) keyOf(handle int64) (axKey, bool) {
	slot, gen := axUnpack(handle)
	if slot < 0 || int(slot) >= len(e.slots) {
		return axKey{}, false
	}
	s := &e.slots[slot]
	if !s.live || s.gen != gen {
		return axKey{}, false
	}
	return s.key, true
}

// handleOf returns the handle held for a key, for tests and for the
// notification flush, which speaks in keys and must end in elements.
func (e *axElems) handleOf(key axKey) (int64, bool) {
	h, ok := e.byKey[key]
	return h, ok
}

// sweep retires every element whose node is not in live and returns them
// for the platform to release. Nothing is compared but liveness: an
// element answers from the published tree every time it is asked, so a node
// whose name, value or bounds changed needs no new object and no update.
func (e *axElems) sweep(live map[axKey]int32) []uintptr {
	var gone []uintptr
	for key, h := range e.byKey {
		if _, ok := live[key]; ok {
			continue
		}
		slot, _ := axUnpack(h)
		s := &e.slots[slot]
		gone = append(gone, s.elem)
		s.key, s.elem, s.live = axKey{}, 0, false
		e.free = append(e.free, slot)
		delete(e.byKey, key)
	}
	return gone
}

// axRole maps a ggui role onto the role and subrole an AppKit accessibility
// element reports. The names are the values of the NSAccessibility*Role
// constants rather than the constants themselves, which are NSStrings that
// would have to be looked up out of AppKit one at a time; they are part of
// the API and do not change. An empty subrole means the element reports
// none, which is the usual case.
//
// A few of these are not one-for-one, and the choice is the one that makes
// VoiceOver say the right thing rather than the one that reads best in a
// table. A switch is a check box with the switch subrole, because AppKit
// has no switch role. A tab is a radio button with the tab subrole, which
// is what a real NSTabView reports. A dialog is a window with the dialog
// subrole, so that VoiceOver treats it as a thing to be dismissed.
func axRole(r Role) (role, subrole string) {
	switch r {
	case RoleButton:
		return "AXButton", ""
	case RoleCheckbox:
		return "AXCheckBox", ""
	case RoleRadio:
		return "AXRadioButton", ""
	case RoleSwitch:
		return "AXCheckBox", "AXSwitch"
	case RoleSlider:
		return "AXSlider", ""
	case RoleTextField:
		return "AXTextField", ""
	case RoleSelect:
		return "AXPopUpButton", ""
	case RoleOption:
		return "AXMenuItem", ""
	case RoleMenu:
		return "AXMenu", ""
	case RoleMenuItem:
		return "AXMenuItem", ""
	case RoleTab:
		return "AXRadioButton", "AXTabButton"
	case RoleTabs:
		return "AXTabGroup", ""
	case RoleDisclosure:
		return "AXDisclosureTriangle", ""
	case RoleDialog:
		return "AXWindow", "AXDialog"
	case RoleRow, RoleListItem:
		return "AXRow", ""
	case RoleAccordion:
		return "AXGroup", "AXDisclosureTriangle"
	case RoleCombobox:
		return "AXComboBox", ""
	case RoleSeparator:
		return "AXSplitter", ""
	case RoleText:
		return "AXStaticText", ""
	case RoleHeading:
		return "AXHeading", ""
	case RoleImage:
		return "AXImage", ""
	case RoleList:
		return "AXList", ""
	case RoleProgress:
		return "AXProgressIndicator", ""
	case RoleLink:
		return "AXLink", ""
	case RoleToolbar:
		return "AXToolbar", ""
	case RoleStatus:
		return "AXGroup", ""
	case RoleWindow:
		return "AXWindow", ""
	case RoleGroup:
		return "AXGroup", ""
	}
	return "AXUnknown", ""
}

// axNumber is the number an element reports as its AXValue, for the roles
// whose value is one: a check box reports its tick as 0, 1 or 2, where 2 is
// the mixed state AppKit expects from a check box with mixed children, and
// a slider or a progress bar reports where it sits in its range. The second
// result is false for a node whose value is text, or none.
func axNumber(n Node) (float64, bool) {
	if n.Checked != TriNone {
		switch n.Checked {
		case TriOn:
			return 1, true
		case TriMixed:
			return 2, true
		}
		return 0, true
	}
	switch n.Role {
	case RoleSlider, RoleProgress:
		return n.Now, true
	}
	return 0, false
}

// axRange is the range a slider or a progress bar reports as AXMinValue and
// AXMaxValue. A node that gave neither a minimum nor a maximum reports no
// range at all, rather than a range of nothing to nothing, which VoiceOver
// would read out.
func axRange(n Node) (lo, hi float64, ok bool) {
	switch n.Role {
	case RoleSlider, RoleProgress:
		if n.Min == 0 && n.Max == 0 {
			return 0, 0, false
		}
		return n.Min, n.Max, true
	}
	return 0, 0, false
}

// axBounds is the rectangle an element reports, in the window's own
// coordinates: logical pixels, which are what AppKit calls points, with the
// y axis flipped, since ggui measures down from the top of the window and
// Cocoa measures up from the bottom. The caller turns the result into
// screen coordinates, which is the one step that needs the window.
//
// The bounds are the ones the node painted at rather than the ones it was
// clipped to, so that a row scrolled out of a list still says where it
// would be; an element that is offscreen says so separately.
func axBounds(n SemNode, viewHeight float64) (x, y, w, h float64) {
	r := n.Full
	return r.Origin.X, viewHeight - (r.Origin.Y + r.Size.H), r.Size.W, r.Size.H
}

// axHitTest returns the deepest node containing p, in the window's
// coordinates with the y axis already flipped back, or -1 for none. It
// walks the tree rather than the node array because the array is in paint
// order, and the deepest hit is wanted, not the last painted: a query comes
// from the pointer, and an assistive technology follows it into containers.
//
// An offscreen node is never hit: it is not where it says it is.
func axHitTest(t *SemTree, p Point) int {
	found := -1
	var walk func(i int)
	walk = func(i int) {
		n := t.At(i)
		if n.Offscreen || !n.Full.Contains(p) {
			return
		}
		found = i
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range t.Roots() {
		walk(r)
	}
	return found
}

// axNotice is the kind of thing that happened, which the platform turns
// into whatever notification it posts for it.
type axNotice uint8

const (
	axLayoutChanged    axNotice = iota // elements came or went
	axValueChanged                     // a node that stayed reports something else
	axSelectionChanged                 // a child of some container became the chosen one
	axFocusChanged                     // keyboard focus moved
	axAnnouncement                     // something to say out loud, from Announce
)

// axNote is one notification waiting to be posted. It names a node by key
// rather than by element, so that the whole diff can be worked out on the
// frame goroutine and only turned into platform objects at the moment it is
// delivered, on the thread that may make them.
type axNote struct {
	kind axNotice
	key  axKey  // the node it is about, when root is false
	root bool   // post on the container instead: the change is the window's
	text string // what to say, for axAnnouncement
	loud bool   // interrupt rather than wait, for axAnnouncement
}

// axDiff works out what an assistive technology must be told about the step
// from one published tree to the next. It is deliberately shallow: an
// element answers every question from the current tree whenever it is
// asked, so nothing has to be pushed except the fact that an answer would
// now be different, and only for the handful of things a screen reader acts
// on rather than re-reads.
//
// The notes come back in paint order within a kind, and layout before
// values before focus, so that VoiceOver has the new tree before it is told
// where to go in it.
func axDiff(prev, next *axFrame) []axNote {
	if next == nil {
		return nil
	}
	if prev == nil {
		return []axNote{{kind: axLayoutChanged, root: true}}
	}
	var layout bool
	var values, selections []axNote
	for i := range next.tree.Len() {
		n := next.tree.At(i)
		k := axKeyOf(n.ID)
		j, was := prev.index[k]
		if !was || next.index[k] != int32(i) {
			// Either the node is new, or it lost a key collision this
			// frame and is not an element of its own; both are the tree
			// having changed shape.
			layout = layout || !was
			continue
		}
		o := prev.tree.At(int(j))
		if axSpeaks(o.Node, n.Node) {
			values = append(values, axNote{kind: axValueChanged, key: k})
		}
		if o.Selected != n.Selected {
			selections = append(selections, axParentNote(next, n))
		}
	}
	layout = layout || len(prev.index) != len(next.index)

	var out []axNote
	if layout {
		out = append(out, axNote{kind: axLayoutChanged, root: true})
	}
	out = append(out, values...)
	out = append(out, selections...)
	if before, now := axFocusKey(prev), axFocusKey(next); before != now {
		if _, ok := next.tree.Focused(); ok {
			out = append(out, axNote{kind: axFocusChanged, key: now})
		}
	}
	return out
}

// axSpeaks reports whether what a node says about itself changed in a way a
// screen reader would repeat. The name is not among them: a node that is
// renamed is read again the next time it is reached, and announcing every
// label that a rebuild happened to rewrite would talk over the user.
func axSpeaks(before, after Node) bool {
	if before.Value != after.Value || before.Checked != after.Checked || before.Now != after.Now {
		return true
	}
	return axExpanded(before) != axExpanded(after)
}

// axExpanded folds Node.Expanded into a comparable tri-state: -1 for a node
// that does not expand at all, 0 closed, 1 open.
func axExpanded(n Node) int {
	if n.Expanded == nil {
		return -1
	}
	if *n.Expanded {
		return 1
	}
	return 0
}

// axParentNote is the selection note for a node: it goes to whatever
// contains it, since the notification says which of a container's children
// is now the chosen one, and to the container itself for a root.
func axParentNote(f *axFrame, n SemNode) axNote {
	if n.Parent < 0 {
		return axNote{kind: axSelectionChanged, root: true}
	}
	return axNote{kind: axSelectionChanged, key: axKeyOf(f.tree.At(n.Parent).ID)}
}

// axFocusKey is the key of the focused node, or the zero key when nothing
// is focused. Comparing the keys rather than the indices is what keeps a
// rebuild that shifted everything along from looking like a focus move.
func axFocusKey(f *axFrame) axKey {
	n, ok := f.tree.Focused()
	if !ok {
		return axKey{}
	}
	return axKeyOf(n.ID)
}

// axSpeak turns the queue Announce filled into notes. Politeness survives
// as loud, which the platform half maps onto whatever priority it has.
func axSpeak(notices []Announcement) []axNote {
	out := make([]axNote, 0, len(notices))
	for _, a := range notices {
		out = append(out, axNote{kind: axAnnouncement, root: true, text: a.Text, loud: a.Politeness == Assertive})
	}
	return out
}

// axAct is one of the actions a platform accessibility API asks an element
// to carry out, named here rather than by its selector so that which
// actions a node offers is decided in plain Go.
type axAct uint8

const (
	axPress axAct = iota
	axIncrement
	axDecrement
	axShowMenu
	axPick
	axConfirm
	axSetValue
	axSetFocus
	axSetSelection
)

// axActOf is the ggui action an AppKit action asks for. Confirm is a press:
// it is what Return does to the focused element, which is what Space
// already does to every ggui control. Show menu is an expansion, which is
// what opening a combobox or a select is.
func axActOf(a axAct) ActionSet {
	switch a {
	case axPress, axConfirm:
		return ActionPress
	case axIncrement:
		return ActionIncrement
	case axDecrement:
		return ActionDecrement
	case axShowMenu:
		return ActionExpand
	case axPick:
		return ActionSelect
	case axSetValue:
		return ActionSetValue
	case axSetFocus:
		return ActionFocus
	case axSetSelection:
		return ActionSetSelection
	}
	return 0
}

// axAllows reports whether a node offers an action, which is what decides
// the list an assistive technology shows the user: only what Node.Actions
// claims, and nothing at all on a disabled control except being looked at.
func axAllows(n Node, a axAct) bool {
	kind := axActOf(a)
	if kind == 0 || !n.Actions.Has(kind) {
		return false
	}
	return !n.Disabled || kind == ActionFocus
}

// A text field is the one node an assistive technology does not merely read
// out: it walks it, character by character and line by line, and moves the
// caret as it goes. macOS asks for that through a protocol of its own, in
// UTF-16 offsets, and ggui stores UTF-8 bytes, so everything below converts
// between the two. The answers come from the frozen snapshot -- Node.Value,
// Node.SelStart and SelEnd, and the Runs a text widget froze into it -- and
// never from the live widget, which belongs to another thread.

// axDetail reports whether anything is reading the tree closely enough to
// want a text node's full layout. Building it costs a measurement per
// character on every frame, which is not a price to pay while nothing is
// listening; a widget asks this before filling Node.Runs.
func axDetail() bool { return axWantsDetail.Load() }

var axWantsDetail atomic.Bool

// axUTF16Len is the length of s in UTF-16 code units, which is what every
// offset crossing into AppKit is counted in.
func axUTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// axByteAt converts a UTF-16 offset into a byte offset in s, clamped to the
// ends. An offset landing between the halves of a surrogate pair resolves
// to the start of the rune, which is where a caret can actually go.
func axByteAt(s string, u int) int {
	if u <= 0 {
		return 0
	}
	n := 0
	for i, r := range s {
		w := 1
		if r > 0xFFFF {
			w = 2
		}
		if n >= u || n+w > u {
			return i
		}
		n += w
	}
	return len(s)
}

// axUTF16At converts a byte offset in s into a UTF-16 offset.
func axUTF16At(s string, b int) int {
	n := 0
	for i, r := range s {
		if i >= b {
			return n
		}
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

// axCharCount is the length of the field, in the units AppKit counts in.
func axCharCount(n Node) int { return axUTF16Len(n.Value) }

// axSelection is the selected range, as a UTF-16 location and length. An
// empty selection is the caret, which is what it usually is.
func axSelection(n Node) (loc, length int) {
	lo := axUTF16At(n.Value, n.SelStart)
	return lo, axUTF16At(n.Value, n.SelEnd) - lo
}

// axSelected is the selected text.
func axSelected(n Node) string {
	loc, length := axSelection(n)
	return axStringForRange(n, loc, length)
}

// axByteRange converts a UTF-16 location and length into the byte range an
// Action carries, clamped to the text.
func axByteRange(n Node, loc, length int) (start, end int) {
	start = axByteAt(n.Value, loc)
	end = axByteAt(n.Value, loc+max(length, 0))
	return start, max(end, start)
}

// axStringForRange is the text a UTF-16 range covers.
func axStringForRange(n Node, loc, length int) string {
	start, end := axByteRange(n, loc, length)
	return n.Value[start:end]
}

// axLineForIndex is the line a UTF-16 offset falls on, counting from zero.
// A field that never said how it was laid out is one line, which is the
// honest answer for a field that is.
func axLineForIndex(n Node, u int) int {
	b := axByteAt(n.Value, u)
	for i, r := range n.Runs {
		if b <= r.End {
			return i
		}
	}
	return max(len(n.Runs)-1, 0)
}

// axRangeForLine is the UTF-16 range a line covers, and false for a line
// number the field does not have.
func axRangeForLine(n Node, line int) (loc, length int, ok bool) {
	if line < 0 || line >= len(n.Runs) {
		if line == 0 {
			return 0, axUTF16Len(n.Value), true
		}
		return 0, 0, false
	}
	r := n.Runs[line]
	loc = axUTF16At(n.Value, r.Start)
	return loc, axUTF16At(n.Value, r.End) - loc, true
}

// axInsertionLine is the line the caret is on, which is what VoiceOver says
// when the user moves between lines.
func axInsertionLine(n Node) int { return axLineForIndex(n, axUTF16At(n.Value, n.SelEnd)) }

// axRectForRange is the area a UTF-16 range covers, in the same logical
// coordinates as SemNode.Full. A range spanning several lines comes back as
// the part of it on the first, since the caller wants somewhere to put a
// cursor and a union of lines is not that. A range on a field that froze no
// layout falls back to the whole field.
func axRectForRange(n SemNode, loc, length int) Rect {
	start, end := axByteRange(n.Node, loc, length)
	for _, r := range n.Runs {
		if start > r.End {
			continue
		}
		a := r.At(start)
		b := r.At(min(end, r.End))
		return Rct(Pt(r.Rect.Origin.X+a, r.Rect.Origin.Y), Sz(max(b-a, 1), r.Rect.Size.H))
	}
	return n.Full
}

// axTextual reports whether a node answers the text protocol at all. A
// button has no characters, and being asked for them would be answered with
// the emptiness of a field rather than with nothing.
func axTextual(n Node) bool { return n.Role == RoleTextField }
