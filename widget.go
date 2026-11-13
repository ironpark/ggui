package ggui

import (
	"cmp"
	"image/color"
	"math"
	"slices"
	"sync"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/pkg/gem"
	"github.com/ironpark/ggui/pkg/geom"
	"github.com/ironpark/ggui/pkg/layout"
)

// Widget is the unit of composition. Layout asks it for a size under some
// Constraints; Paint then hands it a Canvas and the Rect its parent assigned:
// the origin the parent chose and the size Layout returned. Layout always runs
// before Paint in a frame, so a widget keeps only what the Rect cannot tell
// it, such as where its children go. Leaf widgets usually keep nothing at all.
//
// Value properties support runtime assignment; BindX connects a reader and X
// replaces it with a literal. Structure, identity and construction modes are
// configured before mount. Custom parents can set child literals before Layout;
// custom non-reactive data affecting size requires Invalidate on its layout Env.
type Widget interface {
	Layout(c Constraints, env Env) Size
	Paint(dst *Canvas, r Rect)
}

// Builder constructs a widget tree. App and Component setup run once;
// View, If, Key and Each rerun a builder when the value they follow changes.
// A builder runs untracked: a signal it reads itself is a snapshot, and a
// ggui_debug build reports each place that reads one. Reactive alone reruns
// on whatever its builder reads.
type Builder func() Widget

// ComponentWidget is a subtree with its own rebuild boundary. Build one with
// Component, Key or Reactive.
type ComponentWidget struct {
	child Widget
	// A rebuild boundary is a layout boundary too: the subtree is measured
	// again when this component rebuilds, when the constraints or the
	// inherited Env change, or when something inside calls Invalidate.
	// Without it every StateValue write re-measured the whole tree, since the
	// runtime lays out from the root whenever anything was written.
	cw    CachedWidget
	mount func() // runs setup at the first Layout; nil once mounted

	// For the inspector: how many times Reactive built the subtree, and in
	// a ggui_debug build where the Reactive or View was made.
	builds int
	origin string
}

// Component runs setup once when mounted. Reads in setup are untracked;
// use bindings, View, Reactive or control-flow blocks for later changes.
func Component(setup func() Widget) *ComponentWidget {
	c := &ComponentWidget{}
	owner := reactive.CurrentOwner()
	c.mount = func() {
		if owner != nil && owner.Disposed() {
			return
		}
		if owner == nil {
			owner = reactive.CurrentOwner()
		}
		reactive.WithOwner(owner, func() {
			reactive.RootWith(c, "", func() {
				c.child = remembering(setup)
				c.cw.child = c.child
			})
		})
	}
	return c
}

// Reactive gives build an effect of its own: when the signals it reads
// change, this subtree rebuilds and the parent does not. Use it to keep a
// parent's Builder static so the components it holds survive.
func Reactive[W Widget](build func() W) *ComponentWidget {
	c := &ComponentWidget{origin: reactive.Origin()}
	b := newBoundary()
	reactive.Observe(func() {
		c.builds++
		c.child = b.build(func() Widget { return build() })
		c.cw.child = c.child
		c.cw.invalidate()
	})
	return c
}

// View builds a widget from a reactive value and rebuilds it when the value
// changes, with the widget type inferred from the constructor. Only r is
// followed: build runs untracked like every other builder, so what it reads
// itself is a snapshot, and a value that changes is bound instead.
//
//	ggui.View(label, ggui.Text)
//	ggui.View(rows, func(r []Row) *ggui.ColumnWidget { return ggui.List(r, rowWidget) })
func View[T any, W Widget](r Readable[T], build func(T) W) Widget {
	return Reactive(func() Widget {
		v := r.Get()
		return reactive.Build(func() Widget { return build(v) })
	})
}

