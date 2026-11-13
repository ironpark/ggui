package ui

import (
	"math"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
)

// sheetSide says which edge a sheet slides in from.
type sheetSide int

const (
	sheetRight sheetSide = iota // the default
	sheetLeft
	sheetTop
	sheetBottom // what Drawer uses
)

// SheetWidget is a modal panel anchored to an edge of the window: a scrim
// dims everything else and takes the clicks, Tab stays inside, Escape or a
// click on the scrim closes it, and focus returns to where it was. Build one
// with Sheet, or with Drawer for the bottom edge. It takes no space where it
// sits in the tree; put it anywhere.
//
// The sheet slides in and out over the theme's MotionSlow. The motion lives
// in the widget, so a sheet kept across rebuilds -- in a Component, or
// simply constructed once beside the signal it binds -- animates, and one
// rebuilt from scratch every frame appears and goes at once, as Dialog does.
type SheetWidget struct {
	nameReader ggui.Readable[string]
	props      property.Owner
	modal
	side   sheetSide
	extent float64
	handle bool
	slide  ggui.Motion
	effect *ggui.TransitionWidget

	draggable                                            bool
	snapPoints                                           []float64
	snapIndex                                            int
	snapBinding                                          ggui.Binding[int]
	onSnapChange                                         func(int)
	grip                                                 *drawerHandle
	snapMotion                                           ggui.Motion
	dragging                                             bool
	dragStart, dragExtent, visibleExtent, viewportHeight float64
}

// Sheet creates a panel that shows content along the window's right edge
// while open is true.
//
//	filters := ggui.State(false)
//	ui.Sheet(filters, filterForm).Title("Filters").Left()
func Sheet(open ggui.Binding[bool], content ggui.Widget) *SheetWidget {
	s := &SheetWidget{}
	s.open, s.content, s.dismissible = open, content, true
	return s
}

// Drawer is a Sheet that rises from the bottom edge, with the grab handle
// that says so. Drag the handle down to dismiss, or configure SnapPoints.
// Content keeps its own pointer and scrolling behavior.
func Drawer(open ggui.Binding[bool], content ggui.Widget) *SheetWidget {
	s := Sheet(open, content).Bottom()
	s.extent, s.handle, s.draggable = 280, true, true
	return s
}

// Title puts a heading above the content, which also names the sheet for
// Probe.Find.
func (s *SheetWidget) Title(v string) *SheetWidget { s.setTitle(v); return s }

// Name sets an accessible name without adding a visible heading.
func (s *SheetWidget) Name(name string) *SheetWidget {
	if s.nameReader != nil {
		s.nameReader = nil
		s.props.Changed()
	}
	defer property.Watch(&s.props, &s.name)()
	s.name = name
	return s
}

// Compact removes the outer padding for content with its own spacing.
func (s *SheetWidget) Compact() *SheetWidget {
	defer property.Watch(&s.props, &s.compact)()
	s.compact = true
	return s
}

// Dismissible says whether a click on the scrim closes the sheet; it does
// by default. Escape closes either way.
func (s *SheetWidget) Dismissible(v bool) *SheetWidget {
	defer property.Watch(&s.props, &s.dismissible)()
	s.dismissible = v
	return s
}

// Left anchors the sheet to the left edge.
func (s *SheetWidget) Left() *SheetWidget {
	defer property.Watch(&s.props, &s.side)()
	s.side = sheetLeft
	return s
}

// Right anchors the sheet to the right edge, which is where it starts.
func (s *SheetWidget) Right() *SheetWidget {
	defer property.Watch(&s.props, &s.side)()
	s.side = sheetRight
	return s
}

// Top anchors the sheet to the top edge.
func (s *SheetWidget) Top() *SheetWidget {
	defer property.Watch(&s.props, &s.side)()
	s.side = sheetTop
	return s
}

// Bottom anchors the sheet to the bottom edge.
func (s *SheetWidget) Bottom() *SheetWidget {
	defer property.Watch(&s.props, &s.side)()
	s.side = sheetBottom
	return s
}

// Size sets how far the sheet reaches from its edge: its width on the left
// or right, its height on the top or bottom. It shrinks to fit a smaller
// window.
func (s *SheetWidget) Size(v float64) *SheetWidget {
	defer property.Watch(&s.props, &s.extent)()
	s.extent = max(0, v)
	return s
}

// OnClose fires when the sheet closes, however it closed.
func (s *SheetWidget) OnClose(fn func()) *SheetWidget { s.onClose = fn; return s }

