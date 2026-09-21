package ui

import (
	"image/color"
	"slices"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/icons"
	"github.com/ironpark/ggui/internal/property"
)

// SelectWidget is a dropdown that picks one of a list of values into a
// signal. Build one with Select.
type SelectWidget[T comparable] struct {
	props property.Owner
	ggui.Interactive
	value      ggui.Binding[T]
	options    []T
	optionProp property.Value[[]T]
	label      func(T) string
	minWidth   float64
	onChange   func(T)

	popup     *ggui.PopupWidget
	text      *ggui.TextWidget
	box       *ggui.BoxWidget // the field's fill and border
	viewport  *selectViewport[T]
	list      *ggui.BoxWidget // the open list's panel
	items     []*selectItem[T]
	pad       ggui.EdgeInsets
	textSize  ggui.Size
	highlight int // the option the keyboard is on while open, or -1
	theme     ggui.Theme
	env       ggui.Env
}

// Select creates a dropdown bound to value, showing each option through
// fmt.Sprint until Format says otherwise. Space or Enter opens it, the arrow
// keys move through the options (or change the value directly while closed)
// and Escape closes it. The options slice is shallow-copied.
func Select[T comparable](value ggui.Binding[T]) *SelectWidget[T] {
	s := &SelectWidget[T]{value: value, label: sprint[T], highlight: -1}
	s.Role = ggui.RoleSelect
	s.AutoKey()
	s.box = ggui.Box()
	column := ggui.Column().Align(ggui.AlignStretch)
	s.viewport = &selectViewport[T]{owner: s, column: column, offset: ggui.State(0.0)}
	s.viewport.scroll = ggui.Scroll(column).BindOffset(s.viewport.offset)
	s.list = ggui.Box(s.viewport)
	s.popup = ggui.Popup(selectAnchor[T]{s}, s.list).Keys(s).Owner(s)
	return s
}

// Options replaces the options with a shallow copy, including after mount,
// and removes any BindOptions binding. It leaves the value and open state
// unchanged and does not call OnChange. If the value is absent it remains
// displayed, with no highlighted option until keyboard or pointer input.
func (s *SelectWidget[T]) Options(options []T) *SelectWidget[T] {
	changed := s.setOptions(options)
	detached := s.optionProp.Set(s.options)
	if changed || detached {
		s.props.Changed()
	}
	return s
}

// BindOptions follows r at layout without rebuilding the control. Changes
// use the same snapshot and selection rules as Options. The last setting
// wins; r must be non-nil. Use Options to detach the source.
func (s *SelectWidget[T]) BindOptions(r ggui.Readable[[]T]) *SelectWidget[T] {
	if s.optionProp.Bind(r, "BindOptions") {
		s.props.Changed()
	}
	return s
}

func (s *SelectWidget[T]) setOptions(options []T) bool {
	if slices.Equal(s.options, options) {
		return false
	}
	s.options = slices.Clone(options)
	s.items = make([]*selectItem[T], len(options))
	rows := make([]ggui.Widget, len(options))
	for i, value := range s.options {
		name := s.label(value)
		it := &selectItem[T]{owner: s, index: i, text: ggui.Text(name).NoWrap()}
		it.Role = ggui.RoleOption
		it.SetName(name)
		// Replaced rows must not adopt a removed row's pointer state by position.
		it.SetKey(it)
		s.items[i], rows[i] = it, it
	}
	s.viewport.column = ggui.Column(rows...).Align(ggui.AlignStretch)
	s.viewport.scroll = ggui.Scroll(s.viewport.column).BindOffset(s.viewport.offset)
	s.highlight = s.index()
	s.viewport.reveal = true
	return true
}

// Format sets how each option is shown.
func (s *SelectWidget[T]) Format(fn func(T) string) *SelectWidget[T] {
	s.label = fn
	for i, it := range s.items {
		it.Role = ggui.RoleOption
		it.SetName(fn(s.options[i]))
		it.text = ggui.Text(it.SemanticName()).NoWrap()
	}
	return s
}