// ViewOf builds a widget from the part of r that sel picks and rebuilds it
// only when that part changes, not on every change to the rest of r:
//
//	ggui.ViewOf(doc, func(d Doc) Selection { return d.Selection }, inspector)
//
// The part is compared the way a StateValue compares its value: with its
// Equal method or ==, so pick a comparable part, or a Map with WithEqual
// for one holding slices or maps. It is View over Map, with the Map made
// once, outside the rebuilds.
func ViewOf[T, K any, W Widget](r Readable[T], sel func(T) K, build func(K) W) Widget {
	return View(Map(r, sel), build)
}

// IfWidget mounts only the selected branch and disposes it on exit.
type IfWidget struct {
	comp     *ComponentWidget
	branches []ifBranch
	other    func() Widget
	mounted  bool
}
type ifBranch struct {
	cond Readable[bool]
	then func() Widget
}

// If creates a conditional block. Finish configuring its branches before mount.
func If(cond Readable[bool], then func() Widget) *IfWidget {
	w := &IfWidget{branches: []ifBranch{{cond, then}}}
	w.comp = Component(func() Widget {
		w.mounted = true
		selected := Derived(func() int {
			for i, branch := range w.branches {
				if branch.cond.Get() {
					return i
				}
			}
			return len(w.branches)
		})
		return Key(selected, func(index int) Widget {
			if index == len(w.branches) {
				if w.other != nil {
					return w.other()
				}
				return nil
			}
			return w.branches[index].then()
		})
	})
	return w
}
func (w *IfWidget) ElseIf(cond Readable[bool], then func() Widget) *IfWidget {
	if w.mounted {
		panic("ggui: If configured after mount")
	}
	w.branches = append(w.branches, ifBranch{cond, then})
	return w
}
func (w *IfWidget) Else(other func() Widget) *IfWidget {
	if w.mounted {
		panic("ggui: If configured after mount")
	}
	w.other = other
	return w
}
func (w *IfWidget) current() Widget { return blockChild(w.comp) }
func (w *IfWidget) absent() bool    { return w.current() == nil }

// blockChild unwraps component boundaries, including an empty branch.
func blockChild(w Widget) Widget {
	for {
		c, ok := w.(*ComponentWidget)
		if !ok {
			return w
		}
		w = c.child
	}
}

// Key recreates a subtree whenever its key changes. The branch factory is
// untracked and runs once per key; it owns all computations it creates.
func Key[K comparable](key Readable[K], build func(K) Widget) *ComponentWidget {
	return Component(func() Widget {
		c := &ComponentWidget{}
		var dispose Cleanup
		owner := reactive.CurrentOwner()
		OnCleanup(func() {
			if dispose != nil {
				dispose()
			}
		})
		observeKey(key, func(value K, _ bool) {
			if dispose != nil {
				dispose()
				dispose = nil
			}
			c.child, dispose = mountKeyed(owner, func() Widget { return build(value) })
			c.cw.child = c.child
			c.cw.invalidate()
		})
		return c
	})
}

// observeKey calls changed with key's value now and again whenever it
// changes to a different one; first is set on the initial call. It shares
// Key's and Switcher's dedupe, which a re-run reading the same key skips.
func observeKey[K comparable](key Readable[K], changed func(v K, first bool)) {
	var previous K
	var initialized bool
	reactive.Observe(func() {
		v := key.Get()
		if initialized && v == previous {
			return
		}
		first := !initialized
		previous, initialized = v, true
		changed(v, first)
	})
}

// mountKeyed builds a keyed subtree under owner in a root of its own, and
// returns it with the function that disposes it.
func mountKeyed(owner *reactive.Computation, build func() Widget) (child Widget, dispose Cleanup) {
	reactive.WithOwner(owner, func() {
		// A unique root gives remounted controls fresh identities.
		identity := new(int)
		dispose = reactive.RootWith(identity, "", func() {
			child = remembering(build)
		})
	})
	return child, dispose
}

// vacant is reported by a widget that currently shows nothing, so the flow
// around it leaves out the gap it would otherwise place beside it. It is
// asked after the widget's Layout, which is when an If knows its branch.
type vacant interface{ absent() bool }

