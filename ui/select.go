package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// SelectWidget is a dropdown that picks one of a list of values into a
// signal. Build one with Select or SelectStrings.
type SelectWidget[T comparable] struct {
	ggui.Interactive
	value    ggui.Binding[T]
	options  []T
	label    func(T) string
	minWidth float64
	onChange func(T)

	popup     *ggui.PopupWidget
	text      *ggui.TextWidget
	box       *ggui.BoxWidget // the field's fill and border
	list      *ggui.BoxWidget // the open list's panel
	items     []*selectItem[T]
	pad       ggui.EdgeInsets
	textSize  ggui.Size
	highlight int // the option the keyboard is on while open, or -1
	theme     ggui.Theme
}

// Select creates a dropdown bound to value, showing each option through
// fmt.Sprint until Label says otherwise. Space or Enter opens it, the arrow
// keys move through the options (or change the value directly while closed)
// and Escape closes it.
func Select[T comparable](value ggui.Binding[T], options []T) *SelectWidget[T] {
	s := &SelectWidget[T]{value: value, options: options, minWidth: 0, highlight: -1}
	s.Role = ggui.RoleSelect
	s.AutoKey()
	s.box = ggui.Box()
	rows := make([]ggui.Widget, len(options))
	for i := range options {
		it := &selectItem[T]{owner: s, index: i}
		it.AutoKey()
		s.items = append(s.items, it)
		rows[i] = it
	}
	s.list = ggui.Box(ggui.Column(rows...).Align(ggui.AlignStretch))
	s.popup = ggui.Popup(selectAnchor[T]{s}, s.list).Keys(s).Owner(s)
	return s.Label(sprint[T])
}

// Label sets how each option is shown.
func (s *SelectWidget[T]) Label(fn func(T) string) *SelectWidget[T] {
	s.label = fn
	for i, it := range s.items {
		it.Role, it.Name = ggui.RoleOption, fn(s.options[i])
		it.text = ggui.Text(it.Name).NoWrap()
	}
	return s
}

// Describe implements ggui.Describer: the chosen option and whether the
// list is showing. The options themselves paint through the popup's
// overlay and hang under this node, not beside the tree.
func (s *SelectWidget[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleSelect,
		Name:     s.Name,
		Value:    s.label(s.value.Peek()),
		Expanded: ggui.Expandable(s.popup.IsOpen()),
		Disabled: s.Inert,
		Actions: ggui.ActionPress | ggui.ActionFocus | ggui.ActionSelect |
			pick(s.popup.IsOpen(), ggui.ActionCollapse, ggui.ActionExpand),
	}
}

