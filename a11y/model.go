// Package a11y is ggui's accessibility layer: the vocabulary a widget
// describes itself in, the frozen tree one frame publishes, and the bridges
// that hand that tree to the platform's own accessibility API.
//
// Most programs never import it. The ggui package re-exports every name
// here -- Node, Role, Action and the rest are the same types under the same
// names there -- and sets the whole thing going from Config.Accessibility.
// Import it to read a field list, or to work on a bridge.
//
// Point, Rect and Size are re-exported from geom for the same reason, so
// that a bridge writes them under one name throughout.
//
// It is a package of its own because the bridges must read everything a
// widget published, and a bridge cannot import the package that runs it.
// Defining the vocabulary here, below both, is what keeps one definition of
// Node rather than a copy that silently falls behind.
//
// # Platforms
//
// macOS goes through NSAccessibility and Windows through UI Automation,
// both without cgo. Everywhere else the bridge is absent and costs nothing.
// A new platform is a new file in this package. The seam it fills is the
// axPlatform interface in a11y.go, which is unexported: the bridge and its
// platform halves are one unit, so a platform is added here rather than
// implemented from outside.
package a11y

import (
	"reflect"

	"github.com/ironpark/ggui/geom"
)

// The geometry a node is measured in.
type (
	Point = geom.Point
	Rect  = geom.Rect
	Size  = geom.Size
)

// Pt returns the Point at (x, y). A generic function cannot be aliased, so
// the three geometry constructors are one-line forwards rather than names.
func Pt[T geom.Number](x, y T) Point { return geom.Pt(x, y) }

// Sz returns the Size w by h.
func Sz[T geom.Number](w, h T) Size { return geom.Sz(w, h) }

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return geom.Rct(origin, size) }

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

// Tristate is a checkbox's state: on, off, or the mixed state a parent
// checkbox shows when only some of its children are ticked. The zero value
// says the node has no checked state at all, which is what every node that
// is not a checkbox, radio or switch reports.
type Tristate uint8

const (
	TriNone  Tristate = iota // not checkable
	TriOff                   // checkable and not checked
	TriOn                    // checked
	TriMixed                 // partially checked
)

// Tri returns TriOn for true and TriOff for false, for a control whose
// checked state is a plain bool.
func Tri(v bool) Tristate {
	if v {
		return TriOn
	}
	return TriOff
}

// Expandable returns a pointer to v, for Node.Expanded: a nil Expanded
// means the node does not expand at all, which is not the same as being
// expandable and closed.
//
// go fix offers to inline this into new(v) at every call site. Decline it:
// the name is the point. A bare new(open) beside Role and Name says nothing
// about the three states Expanded has, and this is the only place the
// difference between "closed" and "does not expand" is written down.
func Expandable(v bool) *bool { return &v }

// ActionSet is the set of actions a node accepts, as a bitset. A node
// advertises what it supports so an assistive technology can offer it; see
// Actor for the side that performs them.
type ActionSet uint32

const (
	ActionPress          ActionSet = 1 << iota // activate: a button, a menu item, a checkbox
	ActionIncrement                            // raise a slider or stepper by one step
	ActionDecrement                            // lower it by one step
	ActionSetValue                             // replace the value outright: a text field, a slider
	ActionExpand                               // open a disclosure, accordion or combobox
	ActionCollapse                             // close one
	ActionSelect                               // make this the chosen tab, option or row
	ActionFocus                                // move keyboard focus here
	ActionScrollIntoView                       // bring the node into view
	ActionSetSelection                         // move the caret or the selection in a text field
)

// Has reports whether every action in b is in a.
func (a ActionSet) Has(b ActionSet) bool { return a&b == b }

// Node describes one element of the accessibility tree: what it is, what it
// is called, and what may be done to it. A widget fills in only the fields
// its role gives meaning to; the zero value of every other field says "not
// applicable", which is why Expanded is a pointer and Checked has a TriNone.
// Pointers and slices are borrowed until the frame finishes; the published
// SemTree copies them, so a widget may reuse their storage next frame.
type Node struct {
	Role        Role
	Name        string   // what the element is called, read first
	Description string   // the longer explanation, read after the name
	Value       string   // a text field's contents, a select's current option
	Checked     Tristate // checkbox, radio, switch
	Expanded    *bool    // accordion, collapsible, combobox; nil is not expandable
	Selected    bool     // tab, option, row
	Disabled    bool     // from Interactive.Inert: present, but takes no input
	Min         float64  // slider, progress: the bottom of the range
	Max         float64  // the top of the range, or a list's item count
	Now         float64  // where the value sits in it, or an item's place in a list
	Actions     ActionSet
	Offscreen   bool // clipped out of view, but present with its true bounds

	// A text field carries where its caret and selection are, in bytes
	// into Value, and -- while something is reading the tree closely
	// enough to want it -- how Value was laid out on screen. A platform
	// text API asks for characters, lines and the rectangle a range covers
	// from a thread that must not touch the live widget, so the answers
	// are frozen into the snapshot with everything else.
	SelStart, SelEnd int
	Runs             []TextRun
}

// TextRun is one painted line of a text node: which bytes of Node.Value it
// covers, where it went, and where every character boundary inside it
// landed. Rect is in the same coordinates as SemNode.Full, and a stop's X
// is measured from Rect.Origin.X.
type TextRun struct {
	Start, End int
	Rect       Rect
	Stops      []TextStop
}

// TextStop is one character boundary within a run: a byte offset into
// Node.Value and the horizontal position it sits at. There is one for every
// rune boundary in the run, including both ends, so a caret between any two
// characters has a position.
type TextStop struct {
	Byte int
	X    float64
}

// At returns the horizontal position of byte offset b within the run,
// clamped to its ends. A byte in the middle of a rune takes that rune's
// starting position, which is where a caret would be drawn.
func (r TextRun) At(b int) float64 {
	if len(r.Stops) == 0 {
		return 0
	}
	last := r.Stops[0]
	for _, s := range r.Stops {
		if s.Byte > b {
			break
		}
		last = s
	}
	return last.X
}

// Key returns h as a map key, or nil when h cannot be one: a handler
// holding a slice or a map would panic a lookup.
func Key(h any) any {
	if h == nil {
		return nil
	}
	if t := reflect.TypeOf(h); t == nil || !t.Comparable() {
		return nil
	}
	return h
}

// Control reports whether the role is one the user acts on, as opposed to
// content or grouping. Text inside one belongs to it.
func Control(r Role) bool {
	switch r {
	case RoleButton, RoleCheckbox, RoleRadio, RoleSwitch, RoleSlider, RoleTextField,
		RoleSelect, RoleOption, RoleMenu, RoleMenuItem, RoleTab, RoleDisclosure,
		RoleCombobox, RoleLink:
		return true
	}
	return false
}

// Action is one request aimed at a node.
type Action struct {
	Kind ActionSet // exactly one of the action bits
	Text string    // the new contents, for ActionSetValue on a text field
	Num  float64   // the new number, for ActionSetValue on a slider

	// The byte range to select, for ActionSetSelection. An empty range is
	// a caret. Bytes, not characters: the platform counts in whatever its
	// own text API uses and the bridge converts, so that a widget never
	// has to know what UTF-16 is.
	SelStart, SelEnd int
}

// Politeness says how urgent an announcement is.
type Politeness int

const (
	Polite    Politeness = iota // said when the user is not in the middle of something
	Assertive                   // said at once, interrupting
)

// Announcement is one thing to say out loud, queued by Announce.
type Announcement struct {
	Text       string
	Politeness Politeness
}