func isAbsent(w Widget) bool {
	v, ok := w.(vacant)
	return ok && v.absent()
}

// Layout implements Widget. Showing nothing, it takes no space at all.
func (w *IfWidget) Layout(c Constraints, env Env) Size {
	size := w.comp.Layout(c, env)
	if w.comp.child == nil {
		return Size{}
	}
	return size
}

// Paint implements Widget. The component paints directly, so the inspector
// lists the branch under the If rather than under an extra Component.
func (w *IfWidget) Paint(dst *Canvas, r Rect) { w.comp.Paint(dst, r) }

// Layout implements Widget.
func (c *ComponentWidget) Layout(cs Constraints, env Env) Size {
	c.cw.outer, _ = env.Get(cacheOwner)
	if c.mount != nil {
		c.mount()
		c.mount = nil
	}
	if c.child == nil {
		return cs.Constrain(Size{})
	}
	return c.cw.Layout(cs, env)
}

// Baseline implements Baseliner: the mounted child's.
func (c *ComponentWidget) Baseline() (float64, bool) { return baselineOf(c.child) }

// Baseline implements Baseliner: the shown branch's.
func (w *IfWidget) Baseline() (float64, bool) { return w.comp.Baseline() }

// Paint implements Widget.
func (c *ComponentWidget) Paint(dst *Canvas, r Rect) {
	if c.child != nil {
		dst.Paint(&c.cw, r)
	}
}

// Children builds one Widget per item. It is the bridge from data to tree for
// any widget that takes children:
//
//	Column(Children(rows, func(r Row) Widget { return rowWidget(r) })...)
func Children[T any](items []T, build func(T) Widget) []Widget {
	out := make([]Widget, len(items))
	for i, item := range items {
		out[i] = build(item)
	}
	return out
}

type widgetFunc struct {
	layout func(Constraints, Env) Size
	paint  func(*Canvas, Rect)
}

func (w widgetFunc) Layout(c Constraints, env Env) Size { return w.layout(c, env) }
func (w widgetFunc) Paint(dst *Canvas, r Rect)          { w.paint(dst, r) }

// FromFuncs builds a Widget from a layout and a paint function, for one-off
// widgets that do not deserve a named type.
func FromFuncs(layout func(Constraints, Env) Size, paint func(*Canvas, Rect)) Widget {
	return widgetFunc{layout: layout, paint: paint}
}

func (c *ComponentWidget) absent() bool { return blockChild(c) == nil }

// Baseliner is a widget that knows where its first line of text sits: the
// distance from the top of the Rect it was laid out for to that line's
// baseline. Row.Align(AlignBaseline) lines children up on it, so a small
// caption beside a large title shares its baseline instead of its centre.
//
// The answer is valid after Layout, for the size Layout returned. Text and
// TextInput report their face's ascent; Box, Column, Align and the other
// containers report their first child's baseline moved by where that child
// went. A widget with no text in it reports false, and a Row then rests it
// on the baseline by its bottom edge, as CSS does with an image in a line
// of text.
type Baseliner interface {
	Baseline() (float64, bool)
}

// baselineOf asks w for its baseline, or reports false when w cannot say.
func baselineOf(w Widget) (float64, bool) {
	if b, ok := w.(Baseliner); ok {
		return b.Baseline()
	}
	return 0, false
}

// baselineAt is baselineOf for a child painted dy below its parent's top.
func baselineAt(w Widget, dy float64) (float64, bool) {
	b, ok := baselineOf(w)
	return b + dy, ok
}

// Geometry and constraints are owned by the renderer-independent layout
// package. These aliases keep ggui's public geometry API unchanged.
type (
	// Number is any built-in numeric type. The geometry constructors take
	// one so that pixel counts from ggfx (int) and layout math
	// (float64) can be mixed without a cast at every call site.
	Number = layout.Number

	// Point is a position in logical (device-independent) pixels.
	Point = layout.Point

	// Size is a width/height pair in logical pixels.
	Size = layout.Size

	// Rect is an axis-aligned rectangle anchored at Origin. Paint receives
	// one: the origin its parent chose and the size its own Layout returned.
	Rect = layout.Rect

	// Affine is a 2D affine transform of logical points. The zero value
	// is the identity, and each method returns the transform followed by
	// one more step: Affine{}.Scale(2, 2).Translate(10, 0) scales a point
	// and then moves it.
	Affine = geom.Affine
)