// Act implements ggui.Actor: opening and closing the list, and choosing
// the option a Select action names by value.
func (s *SelectWidget[T]) Act(a ggui.Action) bool {
	if s.Inert {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		s.highlight = s.index()
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

// Named names the dropdown for Probe.Find and the inspector.
func (s *SelectWidget[T]) Named(name string) *SelectWidget[T] { s.Name = name; return s }

// ConsumesKey implements ggui.KeyConsumer: Up and Down step or move.
func (s *SelectWidget[T]) ConsumesKey(ev ggui.KeyEvent) bool {
	return ev.Kind == ggui.KeyPress && (ggui.Activates(ev) || ev.Key == ebiten.KeyArrowUp || ev.Key == ebiten.KeyArrowDown)
}

// Disabled greys the dropdown out and ignores input while v is true.
func (s *SelectWidget[T]) Disabled(v bool) *SelectWidget[T] { s.Inert = v; return s }

// DisabledWhen follows r for Disabled without a rebuild.
func (s *SelectWidget[T]) DisabledWhen(r ggui.Reader[bool]) *SelectWidget[T] {
	s.InertWhen(r)
	return s
}

// MinWidth sets the least width the field asks for; it is otherwise as
// wide as its widest option, and fills a tight width.
func (s *SelectWidget[T]) MinWidth(w float64) *SelectWidget[T] { s.minWidth = w; return s }

// OnChange fires with the new value after the user picks one.
func (s *SelectWidget[T]) OnChange(fn func(T)) *SelectWidget[T] { s.onChange = fn; return s }

// Popup returns the popup the options open in.
func (s *SelectWidget[T]) Popup() *ggui.PopupWidget { return s.popup }

// Layout implements Widget.
func (s *SelectWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.Sync()
	t := env.Theme()
	s.theme = t
	s.pad = t.FieldPad
	s.box.Radius(t.Radius).Fill(t.Field)
	s.list.Shadow(panelShadow(t)).Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Padding(t.PanelPad)
	s.text = ggui.Text(s.label(s.value.Peek())).NoWrap().Color(pick[color.Color](s.Inert, t.Muted, t.Fg))
	// As wide as the widest option, so the field does not resize as the
	// value changes, and never wider than the row wants unless told to.
	widest := 0.0
	for _, it := range s.items {
		widest = max(widest, it.text.Layout(ggui.Loose(ggui.Sz(ggui.Unbounded, c.MaxH)), env).W)
	}
	natural := widest + s.pad.Left + s.pad.Right + controlSize + controlGap
	w := clamp(max(natural, s.minWidth), c.MinW, c.MaxW)
	s.textSize = s.text.Layout(ggui.Loose(ggui.Sz(max(w-s.pad.Left-s.pad.Right-controlSize-controlGap, 0), c.MaxH)), env)
	inner := ggui.Constraints{MinW: w, MaxW: w, MinH: c.MinH, MaxH: c.MaxH}
	return s.popup.Layout(inner, env)
}

// paintField paints the box with the current label and a chevron; it is
// what the popup's anchor draws.
func (s *SelectWidget[T]) paintField(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	open := s.popup.IsOpen()
	s.box.Border(1, pick(open || s.Focused, focusColor(t), t.Border))
	s.Hit(dst, r, s, ebiten.CursorShapePointer)
	dst.Paint(s.box, r)
	dst.Clip(r).Paint(s.text, ggui.Rct(ggui.Pt(r.Origin.X+s.pad.Left, r.Origin.Y+(r.Size.H-s.textSize.H)/2), s.textSize))
	// Chevron, pointing down, or up while open.
	cx := r.Origin.X + r.Size.W - t.Space - controlSize*0.3
	cy := r.Origin.Y + r.Size.H/2
	dy := pick(open, -2.0, 2.0)
	col := pick(s.Inert, t.Muted, t.Fg)
	dst.StrokeLine(ggui.Pt(cx-4, cy-dy), ggui.Pt(cx, cy+dy), 1.5, col)
	dst.StrokeLine(ggui.Pt(cx, cy+dy), ggui.Pt(cx+4, cy-dy), 1.5, col)
	s.FocusRing(dst, r, t.Radius, focusColor(t))
}

// Paint implements Widget.
func (s *SelectWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	for i, it := range s.items {
		it.active = i == s.highlight
	}
	dst.Paint(s.popup, r)
}

func (s *SelectWidget[T]) index() int {
	v := s.value.Peek()
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
	case ebiten.KeyEscape:
		s.popup.Hide()
	case ebiten.KeyArrowUp, ebiten.KeyArrowDown:
		if len(s.options) == 0 {
			return
		}
		dir := pick(ev.Key == ebiten.KeyArrowUp, -1, 1)
		if open {
			s.highlight = stepIndex(s.highlight, dir, len(s.options), nil)
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
}

func (it *selectItem[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	it.Sync()
	t := env.Theme()
	it.pad = t.ItemPad
	it.text.Color(t.Fg)
	it.textSize = it.text.Layout(it.pad.Shrink(c).Loosen(), env)
	return c.Constrain(it.pad.Inflate(ggui.Sz(it.textSize.W+controlSize+controlGap, it.textSize.H)))
}

// Describe implements ggui.Describer: whether this option is the value.
func (it *selectItem[T]) Describe() ggui.Node {
	return ggui.Node{
		Role:     ggui.RoleOption,
		Name:     it.Name,
		Selected: it.index == it.owner.index(),
		Disabled: it.Inert,
		Actions:  ggui.ActionSelect | ggui.ActionPress | ggui.ActionFocus,
	}
}

func (it *selectItem[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.owner.theme
	dst.Describe(r, it)
	dst.HitPointer(r, it)
	dst.HitCursor(r, ebiten.CursorShapePointer)
	if it.Hovered || it.active {
		dst.FillRoundRect(r, t.Radius*0.75, mutedSurface(t))
	}
	at := ggui.Pt(r.Origin.X+it.pad.Left+controlSize+controlGap, r.Origin.Y+(r.Size.H-it.textSize.H)/2)
	dst.Paint(it.text, ggui.Rct(at, it.textSize))
	if it.index == it.owner.index() {
		x, y := r.Origin.X+it.pad.Left, r.Origin.Y+r.Size.H/2
		dst.StrokeLine(ggui.Pt(x+2, y), ggui.Pt(x+6, y+4), 2, t.Accent)
		dst.StrokeLine(ggui.Pt(x+6, y+4), ggui.Pt(x+13, y-4), 2, t.Accent)
	}
}

// Act implements ggui.Actor: choosing this option.
func (it *selectItem[T]) Act(a ggui.Action) bool {
	if it.Inert || (a.Kind != ggui.ActionSelect && a.Kind != ggui.ActionPress) {
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
