package ui

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ironpark/ggui"
)

// SelectWidget is a dropdown that picks one of a list of values into a
// signal. Build one with Select or SelectStrings.
type SelectWidget[T comparable] struct {
	value    *ggui.Signal[T]
	options  []T
	label    func(T) string
	disabled bool
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
	hovered   bool
	focus     focusState
	theme     ggui.Theme
}

// Select creates a dropdown bound to value, showing each option through
// label. Space or Enter opens it, the arrow keys move through the options
// (or change the value directly while closed) and Escape closes it.
func Select[T comparable](value *ggui.Signal[T], options []T, label func(T) string) *SelectWidget[T] {
	s := &SelectWidget[T]{value: value, options: options, label: label, minWidth: defaultStripe, highlight: -1}
	s.box = ggui.Box()
	rows := make([]ggui.Widget, len(options))
	for i := range options {
		it := &selectItem[T]{owner: s, index: i, text: ggui.Text(label(options[i])).NoWrap()}
		s.items = append(s.items, it)
		rows[i] = it
	}
	s.list = ggui.Box(ggui.Column(rows...).Align(ggui.AlignStretch))
	s.popup = ggui.Popup(selectAnchor[T]{s}, s.list).Keys(s)
	return s
}

// SelectStrings is Select for plain strings, labelled as they are.
func SelectStrings(value *ggui.Signal[string], options ...string) *SelectWidget[string] {
	return Select(value, options, func(s string) string { return s })
}

// Disabled greys the dropdown out and ignores input while v is true.
func (s *SelectWidget[T]) Disabled(v bool) *SelectWidget[T] { s.disabled = v; return s }

// MinWidth sets the width the field asks for when its parent leaves the
// width to it; it fills a bounded width.
func (s *SelectWidget[T]) MinWidth(w float64) *SelectWidget[T] { s.minWidth = w; return s }

// OnChange fires with the new value after the user picks one.
func (s *SelectWidget[T]) OnChange(fn func(T)) *SelectWidget[T] { s.onChange = fn; return s }

// Popup returns the popup the options open in.
func (s *SelectWidget[T]) Popup() *ggui.PopupWidget { return s.popup }