// Pt returns the Point at (x, y), whatever numeric type they are.
func Pt[T Number](x, y T) Point { return layout.Pt(x, y) }

// Sz returns the Size w by h, whatever numeric type they are.
func Sz[T Number](w, h T) Size { return layout.Sz(w, h) }

// Rct returns the Rect at origin with size.
func Rct(origin Point, size Size) Rect { return layout.Rct(origin, size) }

// Unbounded is the maximum a Scroll gives its child along the scroll axis:
// take whatever you need. Widgets that would fill the space they are given
// fall back to their content size on an unbounded axis.
var Unbounded = math.Inf(1)

// bounded returns v, or fallback when v is Unbounded.
func bounded(v, fallback float64) float64 { return gem.Bounded(v, fallback) }

// Constraints bound the size a widget may choose during layout.
type Constraints = layout.Constraints

// Tight returns constraints that permit exactly one size.
func Tight(s Size) Constraints { return layout.Tight(s) }

// Loose returns constraints that permit anything up to s.
func Loose(s Size) Constraints { return layout.Loose(s) }

// clamp keeps v within [lo, hi]; lo wins when the bounds contradict.
func clamp[T cmp.Ordered](v, lo, hi T) T { return gem.Clamp(v, lo, hi) }

// Role says what kind of element a node or region is. The ui package fills
// it in; Probe.Find, the inspector and the semantics tree read it. The
// control roles name things the user acts on, the rest name content and
// structure a screen reader still has to read out.
//
// It is defined in the a11y package, so that a bridge can read a role
// without importing this one.
type Role = a11y.Role

// The control roles: things the user acts on.
const (
	RoleButton     = a11y.RoleButton
	RoleCheckbox   = a11y.RoleCheckbox
	RoleRadio      = a11y.RoleRadio
	RoleSwitch     = a11y.RoleSwitch
	RoleSlider     = a11y.RoleSlider
	RoleTextField  = a11y.RoleTextField
	RoleSelect     = a11y.RoleSelect
	RoleOption     = a11y.RoleOption
	RoleMenu       = a11y.RoleMenu
	RoleMenuItem   = a11y.RoleMenuItem
	RoleTab        = a11y.RoleTab
	RoleTabs       = a11y.RoleTabs
	RoleDisclosure = a11y.RoleDisclosure
	RoleDialog     = a11y.RoleDialog
	RoleRow        = a11y.RoleRow
	RoleAccordion  = a11y.RoleAccordion
	RoleCombobox   = a11y.RoleCombobox
	RoleSeparator  = a11y.RoleSeparator
	RoleSpinButton = a11y.RoleSpinButton
	RoleTreeItem   = a11y.RoleTreeItem
)

// Content and structure: these describe what is on screen rather than what
// takes input, so they live in the semantics tree alone and never register
// a hit region.
const (
	RoleText     = a11y.RoleText
	RoleHeading  = a11y.RoleHeading
	RoleImage    = a11y.RoleImage
	RoleList     = a11y.RoleList
	RoleListItem = a11y.RoleListItem
	RoleGroup    = a11y.RoleGroup
	RoleProgress = a11y.RoleProgress
	RoleLink     = a11y.RoleLink
	RoleToolbar  = a11y.RoleToolbar
	RoleStatus   = a11y.RoleStatus
	RoleWindow   = a11y.RoleWindow
	RoleTable    = a11y.RoleTable
	RoleTree     = a11y.RoleTree
	RoleCell     = a11y.RoleCell
	RoleHeader   = a11y.RoleHeader
)

