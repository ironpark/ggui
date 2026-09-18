package ggui

import (
	"image/color"
)

// Control is a widget that takes both pointer and keyboard input.
type Control interface {
	PointerHandler
	KeyHandler
}

// Role says what kind of element a node or region is. The ui package fills
// it in; Probe.Find, the inspector and the semantics tree read it. The
// control roles name things the user acts on, the rest name content and
// structure a screen reader still has to read out.
type Role string

const (
	RoleButton     Role = "button"
	RoleCheckbox   Role = "checkbox"
	RoleRadio      Role = "radio"
	RoleSwitch     Role = "switch"
	RoleSlider     Role = "slider"
	RoleTextField  Role = "textfield"
	RoleSelect     Role = "select"
	RoleOption     Role = "option"
	RoleMenu       Role = "menu"
	RoleMenuItem   Role = "menuitem"
	RoleTab        Role = "tab"
	RoleTabs       Role = "tabs"
	RoleDisclosure Role = "disclosure"
	RoleDialog     Role = "dialog"
	RoleRow        Role = "row"
	RoleAccordion  Role = "accordion"
	RoleCombobox   Role = "combobox"
	RoleSeparator  Role = "separator"

	// Content and structure: these describe what is on screen rather than
	// what takes input, so they live in the semantics tree alone and never
	// register a hit region.
	RoleText     Role = "text"
	RoleHeading  Role = "heading"
	RoleImage    Role = "image"
	RoleList     Role = "list"
	RoleListItem Role = "listitem"
	RoleGroup    Role = "group"
	RoleProgress Role = "progress"
	RoleLink     Role = "link"
	RoleToolbar  Role = "toolbar"
	RoleStatus   Role = "status"
	RoleWindow   Role = "window"
)

// Semantic is a handler that reports a role and a label for its region;
// Interactive implements it. Probe.Find looks regions up by them.
type Semantic interface {
	Semantics() (Role, string)
}

// Interactive is the input state a control shares with every other:
// whether it takes input, hover, press, keyboard focus and whether that
// focus should be shown. Embed it, call Hit from Paint and Pointer and
// Keyboard from the handlers, and the state carries across a rebuild
// through Adopt for free. The ui package's controls are built on it.
type Interactive struct {
	Inert        bool // takes no input and draws muted
	Hovered      bool
	Pressed      bool
	Focused      bool
	FocusVisible bool   // focus arrived by keyboard: draw the ring
	Role         Role   // what the control is, for Probe.Find and the inspector
	Name         string // what it is called: the text on it, or what a Named setter gave

	id        any // from Key
	auto      any // from the keyed component it was constructed in
	inertWhen Reader[bool]
}

// SetName names the control for Probe.Find and the inspector when nothing
// on it does; ui.Field uses it to hand its label to the input inside.
func (s *Interactive) SetName(name string) { s.Name = name }

// HasName reports whether the control has an explicit name.
func (s *Interactive) HasName() bool { return s.Name != "" }

// SetInert sets disabled state and replaces any InertWhen binding.
func (s *Interactive) SetInert(v bool) {
	if s.inertWhen != nil || s.Inert != v {
		requestLayout()
	}
	s.inertWhen = nil
	s.Inert = v
}

// AutoKey takes the identity the keyed component being built gives the
// control, if any; a constructor calls it. Key overrides it.
func (s *Interactive) AutoKey() { s.auto = autoID() }

// Semantics implements Semantic.
func (s *Interactive) Semantics() (Role, string) { return s.Role, s.Name }

// ConsumesKey implements KeyConsumer: a control acts on Space and Enter.
// A control that acts on more keys overrides it.
func (s *Interactive) ConsumesKey(ev KeyEvent) bool { return Activates(ev) }

// InertWhen makes the control follow r for Inert: a control reads it in
// Layout and Paint through Sync, so nothing rebuilds when it changes.
func (s *Interactive) InertWhen(r Reader[bool]) { s.inertWhen = r; requestLayout() }

// Sync refreshes Inert from InertWhen, if set. Call it at the start of
// Layout and Paint.
func (s *Interactive) Sync() {
	if s.inertWhen != nil {
		s.Inert = s.inertWhen.Get()
	}
}

// Key gives the control an identity, so a rebuilt one that also moved
// keeps its state. Without one its Rect identifies it.
func (s *Interactive) Key(k any) { s.id = k }

// HitID implements Identified: the Key, else what AutoKey took, else nil.
func (s *Interactive) HitID() any {
	if s.id != nil {
		return s.id
	}
	return s.auto
}

// Anchor returns where to retain state for r: the control's ID when it has
// one, else r.
func (s *Interactive) Anchor(r Rect) Anchor { return Anchor{Rect: r, ID: s.HitID()} }

func (s *Interactive) state() *Interactive { return s }

// Hit registers r for pointer, keys and the cursor, unless Inert, and
// describes the control into the frame's semantics tree either way. A
// disabled control takes no input, which is why the regions are skipped,
// but it is still on screen and a screen reader must still read it out,
// which is why the node is not.
func (s *Interactive) Hit(dst *Canvas, r Rect, h Control, cursor CursorShape) {
	s.Sync()
	dst.Describe(r, h)
	if s.Inert {
		return
	}
	dst.HitPointer(r, h)
	dst.HitKey(r, h)
	dst.HitCursor(r, cursor)
}

// Pointer tracks hover and press and calls onTap on a left click. Press is
// cleared by PointerUp, not by leaving, so a drag that strays outside keeps
// going. Scroll is left to whatever is behind.
func (s *Interactive) Pointer(ev PointerEvent, onTap func()) bool {
	switch ev.Kind {
	case PointerEnter, PointerMove:
		s.Hovered = true
	case PointerExit:
		s.Hovered = false
	case PointerDown:
		s.Pressed = ev.Button == MouseButtonLeft
	case PointerUp:
		s.Pressed = false
	case PointerTap:
		if ev.Button == MouseButtonLeft && onTap != nil {
			onTap()
		}
	case PointerScroll:
		return false
	}
	return true
}

// Keyboard tracks focus and calls onActivate for Space or Enter.
func (s *Interactive) Keyboard(ev KeyEvent, onActivate func()) {
	switch ev.Kind {
	case KeyFocus:
		s.Focused, s.FocusVisible = true, ev.Key == KeyTab
	case KeyBlur:
		s.Focused, s.FocusVisible = false, false
	}
	if Activates(ev) && onActivate != nil {
		onActivate()
	}
}

// Activates reports whether ev is the key press that triggers a control:
// Space or Enter.
func Activates(ev KeyEvent) bool {
	return ev.Kind == KeyPress && (ev.Key == KeySpace || ev.Key == KeyEnter || ev.Key == KeyNumpadEnter)
}

// FocusRing outlines r in c when focus is visible.
func (s *Interactive) FocusRing(dst *Canvas, r Rect, radius float64, c color.Color) {
	if !s.Focused || !s.FocusVisible {
		return
	}
	ring := Rct(r.Origin.Add(Pt(-2, -2)), Sz(r.Size.W+4, r.Size.H+4))
	dst.StrokeRoundRect(ring, radius+2, 2, c)
}

// Adopt implements Adopter: hover, press and focus carry across a rebuild.
// Inert is the new widget's to decide.
func (s *Interactive) Adopt(prev any) {
	if p, ok := prev.(interface{ state() *Interactive }); ok {
		q := p.state()
		s.Hovered, s.Pressed, s.Focused, s.FocusVisible = q.Hovered, q.Pressed, q.Focused, q.FocusVisible
	}
}