// Layout implements Widget.
func (s *SelectWidget[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	s.theme = t
	s.pad = ggui.Insets(t.Space*0.75, t.Space)
	s.box.Radius(t.Radius).Fill(t.Field)
	s.list.Fill(t.Surface).Border(1, t.Border).Radius(t.Radius).Pad(t.Space * 0.5)
	s.text = ggui.Text(s.label(s.value.Peek())).NoWrap().Color(pick[color.Color](s.disabled, t.Muted, t.Fg))
	w := max(c.MinW, bounded(c.MaxW, s.minWidth))
	s.textSize = s.text.Layout(ggui.Loose(ggui.Sz(max(w-s.pad.Left-s.pad.Right-controlSize-controlGap, 0), c.MaxH)), env)
	inner := ggui.Constraints{MinW: w, MaxW: w, MinH: c.MinH, MaxH: c.MaxH}
	return s.popup.Layout(inner, env)
}

// paintField paints the box with the current label and a chevron; it is
// what the popup's anchor draws.
func (s *SelectWidget[T]) paintField(dst *ggui.Canvas, r ggui.Rect) {
	t := s.theme
	open := s.popup.IsOpen()
	s.box.Border(1, pick(open || s.focus.focused, t.Accent, t.Border))
	if !s.disabled {
		dst.HitPointer(r, s)
		dst.HitKey(r, s)
		dst.HitCursor(r, ebiten.CursorShapePointer)
	}
	dst.Paint(s.box, r)
	dst.Clip(r).Paint(s.text, ggui.Rct(ggui.Pt(r.Origin.X+s.pad.Left, r.Origin.Y+(r.Size.H-s.textSize.H)/2), s.textSize))
	// Chevron, pointing down, or up while open.
	cx := r.Origin.X + r.Size.W - t.Space - controlSize*0.3
	cy := r.Origin.Y + r.Size.H/2
	dy := pick(open, -2.0, 2.0)
	col := pick(s.disabled, t.Muted, t.Fg)
	dst.StrokeLine(ggui.Pt(cx-4, cy-dy), ggui.Pt(cx, cy+dy), 1.5, col)
	dst.StrokeLine(ggui.Pt(cx, cy+dy), ggui.Pt(cx+4, cy-dy), 1.5, col)
	s.focus.paintRing(dst, r, t.Radius, t)
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
	s.value.Set(s.options[i])
	if s.onChange != nil {
		s.onChange(s.options[i])
	}
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
	s.focus.handle(ev)
	if ev.Kind == ggui.KeyBlur {
		s.popup.Hide()
	}
	if ev.Kind != ggui.KeyPress {
		return
	}
	open := s.popup.IsOpen()
	switch ev.Key {
	case ebiten.KeySpace, ebiten.KeyEnter, ebiten.KeyNumpadEnter:
		if open && s.highlight >= 0 {
			s.choose(s.highlight)
		} else {
			s.toggle()
		}
	case ebiten.KeyEscape:
		s.popup.Hide()
	case ebiten.KeyArrowUp, ebiten.KeyArrowDown:
		if len(s.options) == 0 {
			return
		}
		dir := pick(ev.Key == ebiten.KeyArrowUp, -1, 1)
		if open {
			s.highlight = (max(s.highlight, 0) + dir + len(s.options)) % len(s.options)
			if s.highlight < 0 {
				s.highlight = 0
			}
		} else {
			s.choose((max(s.index(), 0) + dir + len(s.options)) % len(s.options))
		}
	}
}

// Adopt implements ggui.Adopter: an open list and hover carry across a
// rebuild.
func (s *SelectWidget[T]) Adopt(prev any) {
	if p, ok := prev.(*SelectWidget[T]); ok {
		s.hovered, s.focus, s.highlight = p.hovered, p.focus, p.highlight
		if p.popup.IsOpen() {
			s.popup.Show()
		}
	}
}

// HandlePointer implements PointerHandler.
func (s *SelectWidget[T]) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		s.hovered = true
	case ggui.PointerExit:
		s.hovered = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft {
			s.toggle()
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}

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
	owner   *SelectWidget[T]
	index   int
	text    *ggui.TextWidget
	hovered bool
	active  bool // highlighted from the keyboard

	pad      ggui.EdgeInsets
	textSize ggui.Size
}

func (it *selectItem[T]) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := env.Theme()
	it.pad = ggui.Insets(t.Space*0.5, t.Space)
	it.text.Color(t.Fg)
	it.textSize = it.text.Layout(it.pad.Shrink(c).Loosen(), env)
	return c.Constrain(it.pad.Inflate(ggui.Sz(it.textSize.W+controlSize+controlGap, it.textSize.H)))
}

func (it *selectItem[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := it.owner.theme
	dst.HitPointer(r, it)
	dst.HitCursor(r, ebiten.CursorShapePointer)
	if it.hovered || it.active {
		dst.FillRoundRect(r, t.Radius*0.75, t.Selection)
	}
	at := ggui.Pt(r.Origin.X+it.pad.Left+controlSize+controlGap, r.Origin.Y+(r.Size.H-it.textSize.H)/2)
	dst.Paint(it.text, ggui.Rct(at, it.textSize))
	if it.index == it.owner.index() {
		x, y := r.Origin.X+it.pad.Left, r.Origin.Y+r.Size.H/2
		dst.StrokeLine(ggui.Pt(x+2, y), ggui.Pt(x+6, y+4), 2, t.Accent)
		dst.StrokeLine(ggui.Pt(x+6, y+4), ggui.Pt(x+13, y-4), 2, t.Accent)
	}
}

func (it *selectItem[T]) Adopt(prev any) {
	if p, ok := prev.(*selectItem[T]); ok {
		it.hovered = p.hovered
	}
}

func (it *selectItem[T]) HandlePointer(ev ggui.PointerEvent) bool {
	switch ev.Kind {
	case ggui.PointerEnter, ggui.PointerMove:
		it.hovered = true
		it.owner.highlight = it.index
	case ggui.PointerExit:
		it.hovered = false
	case ggui.PointerTap:
		if ev.Button == ebiten.MouseButtonLeft {
			it.owner.choose(it.index)
		}
	case ggui.PointerScroll:
		return false
	}
	return true
}