// Control is a widget that takes both pointer and keyboard input.
type Control interface {
	PointerHandler
	KeyHandler
}

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
	Hovered      bool
	Pressed      bool
	Focused      bool
	FocusVisible bool
	Role         Role
	id           any
	auto         any
	properties   property.Owner
	name         property.Value[string]
	inert        property.Value[bool]
}

// SetName sets the accessible name and removes a name binding.
func (s *Interactive) SetName(name string) {
	if s.name.Set(name) {
		s.properties.Changed()
	}
}

// SemanticName returns the resolved explicit name without a control's fallback.
func (s *Interactive) SemanticName() string { s.properties.Read(); return s.name.Get() }

// HasName reports whether a name was set or bound, including an empty binding.
func (s *Interactive) HasName() bool { return s.name.Bound() || s.name.Current() != "" }

// BindName borrows a non-nil name reader until SetName or another BindName.
func (s *Interactive) BindName(r Readable[string]) {
	if s.name.Bind(r, "BindName") {
		s.properties.Changed()
	}
}

// SetInert changes input availability and removes an inert binding.
func (s *Interactive) SetInert(v bool) {
	if s.inert.Set(v) {
		s.properties.Changed()
	}
}

// IsInert reports whether the control currently ignores input.
func (s *Interactive) IsInert() bool { s.properties.Read(); return s.inert.Get() }

// AutoKey takes the identity the keyed component being built gives the
// control, if any; a constructor calls it. SetKey overrides it.
func (s *Interactive) AutoKey() { s.auto = reactive.AutoID() }

// Semantics implements Semantic.
func (s *Interactive) Semantics() (Role, string) {
	return s.Role, s.SemanticName(

	// ConsumesKey implements KeyConsumer: a control acts on Space and Enter.
	// A control that acts on more keys overrides it.
	)
}

func (s *Interactive) ConsumesKey(ev KeyEvent) bool { return Activates(ev) }

// BindInert borrows a non-nil reader for input availability.
func (s *Interactive) BindInert(r Readable[bool]) {
	if s.inert.Bind(r, "BindInert") {
		s.properties.Changed()
	}
}

// Sync resolves input properties in the current tracking scope. Layout and
// Paint call it before consuming the control's state.
func (s *Interactive) Sync() { s.properties.Read(); s.name.Get(); s.inert.Get() }

// SetKey gives the control an identity, so a rebuilt one that also moved
// keeps its state. Without one its Rect identifies it.
func (s *Interactive) SetKey(k any) { s.id = k }

// HitID implements Identified: what SetKey set, else what AutoKey took, else nil.
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
// which is why the node is not. cursor is what the pointer shows over the
// control whatever it is painted over: one asking for CursorShapeDefault
// inside a text field shows the arrow there, not the field's beam.
func (s *Interactive) Hit(dst *Canvas, r Rect, h Control, cursor CursorShape) {
	s.HitReach(dst, r, Rect{}, h, cursor)
}