// horizontal reports whether the sheet slides along the x axis.
func (s *SheetWidget) horizontal() bool { return s.side == sheetLeft || s.side == sheetRight }

// Layout implements ggui.Widget: the sheet takes no room in the tree. The
// panel gets no border or radius: it is flush with the edge it came from.
func (s *SheetWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout().EndLayout()
	if s.nameReader != nil {
		s.name = s.nameReader.Get()
	}
	s.build(env)
	if s.snapBinding != nil {
		s.snapBinding.Get()
	}
	if !ggui.Untrack(s.open.Get) || !s.draggable {
		s.dragging = false
	}
	if s.grip == nil {
		s.grip = &drawerHandle{owner: s}
		s.grip.Role = ggui.RoleSlider
		s.grip.SetKey(s.grip)
	}
	s.grip.SetName(pick(s.name == "", "Drawer size", s.name+" size"))
	if s.effect == nil {
		s.effect = ggui.Transition(ggui.FromFuncs(func(c ggui.Constraints, e ggui.Env) ggui.Size { return s.panel.Layout(c, e) }, s.paintDrawerSurface)).Fade().Easing(ggui.EaseLinear)
	}
	return c.Constrain(ggui.Size{})
}

// Paint implements ggui.Widget: the panel paints through Canvas.Overlay
// while it is open, and while it is still on its way out.
func (s *SheetWidget) Paint(dst *ggui.Canvas, _ ggui.Rect) {
	open := ggui.Untrack(s.open.Get)
	now := ggui.FrameTime()
	if !open {
		s.dragging = false
	}
	v := clamp(s.slide.Easing(ggui.EaseInOut).Toggle(open, now, s.env.Motion(s.theme.MotionSlow)), 0, 1)
	if !open && v <= 0.001 {
		s.rect = ggui.Rect{}
		s.snapMotion = ggui.Motion{}
		return
	}
	dst.Overlay(func(dst *ggui.Canvas) { s.paintPanel(dst, v, open) })
}

func (s *SheetWidget) paintPanel(dst *ggui.Canvas, v float64, open bool) {
	t := s.theme
	screen := dst.Size()
	if screen == (ggui.Size{}) {
		screen = ggui.Sz(s.extent, s.extent)
	}
	var cs ggui.Constraints
	if s.horizontal() {
		w := min(384, screen.W*.75)
		if s.extent > 0 {
			w = min(s.extent, screen.W)
		}
		cs = ggui.Constraints{MinW: w, MaxW: w, MinH: screen.H, MaxH: screen.H}
	} else {
		h := min(s.extent, screen.H)
		if s.isDrawer() && len(s.snapPoints) > 0 {
			h = min(s.snapPoints[len(s.snapPoints)-1], screen.H)
		}
		if s.extent <= 0 && !(s.isDrawer() && len(s.snapPoints) > 0) {
			h = s.panel.Layout(ggui.Constraints{MinW: screen.W, MaxW: screen.W, MaxH: screen.H}, s.env).H
		}
		cs = ggui.Constraints{MinW: screen.W, MaxW: screen.W, MinH: h, MaxH: h}
	}
	size := s.effect.Layout(cs, s.env)
	s.viewportHeight = screen.H
	visible := size.H
	if s.isDrawer() {
		target := size.H
		if len(s.snapPoints) > 0 {
			target = min(s.snapPoints[s.selectedSnap()], size.H)
		}
		if s.dragging {
			target = clamp(s.dragExtent, 0, size.H)
		}
		if !open {
			target = s.visibleExtent
		}
		duration := s.env.Motion(t.MotionSlow)
		if s.dragging {
			duration = 0
		}
		s.snapMotion.MoveTo(target, ggui.FrameTime(), duration)
		visible = s.snapMotion.Value(ggui.FrameTime())
		s.visibleExtent = visible
	}
	distance := 40.0
	if s.handle {
		distance = size.H
	}
	var at ggui.Point
	switch s.side {
	case sheetLeft:
		at = ggui.Pt(-(1-v)*distance, 0)
	case sheetTop:
		at = ggui.Pt(0, -(1-v)*distance)
	case sheetBottom:
		at = ggui.Pt(0, screen.H-size.H+(1-v)*distance)
		if s.isDrawer() {
			at.Y = screen.H - visible*v
		}
	default:
		at = ggui.Pt(screen.W-size.W+(1-v)*distance, 0)
	}

	// A sheet on its way out is a picture: it must not take the click that
	// lands where it used to be, nor hold focus it is about to give back.
	if !open {
		dst = dst.Inert()
	}
	rect := ggui.Rct(at, size)
	s.effect.Progress(v, !open)
	scrimProgress := v
	if s.isDrawer() && size.H > 0 {
		scrimProgress *= clamp(visible/size.H, 0, 1)
	}
	s.paintContent(dst, screen, rect, fade(t.Scrim, scrimProgress), s, s.effect)
}