// Describe implements ggui.Describer: the chosen option and whether the
// list is showing. The options themselves paint through the popup's
// overlay and hang under this node, not beside the tree.
func (s *SelectWidget[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSelect,
		Name:     s.SemanticName(),
		Value:    s.label(ggui.Untrack(s.value.Get)),
		Expanded: ggui.Expandable(s.popup.IsOpen()),
		Disabled: s.IsInert(),
		Actions: ggui.ActionPress | ggui.ActionFocus | ggui.ActionSelect |
			pick(s.popup.IsOpen(), ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: opening and closing the list, and choosing
// the option a Select action names by value.
func (s *SelectWidget[T]) Act(a ggui.Action) bool {
	if s.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		s.highlight = s.index()
		s.viewport.reveal = true
		s.popup.Show()
	case ggui.ActionCollapse:
		s.popup.Hide()
	case ggui.ActionSetValue, ggui.ActionSelect:
		for i, o := range s.options {
			if s.label(o) == a.Text {
				s.choose(i)
				return true
			}
		}
		return false
	default:
		return false
	}
	return true
}

// Name names the dropdown for Probe.Find and the inspector.
func (s *SelectWidget[T]) Name(name string) *SelectWidget[T] { s.SetName(name); return s }

// ConsumesKey implements ggui.KeyConsumer: Up and Down step or move.
func (s *SelectWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && (ggui.Activates(ev) || ev.Key == ggui.KeyArrowUp || ev.Key == ggui.KeyArrowDown)
}

// Disabled greys the dropdown out and ignores input while v is true.
func (s *SelectWidget[T]) Disabled(v bool) *SelectWidget[T] { s.SetInert(v); return s }

// BindDisabled follows r for Disabled without a rebuild.
func (s *SelectWidget[T]) BindDisabled(r ggui.Readable[bool]) *SelectWidget[T] {
	s.BindInert(r)
	return s
}

// MinWidth sets the least width the field asks for; it is otherwise as
// wide as its widest option, and fills a tight width.
func (s *SelectWidget[T]) MinWidth(w float64) *SelectWidget[T] {
	defer property.Watch(&s.props, &s.minWidth)()
	s.minWidth = w
	return s
}

// OnChange fires with the new value after the user picks one.
func (s *SelectWidget[T]) OnChange(fn func(T)) *SelectWidget[T] { s.onChange = fn; return s }

// Popup returns the popup the options open in.
func (s *SelectWidget[T]) Popup() *ggui.PopupWidget { return s.popup }

// Layout implements Widget.
func (s *SelectWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer s.props.Layout()()
	s.setOptions(s.optionProp.Get())
	s.Sync()
	if s.IsInert() {
		s.popup.Hide()
	}
	t := env.Theme()
	s.theme = t
	s.env = env
	s.pad = t.FieldPad
	s.box.Radius(t.Radius).Fill(t.Input)
	panelBox(s.list, t)
	s.text = ggui.Text(s.label(ggui.Untrack(s.value.Get))).NoWrap().Color(pick[color.Color](s.IsInert(), t.MutedFg, t.Fg))
	// As wide as the widest option, so the field does not resize as the
	// value changes, and never wider than the row wants unless told to.
	widest := 0.0
	for _, it := range s.items {
		widest = max(widest, it.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env).W)
	}
	natural := widest + s.pad.Left + s.pad.Right + t.ControlSize + t.ControlGap
	w := clamp(max(natural, s.minWidth), c.MinW, c.MaxW)
	s.list.Width(w) // the popup follows its trigger, not the window width
	s.textSize = s.text.Layout(ggui.Loose(ggui.Sz(max(w-s.pad.Left-s.pad.Right-t.ControlSize-t.ControlGap, 0), c.MaxH)), env)
	inner := ggui.Constraints{MinW: w, MaxW: w, MinH: c.MinH, MaxH: c.MaxH}
	return s.popup.Layout(inner, env)
}

// paintField paints the box with the current label and a chevron; it is
// what the popup's anchor draws.
func (s *SelectWidget[T]) paintField(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	open := s.popup.IsOpen()
	s.box.Border(t.BorderWidth, pick(open || s.Focused, t.Ring, colorOr(t.InputBorder, t.Border)))
	s.Hit(dst, r, s, ggui.CursorShapePointer)
	dst.Paint(s.box, r)
	dst.Clip(r).Paint(s.text, ggui.Rct(ggui.Pt(r.Origin.X+s.pad.Left, r.Origin.Y+(r.Size.H-s.textSize.H)/2), s.textSize))
	// Chevron, pointing down, or up while open.
	center := ggui.Pt(r.Origin.X+r.Size.W-t.Space-t.ControlSize*0.3, r.Origin.Y+r.Size.H/2)
	chevron(dst, s.env, center, pick(open, -2.0, 2.0), pick(s.IsInert(), t.MutedFg, t.Fg))
	s.FocusRing(dst, r, t.Radius, t.Ring)
}

// Paint implements Widget.
func (s *SelectWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i, it := range s.items {
		it.active = i == s.highlight
	}
	dst.Paint(s.popup, r)
}

func (s *SelectWidget[T]) index() int {
	v := ggui.Untrack(s.value.Get)
	for i, o := range s.options {
		if o == v {
			return i
		}
	}
	return -1
}

func (s *SelectWidget[T]) choose(i int) {
	if i < 0 || i >= len(s.options) {
		return
	}
	setChanged(s.value, s.options[i], s.onChange)
	s.popup.Hide()
}

func (s *SelectWidget[T]) toggle() {
	if s.popup.IsOpen() {
		s.popup.Hide()
		return
	}
	s.highlight = s.index()
	s.viewport.reveal = true
	s.popup.Show()
}

// HandleKey implements KeyHandler.
func (s *SelectWidget[T]) HandleKey(ev ggui.KeyEvent) {
	s.Keyboard(ev, nil)
	if ev.Kind == ggui.KeyBlur {
		s.popup.Hide()
	}
	if ev.Kind != ggui.KeyPress {
		return
	}
	open := s.popup.IsOpen()
	if ggui.Activates(ev) {
		if open && s.highlight >= 0 {
			s.choose(s.highlight)
		} else {
			s.toggle()
		}
		return
	}
	switch ev.Key {
	case ggui.KeyEscape:
		s.popup.Hide()
	case ggui.KeyArrowUp, ggui.KeyArrowDown:
		if len(s.options) == 0 {
			return
		}
		dir := pick(ev.Key == ggui.KeyArrowUp, -1, 1)
		if open {
			s.highlight = stepIndex(s.highlight, dir, len(s.options), nil)
			s.viewport.reveal = true
		} else {
			s.choose(stepIndex(s.index(), dir, len(s.options), nil))
		}
	}
}

// Adopt implements ggui.Adopter: an open list carries across a rebuild.
func (s *SelectWidget[T]) Adopt(prev any) {
	s.Interactive.Adopt(prev)
	if p, ok := prev.(*SelectWidget[T]); ok {
		s.highlight = p.highlight
		s.viewport.offset.Set(ggui.Untrack(p.viewport.offset.Get))
		s.viewport.reveal = p.viewport.reveal
		if p.popup.IsOpen() {
			s.popup.Show()
		}
	}
}

// HandlePointer implements PointerHandler.
func (s *SelectWidget[T]) HandlePointer(ev ggui.PointerEvent) bool { return s.Pointer(ev, s.toggle) }

// selectAnchor is the popup's anchor: the field, painted by its Select.
type selectAnchor[T comparable] struct{ s *SelectWidget[T] }

func (a selectAnchor[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s := a.s
	s.box.Size(c.MaxW, s.pad.Inflate(s.textSize).H)
	return s.box.Layout(c, env)
}

func (a selectAnchor[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { a.s.paintField(dst, r) }

// selectItem is one option row in the open list.
type selectItem[T comparable] struct {
	ggui.Interactive
	owner  *SelectWidget[T]
	index  int
	text   *ggui.TextWidget
	active bool // highlighted from the keyboard

	pad      ggui.EdgeInsets
	textSize ggui.Size
	height   float64
}

func (it *selectItem[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	it.Sync()
	t := env.Theme()
	it.pad = t.ItemPad
	it.text.Color(colorOr(t.PopoverFg, t.Fg))
	inner := it.pad.Shrink(c).Loosen()
	inner.MaxW = max(0, inner.MaxW-t.ControlSize-t.ControlGap)
	it.textSize = it.text.Layout(inner, env)
	size := c.Constrain(it.pad.Inflate(ggui.Sz(it.textSize.W+t.ControlSize+t.ControlGap, it.textSize.H)))
	it.height = size.H
	return size
}

// Describe implements ggui.Describer: whether this option is the value.
func (it *selectItem[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleOption,
		Name:     it.SemanticName(),
		Selected: it.index == it.owner.index(),
		Disabled: it.IsInert(),
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

func (it *selectItem[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.owner.theme
	dst.Describe(r, it)
	dst.HitPointer(r, it)
	dst.HitCursor(r, ggui.CursorShapePointer)
	if it.Hovered || it.active {
		dst.FillRoundRect(r, t.RadiusSm, colorOr(t.Accent, t.Muted))
	}
	at := ggui.Pt(r.Origin.X+it.pad.Left+t.ControlSize+t.ControlGap, r.Origin.Y+(r.Size.H-it.textSize.H)/2)
	dst.Clip(r).Paint(it.text, ggui.Rct(at, it.textSize))
	if it.index == it.owner.index() {
		x, y := r.Origin.X+it.pad.Left, r.Origin.Y+r.Size.H/2
		paintIcon(dst, it.owner.env, icons.Check, ggui.Rct(ggui.Pt(x, y-8), ggui.Sz(16, 16)), t.Primary, 0)
	}
}

// Act implements ggui.Actor: choosing this option.
func (it *selectItem[T]) Act(a ggui.Action) bool {
	if it.IsInert() || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
		return false
	}
	it.owner.choose(it.index)
	return true
}

func (it *selectItem[T]) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerEnter || ev.Kind == ggui.PointerMove {
		it.owner.highlight = it.index
	}
	return it.Pointer(ev, func() { it.owner.choose(it.index) })
}

// selectViewport measures the full list without changing its scroll position.
// Popup first measures with unlimited height to choose above/below placement;
// only the final painted viewport may clamp the scroll offset.
type selectViewport[T comparable] struct {
	owner  *SelectWidget[T]
	column *ggui.ColumnWidget
	scroll *ggui.ScrollWidget
	offset *ggui.StateValue[float64]
	reveal bool
	env    ggui.Env
}

func (v *selectViewport[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	v.env = env
	natural := v.column.Layout(ggui.Constraints{MinW: c.MinW, MaxW: c.MaxW, MaxH: ggui.Unbounded}, env)
	return c.Constrain(natural)
}

func (v *selectViewport[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if v.reveal {
		v.reveal = false
		top := 0.0
		for i, item := range v.owner.items {
			if i == v.owner.highlight {
				offset := ggui.Untrack(v.offset.Get)
				if top < offset {
					v.offset.Set(top)
				} else if top+item.height > offset+r.Size.H {
					v.offset.Set(max(0, top+item.height-r.Size.H))
				}
				break
			}
			top += item.height
		}
	}
	v.scroll.Layout(ggui.Constraints{MinW: r.Size.W, MaxW: r.Size.W, MinH: r.Size.H, MaxH: r.Size.H}, v.env)
	dst.Paint(v.scroll, r)
}