// HitReach is Hit with the pointer landing in reach as well as in r: a
// small control is easier to hit than to see, as a checkbox with a margin
// around its box is. r stays the control's Rect in every other way, the
// one a probe finds, a reveal scrolls to and a rebuilt control is matched
// by, and the two are one region: the pointer crossing from one into the
// other neither leaves nor enters. An empty reach adds nothing.
func (s *Interactive) HitReach(dst *Canvas, r, reach Rect, h Control, cursor CursorShape) {
	s.Sync()
	dst.Describe(r, h)
	if s.IsInert() {
		return
	}
	// One region with all three, as the three registrations would merge
	// into: the identity, semantics and handoff are looked up once.
	dst.add(dst.region(r, h, hitRegion{pointer: h, key: h, cursor: cursor, arrow: cursor == CursorShapeDefault, reach: reach}))
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

// CachedWidget skips laying its subtree out again while nothing in it has
// changed. Build one with Cached. App already skips the whole layout on a
// still frame. Component, Reactive and Key cache their own subtrees, so a
// signal write in one panel does not
// re-measure every other panel. Cached is for a static subtree that sits
// under something which does rebuild.
//
// The cache holds while the constraints and everything inherited through
// the Env (text style, Scroll viewport, Provide values) are the same
// as last time and nothing inside asked for a layout. Reactive, EachKeyed, Scroll
// and TextInput ask when they change; a custom widget whose size depends on
// state outside a signal calls Invalidate with the Env it was laid out
// under.
type CachedWidget struct {
	child Widget
	// sources are what the last measurement read itself, each at the
	// version it read last, and kids the caches it laid out, each at the
	// measurement it took: a source read inside a nested cache is that
	// cache's alone, so a source is kept once, not by every cache above it.
	sources []sourceRead
	kids    []kidCache

	outer   *CachedWidget // the nearest Cached above, told when this one is
	dirty   bool
	valid   bool
	cons    Constraints
	fontGen uint64
	rev     uint64 // the Env revision the size was measured under
	size    Size
	gen     uint64 // counts the measurements
	// measuring is set while the cache measures its child, and again that
	// a Layout inside that measurement called Invalidate; see stale.
	measuring bool
	again     bool
}

// mostLateKids is how many caches laid out after a cache measured, as a
// viewport lays out what scrolls into it, it keeps as its own before it
// measures afresh to let go of those gone.
const mostLateKids = 512

// sourceRead is a source a measurement read, at the version it read.
type sourceRead struct {
	src     reactive.LayoutSource
	version uint64
}

// kidCache is a cache laid out within another's measurement, at the
// measurement of its own it gave.
type kidCache struct {
	c   *CachedWidget
	gen uint64
}

var cacheOwner = NewEnvKey[*CachedWidget]("layoutCache")

// Cached wraps child in a layout cache.
func Cached(child Widget) *CachedWidget { return &CachedWidget{child: child} }

// invalidate marks this cache and every one above it stale.
func (c *CachedWidget) invalidate() {
	// The runtime lays out only when something moved; a stale cache is
	// such a thing even when no StateValue was written.
	reactive.RequestLayout()
	for ; c != nil; c = c.outer {
		c.dirty = true
	}
}

// stale marks this cache and every one above it stale, as invalidate does,
// for the next layout, asking for none itself. A cache measuring now, as
// one is above a Layout that calls Invalidate, takes the mark when the
// measurement ends: marked now, the end would clear it as done with, and
// the layout asked for would find every cache above the widget as it was
// and lay none of it out again.
func (c *CachedWidget) stale() {
	for ; c != nil; c = c.outer {
		if c.measuring {
			c.again = true
		} else {
			c.dirty = true
		}
	}
}

// Layout implements Widget.
func (c *CachedWidget) Layout(cs Constraints, env Env) Size {
	c.outer, _ = env.Get(cacheOwner)
	gen := fontGen()
	if c.valid && !c.dirty && cs == c.cons && env.rev == c.rev && c.fontGen == gen && c.inputsEqual() {
		c.tellOuter()
		return c.size
	}
	clear(c.kids)
	c.sources, c.kids = c.sources[:0], c.kids[:0]
	c.measuring = true
	reactive.Measure(c.record, func() { c.size = c.child.Layout(cs, env.With(cacheOwner, c)) })
	c.measuring = false
	c.sources = dedupeReads(c.sources)
	c.kids = slices.Clip(c.kids)
	c.gen++
	c.fontGen = gen
	c.cons, c.rev, c.valid, c.dirty, c.again = cs, env.rev, true, c.again, false
	c.tellOuter()
	return c.size
}

// tellOuter tells the cache above that this one's size, at its current
// measurement, is part of its own: laid out within it, or after it, as a
// viewport lays out what scrolls into it.
func (c *CachedWidget) tellOuter() {
	o := c.outer
	if o == nil {
		return
	}
	for i := range o.kids {
		if o.kids[i].c == c {
			o.kids[i].gen = c.gen
			return
		}
	}
	if !o.measuring && len(o.kids) >= mostLateKids {
		// Most of those are likely gone by now.
		o.invalidate()
		return
	}
	o.kids = append(o.kids, kidCache{c, c.gen})
}

// Paint implements Widget.
func (c *CachedWidget) Paint(dst *Canvas, r Rect) { dst.Paint(c.child, r) }

// Baseline implements Baseliner: the child's.
func (c *CachedWidget) Baseline() (float64, bool) { return baselineOf(c.child) }

// record notes src as read by this cache's measurement. Layout installs it
// as the reactive core's recorder for the duration of a measurement, saving
// and restoring the previous one so that nested caches nest correctly.
func (c *CachedWidget) record(src reactive.LayoutSource, version uint64) {
	// A widget reports a source at the start of its layout and again at
	// its end: the second replaces the first.
	if n := len(c.sources); n > 0 && c.sources[n-1].src == src {
		c.sources[n-1].version = version
		return
	}
	c.sources = append(c.sources, sourceRead{src, version})
}

// inputsEqual reports whether every source this cache's measurement read,
// itself or through the caches inside, is at the version it read, and those
// caches stand as they were.
func (c *CachedWidget) inputsEqual() bool {
	for _, r := range c.sources {
		if r.src.LayoutVersion() != r.version {
			return false
		}
	}
	for _, k := range c.kids {
		if k.c.gen != k.gen || k.c.dirty || !k.c.valid || !k.c.inputsEqual() {
			return false
		}
	}
	return true
}

// mostScannedReads is how many reads dedupeReads compares pairwise before
// it takes a map to.
const mostScannedReads = 32

// readsSeen are dedupeReads' maps, reused: each window lays out on a
// goroutine of its own.
var readsSeen = sync.Pool{New: func() any { return map[reactive.LayoutSource]int{} }}

// dedupeReads keeps each source once, at the version it was read last, in
// the order it was first read, in a slice no larger than it needs.
func dedupeReads(reads []sourceRead) []sourceRead {
	n := 0
	if len(reads) <= mostScannedReads {
	scan:
		for _, r := range reads {
			for j := range n {
				if reads[j].src == r.src {
					reads[j].version = r.version
					continue scan
				}
			}
			reads[n] = r
			n++
		}
	} else {
		readSeen := readsSeen.Get().(map[reactive.LayoutSource]int)
		defer readsSeen.Put(readSeen)
		for _, r := range reads {
			if j, ok := readSeen[r.src]; ok {
				reads[j].version = r.version
				continue
			}
			readSeen[r.src] = n
			reads[n] = r
			n++
		}
		clear(readSeen)
	}
	clear(reads[n:])
	reads = reads[:n]
	if cap(reads) > 2*n+4 {
		return slices.Clone(reads)
	}
	return reads
}

// Invalidate tells the runtime that the widget laid out under env changed
// size, or its children did: the tree is laid out again next frame and the
// nearest Cached above the widget measures its subtree afresh. A StateValue
// write does the first half by itself; a widget that keeps size-affecting
// state outside signals calls Invalidate when that state changes.
//
// Under a layout boundary, a Scroll whose viewport keeps its size whatever
// its content's, a Contain or a pane of ui.Resizable, the change shows
// within that boundary alone, and a frame nothing else changed for is
// painted there and nowhere else: a disclosure easing open in a scrolling
// pane or a fixed panel does not paint the window again at every step. A
// boundary that turns out to follow its content's size is painted whole
// once and passes the next change to the one around it; see
// layoutBoundary.
//
// It may be called from the widget's own Layout, for a size that is still
// on its way somewhere, as Motion.LayoutTo does: the layout asked for is
// the next frame's, and runs then, not again in this one.
func Invalidate(env Env) {
	c, _ := env.Get(cacheOwner)
	if b := boundaryOf(env).holding(); b != nil {
		reactive.Contained(reactive.RequestLayout)
		c.stale()
		b.hold()
		return
	}
	reactive.RequestLayout()
	c.stale()
}