// BindName follows a non-nil accessible-name reader.
func (s *SheetWidget) BindName(r ggui.Readable[string]) *SheetWidget {
	property.Require(r, "BindName")
	if !property.Same(s.nameReader, r) {
		s.nameReader = r
		s.props.Changed()
	}
	return s
}

// CloseButton controls the close action, visible by default.
func (s *SheetWidget) CloseButton(show bool) *SheetWidget {
	defer property.Watch(&s.props, &s.hideClose)()
	s.hideClose = !show
	return s
}

// Draggable enables dragging the grab handle on a bottom Drawer. It is true
// for Drawer by default; Sheet and its existing edge APIs are unaffected.
func (s *SheetWidget) Draggable(v bool) *SheetWidget {
	defer property.Watch(&s.props, &s.draggable)()
	s.draggable = v
	if !v {
		s.dragging = false
	}
	return s
}

// SnapPoints sets increasing visible heights in logical pixels for a bottom
// Drawer. Points must be positive, finite and strictly increasing; heights are
// clamped to the viewport. The largest point determines the content's height,
// and smaller points reveal its upper portion. Empty points restore Size.
// The initial point is index zero. Dragging below half the smallest point
// dismisses the drawer if Dismissible is true; otherwise it returns to a point.
func (s *SheetWidget) SnapPoints(points ...float64) *SheetWidget {
	for i, p := range points {
		if math.IsNaN(p) || math.IsInf(p, 0) || p <= 0 || (i > 0 && p <= points[i-1]) {
			panic("ui: Drawer snap points must be finite, positive and increasing")
		}
	}
	if !slices.Equal(s.snapPoints, points) {
		s.snapPoints = slices.Clone(points)
		s.props.Changed()
	}
	return s
}

// SnapPoint selects a zero-based snap index and detaches any controlled binding.
// Out-of-range indices are clamped when used.
func (s *SheetWidget) SnapPoint(index int) *SheetWidget {
	if s.snapBinding != nil {
		s.snapBinding = nil
		s.props.Changed()
	}
	defer property.Watch(&s.props, &s.snapIndex)()
	s.snapIndex = index
	return s
}

// BindSnapPoint makes the selected index controlled. Pointer release and
// keyboard/accessibility edits request an index through this binding; a refused
// request settles back to the getter's index. A drag never writes intermediate
// indices. External binding changes animate to the selected point.
func (s *SheetWidget) BindSnapPoint(index ggui.Binding[int]) *SheetWidget {
	property.Require(index, "BindSnapPoint")
	if !property.Same(s.snapBinding, index) {
		s.snapBinding = index
		s.props.Changed()
	}
	return s
}

// OnSnapChange reports a requested snap index after a drag or keyboard action.
func (s *SheetWidget) OnSnapChange(fn func(int)) *SheetWidget { s.onSnapChange = fn; return s }

func (s *SheetWidget) isDrawer() bool { return s.handle && s.side == sheetBottom }
func (s *SheetWidget) selectedSnap() int {
	i := s.snapIndex
	if s.snapBinding != nil {
		i = ggui.Untrack(s.snapBinding.Get)
	}
	return max(0, min(i, len(s.snapPoints)-1))
}
func (s *SheetWidget) selectSnap(index int) {
	index = max(0, min(index, len(s.snapPoints)-1))
	if index == s.selectedSnap() {
		return
	}
	if s.snapBinding != nil {
		s.snapBinding.Set(index)
	} else {
		s.snapIndex = index
	}
	ggui.Invalidate(s.env)
	if s.onSnapChange != nil {
		s.onSnapChange(index)
	}
}

func (s *SheetWidget) paintDrawerSurface(dst *ggui.Canvas, r ggui.Rect) {
	s.paintSurface(dst, r)
	if !s.handle {
		return
	}
	grip := ggui.Sz(36, 4)
	at := ggui.Pt(r.Origin.X+(r.Size.W-grip.W)/2, r.Origin.Y+s.theme.Space/2)
	dst.FillRoundRect(ggui.Rct(at, grip), grip.H/2, s.theme.Border)
	if s.isDrawer() && s.draggable && s.grip != nil {
		hit := ggui.Rct(ggui.Pt(r.Origin.X+max(0, (r.Size.W-72)/2), r.Origin.Y), ggui.Sz(min(72, r.Size.W), min(24, r.Size.H)))
		s.grip.Hit(dst, hit, s.grip, ggui.CursorShapePointer)
		s.grip.FocusRing(dst, hit, s.theme.Radius, s.theme.Ring)
	}
}

type drawerHandle struct {
	ggui.Interactive
	owner *SheetWidget
}

func (h *drawerHandle) CaptureTouchDrag() bool { return h.owner.draggable }
func (h *drawerHandle) Describe() ggui.Node {
	s := h.owner
	maxIndex := max(0, len(s.snapPoints)-1)
	return ggui.Node{Role: ggui.RoleSlider, Name: h.SemanticName(), Min: 0, Max: float64(maxIndex), Now: float64(s.selectedSnap()), Actions: ggui.ActionFocus | ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue}
}
func (h *drawerHandle) ConsumesKey(ev ggui.KeyEvent) bool { return sliderConsumesKey(ev) }
func (h *drawerHandle) HandleKey(ev ggui.KeyEvent) {
	h.Keyboard(ev, nil)
	s := h.owner
	if !s.draggable || !ggui.Untrack(s.open.Get) || ev.Kind != ggui.KeyPress {
		return
	}
	i := s.selectedSnap()
	switch ev.Key {
	case ggui.KeyHome:
		i = 0
	case ggui.KeyEnd:
		i = len(s.snapPoints) - 1
	case ggui.KeyArrowUp, ggui.KeyArrowRight, ggui.KeyPageUp:
		i++
	case ggui.KeyArrowDown, ggui.KeyArrowLeft, ggui.KeyPageDown:
		i--
	default:
		return
	}
	s.selectSnap(i)
}
func (h *drawerHandle) Act(a ggui.Action) bool {
	s := h.owner
	if !s.draggable || !ggui.Untrack(s.open.Get) {
		return false
	}
	i := s.selectedSnap()
	switch a.Kind {
	case ggui.ActionIncrement:
		i++
	case ggui.ActionDecrement:
		i--
	case ggui.ActionSetValue:
		if math.IsNaN(a.Num) || math.IsInf(a.Num, 0) {
			return false
		}
		i = int(clamp(a.Num, 0, float64(max(0, len(s.snapPoints)-1))))
	default:
		return false
	}
	s.selectSnap(i)
	return true
}
func (h *drawerHandle) HandlePointer(ev ggui.PointerEvent) bool {
	s := h.owner
	if !s.draggable || !ggui.Untrack(s.open.Get) {
		s.dragging = false
		h.Pressed = false
		return false
	}
	switch ev.Kind {
	case ggui.PointerDown:
		if ev.Button != ggui.MouseButtonLeft {
			return false
		}
		s.dragging = true
		s.dragStart = ev.Pos.Y
		s.dragExtent = s.visibleExtent
	case ggui.PointerDrag:
		if s.dragging {
			s.dragExtent = clamp(s.visibleExtent+s.dragStart-ev.Pos.Y, 0, s.rect.Size.H)
			s.dragStart = ev.Pos.Y
		}
	case ggui.PointerUp:
		if s.dragging {
			s.dragExtent = clamp(s.visibleExtent+s.dragStart-ev.Pos.Y, 0, s.rect.Size.H)
			s.dragging = false
			// Seed the settling animation at the release position, including a
			// release that came without an intervening drag frame.
			s.snapMotion.MoveTo(s.dragExtent, ggui.FrameTime(), 0)
			s.visibleExtent = s.dragExtent
			lowest := min(s.rect.Size.H, s.viewportHeight)
			if len(s.snapPoints) > 0 {
				lowest = min(s.snapPoints[0], s.viewportHeight)
			}
			if s.dismissible && s.dragExtent < lowest*.5 {
				s.Close()
			} else if len(s.snapPoints) > 0 {
				best := s.selectedSnap()
				for i, p := range s.snapPoints {
					if math.Abs(min(p, s.viewportHeight)-s.dragExtent) < math.Abs(min(s.snapPoints[best], s.viewportHeight)-s.dragExtent) {
						best = i
					}
				}
				s.selectSnap(best)
			}
		}
	}
	return h.Pointer(ev, nil)
}
