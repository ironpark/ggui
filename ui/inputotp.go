package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// InputOTPPart describes a group of slots or a visual separator. Parts are
// presentation configuration; the whole OTP remains a single native editor.
type InputOTPPart interface{ otpPart() }
type InputOTPSlotSpec struct{ Index int }

func InputOTPSlot(index int) InputOTPSlotSpec { return InputOTPSlotSpec{index} }

type InputOTPGroupSpec struct{ slots []InputOTPSlotSpec }

func (InputOTPGroupSpec) otpPart() {}
func InputOTPGroup(slots ...InputOTPSlotSpec) InputOTPGroupSpec {
	return InputOTPGroupSpec{append([]InputOTPSlotSpec(nil), slots...)}
}

type InputOTPSeparatorSpec struct{}

func (InputOTPSeparatorSpec) otpPart()         {}
func InputOTPSeparator() InputOTPSeparatorSpec { return InputOTPSeparatorSpec{} }

type otpCell struct {
	index int
	rect  ggui.Rect
}
type otpGroup struct{ rect ggui.Rect }

// InputOTPWidget presents one editor as individually focused character slots.
// It retains native IME, clipboard, undo, selection and accessibility editing.
type InputOTPWidget struct {
	props property.Owner
	ggui.Interactive
	input                *ggui.TextInputWidget
	maxLength            int
	parts                []InputOTPPart
	accept               func(rune) bool
	invalid              bool
	invalidWhen          ggui.Readable[bool]
	disabled             bool
	rtl                  bool
	slotSize, gap        float64
	placeholder          []rune
	onChange, onComplete func(string)
	theme                uitheme.Theme
	env                  ggui.Env
	cells                []otpCell
	groups               []otpGroup
	separators           []ggui.Rect
	labels               []*ggui.TextWidget
	sizes                []ggui.Size
	lastText             string
	rect                 ggui.Rect
	blink                time.Time
	reduced              bool
	dragAnchor           int
}

// InputOTP creates a digits-only code field. Without explicit parts it draws
// one joined group. Values are limited to maxLength Unicode code points.
// Filtering removes disallowed characters, including separators in pasted codes.
func InputOTP(value ggui.Binding[string], maxLength int, parts ...InputOTPPart) *InputOTPWidget {
	o := &InputOTPWidget{maxLength: max(1, maxLength), slotSize: 32, gap: 8, parts: append([]InputOTPPart(nil), parts...), lastText: "\x00"}
	o.Role = ggui.RoleTextField
	o.AutoKey()
	o.accept = func(r rune) bool { return r >= '0' && r <= '9' }
	o.input = ggui.TextInput(value).Filter(o.normalize).OnChange(func(s string) {
		o.blink = ggui.Now()
		if o.onChange != nil {
			o.onChange(s)
		}
		if utf8.RuneCountInString(s) == o.maxLength && o.onComplete != nil {
			o.onComplete(s)
		}
	})
	if len(o.parts) == 0 {
		o.Groups(o.maxLength)
	}
	return o
}
func (o *InputOTPWidget) normalize(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if o.accept == nil || o.accept(r) {
			b.WriteRune(r)
			n++
			if n == o.maxLength {
				break
			}
		}
	}
	return b.String()
}

// Groups replaces composition with joined groups separated by a minus sign.
// Counts must add to MaxLength; invalid compositions panic during Layout.
func (o *InputOTPWidget) Groups(counts ...int) *InputOTPWidget {
	defer property.Watch(&o.props, &o.parts)()
	o.parts = nil
	index := 0
	for i, n := range counts {
		if i > 0 {
			o.parts = append(o.parts, InputOTPSeparator())
		}
		slots := make([]InputOTPSlotSpec, max(0, n))
		for j := range slots {
			slots[j] = InputOTPSlot(index)
			index++
		}
		o.parts = append(o.parts, InputOTPGroup(slots...))
	}
	return o
}
func (o *InputOTPWidget) Name(s string) *InputOTPWidget   { o.SetName(s); return o }
func (o *InputOTPWidget) SetName(s string)                { o.Interactive.SetName(s); o.input.Name(s) }
func (o *InputOTPWidget) Disabled(v bool) *InputOTPWidget { o.SetInert(v); return o }
func (o *InputOTPWidget) BindDisabled(r ggui.Readable[bool]) *InputOTPWidget {
	o.BindInert(r)
	return o
}
func (o *InputOTPWidget) Invalid(v bool) *InputOTPWidget {
	defer property.Watch(&o.props, &o.invalid)()
	defer property.Watch(&o.props, &o.invalidWhen)()
	o.invalid = v
	o.invalidWhen = nil
	return o
}
func (o *InputOTPWidget) BindInvalid(r ggui.Readable[bool]) *InputOTPWidget {
	property.Require(r, "BindInvalid")
	if !property.Same(o.invalidWhen, r) {
		o.invalidWhen = r
		o.props.Changed()
	}
	return o
}
func (o *InputOTPWidget) RTL(v bool) *InputOTPWidget {
	defer property.Watch(&o.props, &o.rtl)()
	o.rtl = v
	return o
}
func (o *InputOTPWidget) SlotSize(s float64) *InputOTPWidget {
	defer property.Watch(&o.props, &o.slotSize)()
	o.slotSize = max(1, s)
	return o
}
func (o *InputOTPWidget) Placeholder(s string) *InputOTPWidget {
	defer property.Watch(&o.props, &o.placeholder)()
	o.placeholder = []rune(s)
	return o
}
func (o *InputOTPWidget) OnChange(fn func(string)) *InputOTPWidget   { o.onChange = fn; return o }
func (o *InputOTPWidget) OnComplete(fn func(string)) *InputOTPWidget { o.onComplete = fn; return o }
func (o *InputOTPWidget) OnSubmit(fn func(string)) *InputOTPWidget   { o.input.OnSubmit(fn); return o }

// Alphanumeric accepts ASCII letters and digits.
func (o *InputOTPWidget) Alphanumeric() *InputOTPWidget {
	defer property.Watch(&o.props, &o.accept)()
	o.accept = func(r rune) bool { return r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
	return o
}

// Pattern accepts a character only if the expression matches that whole rune.
func (o *InputOTPWidget) Pattern(pattern *regexp.Regexp) *InputOTPWidget {
	defer property.Watch(&o.props, &o.accept)()
	o.accept = func(r rune) bool {
		if pattern == nil {
			return true
		}
		s := string(r)
		loc := pattern.FindStringIndex(s)
		return loc != nil && loc[0] == 0 && loc[1] == len(s)
	}
	return o
}
func (o *InputOTPWidget) Accept(fn func(rune) bool) *InputOTPWidget { o.accept = fn; return o }

// Input exposes the native editor for selection, submission and advanced use.
func (o *InputOTPWidget) Input() *ggui.TextInputWidget { return o.input }
func (o *InputOTPWidget) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	defer o.props.Layout()()
	o.Sync()
	o.env, o.theme, o.reduced = e, uitheme.From(e), e.ReducedMotion()
	inherited, _ := e.Get(ggui.InputDisabled)
	o.disabled = o.IsInert() || inherited
	if o.invalidWhen != nil {
		o.invalid = o.invalidWhen.Get()
	}
	o.input.Layout(c, e.With(ggui.InputDisabled, o.disabled))
	o.cells = o.cells[:0]
	o.groups = o.groups[:0]
	o.separators = o.separators[:0]
	x := 0.
	seen := make([]bool, o.maxLength)
	for p, part := range o.parts {
		if p > 0 {
			x += o.gap
		}
		switch part := part.(type) {
		case InputOTPGroupSpec:
			start := x
			for _, slot := range part.slots {
				if slot.Index < 0 || slot.Index >= o.maxLength || seen[slot.Index] {
					panic("ui.InputOTP: each slot index must occur exactly once")
				}
				seen[slot.Index] = true
				o.cells = append(o.cells, otpCell{slot.Index, ggui.Rct(ggui.Pt(x, 0.), ggui.Sz(o.slotSize, o.slotSize))})
				x += o.slotSize
			}
			o.groups = append(o.groups, otpGroup{ggui.Rct(ggui.Pt(start, 0.), ggui.Sz(x-start, o.slotSize))})
		case InputOTPSeparatorSpec:
			o.separators = append(o.separators, ggui.Rct(ggui.Pt(x, 0.), ggui.Sz(16, o.slotSize)))
			x += 16
		}
	}
	for _, ok := range seen {
		if !ok {
			panic("ui.InputOTP: groups must cover every slot")
		}
	}
	size := c.Constrain(ggui.Sz(x, o.slotSize))
	scale := 1.
	if x > size.W && x > 0 {
		scale = size.W / x
	}
	adapt := func(r ggui.Rect) ggui.Rect {
		r.Origin.X *= scale
		r.Size.W *= scale
		r.Size.H = min(o.slotSize, size.H)
		if o.rtl {
			r.Origin.X = min(x, size.W) - r.Origin.X - r.Size.W
		}
		return r
	}
	for i := range o.cells {
		o.cells[i].rect = adapt(o.cells[i].rect)
	}
	for i := range o.groups {
		o.groups[i].rect = adapt(o.groups[i].rect)
	}
	for i := range o.separators {
		o.separators[i] = adapt(o.separators[i])
	}
	if len(o.labels) != o.maxLength {
		o.labels = make([]*ggui.TextWidget, o.maxLength)
		o.sizes = make([]ggui.Size, o.maxLength)
		for i := range o.labels {
			o.labels[i] = ggui.Text("").NoWrap()
		}
	}
	o.lastText = "\x00"
	return size
}
func (o *InputOTPWidget) Semantics() (ggui.Role, string) {
	return ggui.RoleTextField, pick(o.SemanticName() != "", o.SemanticName(), "One-time password")
}
func (o *InputOTPWidget) Describe() ggui.Node {
	n := o.input.Describe()
	n.Role, n.Name = o.Semantics()
	n.Disabled = o.disabled
	n.Runs = nil
	n.Description = fmt.Sprintf("%d-character code", o.maxLength)
	if o.invalid {
		n.Description += "; invalid code"
	}
	return n
}
func (o *InputOTPWidget) Paint(d *ggui.Canvas, r ggui.Rect) {
	o.rect = r
	o.Sync()
	if o.invalidWhen != nil {
		o.invalid = o.invalidWhen.Get()
	}
	d.Describe(r, o)
	if !o.disabled {
		d.HitPointer(r, o)
		d.HitKey(r, o)
		d.HitCursor(r, ggui.CursorShapeText)
	}
	o.input.PaintCustom(d, r, o.paintSlots)
}
func (o *InputOTPWidget) paintSlots(d *ggui.Canvas, r ggui.Rect, state ggui.TextInputState) ggui.Rect {
	t := o.theme
	fg, border, fill := t.Fg, colorOr(t.InputBorder, t.Border), t.Input
	if o.invalid {
		border = t.Destructive
	}
	if o.disabled {
		fg = fade(fg, .5)
		border = fade(border, .5)
		fill = fade(fill, .5)
	}
	translate := func(at ggui.Rect) ggui.Rect { at.Origin = at.Origin.Add(r.Origin); return at }
	for _, group := range o.groups {
		at := translate(group.rect)
		d.FillRoundRect(at, t.Radius, fill)
		d.StrokeRoundRect(at, t.Radius, 1, border)
	}
	for _, sep := range o.separators {
		at := translate(sep)
		d.FillRoundRect(ggui.Rct(at.Center().Add(ggui.Pt(-4, -.75)), ggui.Sz(8, 1.5)), .75, fg)
	}
	runes := []rune(state.Text)
	active := min(utf8.RuneCountInString(state.Text[:state.Caret]), o.maxLength-1)
	lo, hi := min(state.Anchor, state.Caret), max(state.Anchor, state.Caret)
	start, end := utf8.RuneCountInString(state.Text[:lo]), utf8.RuneCountInString(state.Text[:hi])
	if start < end {
		active = start
	}
	if state.Text != o.lastText {
		o.lastText = state.Text
		o.blink = ggui.Now()
		for i, l := range o.labels {
			s := ""
			if i < len(runes) {
				s = string(runes[i])
			} else if i < len(o.placeholder) {
				s = string(o.placeholder[i])
			}
			l.Content(s).Style(t.Text).Color(fg)
			o.sizes[i] = l.Layout(ggui.Loose(ggui.Sz(o.slotSize, o.slotSize)), o.env)
		}
	}
	caret := r
	for _, cell := range o.cells {
		at := translate(cell.rect)
		i := cell.index
		// Interior borders are straight; only the group's two outer ends are rounded.
		for _, group := range o.groups {
			if cell.rect.Origin.X > group.rect.Origin.X+.1 && cell.rect.Origin.X < group.rect.Origin.X+group.rect.Size.W-.1 {
				d.FillRect(ggui.Rct(at.Origin, ggui.Sz(1, at.Size.H)), border)
				break
			}
		}
		left, right := false, false
		for _, group := range o.groups {
			left = left || cell.rect.Origin.X == group.rect.Origin.X
			right = right || cell.rect.Origin.X+cell.rect.Size.W == group.rect.Origin.X+group.rect.Size.W
		}
		selected := state.Focused && !o.disabled && (i == active || (i >= start && i < end))
		if selected {
			ring := t.Ring
			if o.invalid {
				ring = t.Destructive
			}
			otpSlotBorder(d, ggui.Rct(at.Origin.Add(ggui.Pt(-1, -1)), ggui.Sz(at.Size.W+2, at.Size.H+2)), t.Radius+1, 3, fade(ring, .5), left, right)
			otpSlotBorder(d, at, t.Radius, 1, ring, left, right)
		}
		if i == active {
			caret = ggui.Rct(at.Center().Add(ggui.Pt(-.5, -8)), ggui.Sz(1, 16))
		}
		col := fg
		if i >= len(runes) {
			col = t.MutedFg
			if o.disabled {
				col = fade(col, .5)
			}
		}
		o.labels[i].Color(col)
		sz := o.sizes[i]
		d.Clip(at).Paint(o.labels[i], ggui.Rct(at.Center().Add(ggui.Pt(-sz.W/2, -sz.H/2)), sz))
		if i == active && selected && i >= len(runes) && !state.Composing && (o.reduced || (ggui.Now().Sub(o.blink)/(500*time.Millisecond))%2 == 0) {
			d.FillRect(caret, fg)
		}
	}
	return caret
}
func (o *InputOTPWidget) byteAt(index int) int {
	s := o.input.EditingState().Text
	at := 0
	for i := 0; i < index && at < len(s); i++ {
		_, n := utf8.DecodeRuneInString(s[at:])
		at += n
	}
	return at
}
func (o *InputOTPWidget) slotAt(p ggui.Point) int {
	best := 0
	dist := 1e30
	for _, cell := range o.cells {
		at := cell.rect
		at.Origin = at.Origin.Add(o.rect.Origin)
		delta := p.X - at.Center().X
		if delta < 0 {
			delta = -delta
		}
		if delta < dist {
			best, dist = cell.index, delta
		}
	}
	return best
}
func (o *InputOTPWidget) HandlePointer(e ggui.PointerEvent) bool {
	if o.disabled {
		return false
	}
	switch e.Kind {
	case ggui.PointerDown:
		if e.Button != ggui.MouseButtonLeft {
			return false
		}
		i := o.slotAt(e.Pos)
		o.dragAnchor = i
		o.input.Select(o.byteAt(i), o.byteAt(i+1))
		o.blink = ggui.Now()
	case ggui.PointerDrag:
		if o.Pressed {
			i := o.slotAt(e.Pos)
			o.input.Select(o.byteAt(min(i, o.dragAnchor)), o.byteAt(max(i, o.dragAnchor)+1))
		}
	}
	return o.Pointer(e, nil)
}
func (o *InputOTPWidget) HandleKey(e ggui.KeyEvent) {
	o.Keyboard(e, nil)
	if o.rtl && e.Kind == ggui.KeyPress {
		if e.Key == ggui.KeyArrowLeft {
			e.Key = ggui.KeyArrowRight
		} else if e.Key == ggui.KeyArrowRight {
			e.Key = ggui.KeyArrowLeft
		}
	}
	o.input.HandleKey(e)
	if e.Kind == ggui.KeyFocus {
		state := o.input.EditingState()
		if state.Anchor == state.Caret && utf8.RuneCountInString(state.Text) >= o.maxLength {
			o.input.Select(o.byteAt(o.maxLength-1), o.byteAt(o.maxLength))
		}
	}
	if e.Kind != ggui.KeyBlur {
		o.blink = ggui.Now()
	}
}
func (o *InputOTPWidget) HandleTick() bool                 { return o.input.HandleTick() }
func (o *InputOTPWidget) ConsumesKey(e ggui.KeyEvent) bool { return o.input.ConsumesKey(e) }
func (o *InputOTPWidget) CaptureTouchDrag() bool           { return !o.disabled }
func (o *InputOTPWidget) Act(a ggui.Action) bool           { return o.input.Act(a) }
func (o *InputOTPWidget) Adopt(prev any) {
	o.Interactive.Adopt(prev)
	if p, ok := prev.(*InputOTPWidget); ok {
		o.input.Adopt(p.input)
		o.dragAnchor = p.dragAnchor
		o.blink = p.blink
	}
}

// Clip the two halves independently so internal slot corners remain square.
func otpSlotBorder(d *ggui.Canvas, r ggui.Rect, radius, width float64, col color.Color, left, right bool) {
	for i, rounded := range []bool{left, right} {
		half := ggui.Rct(ggui.Pt(r.Origin.X+float64(i)*r.Size.W/2, r.Origin.Y-width), ggui.Sz(r.Size.W/2, r.Size.H+2*width))
		if i == 0 {
			half.Origin.X -= width
			half.Size.W += width
		} else {
			half.Size.W += width
		}
		d.Clip(half).StrokeRoundRect(r, pick(rounded, radius, 0.), width, col)
	}
}

// BindName follows the accessible name on the same targets as Name.
func (o *InputOTPWidget) BindName(r ggui.Readable[string]) *InputOTPWidget {
	o.Interactive.BindName(r)
	o.input.BindName(r)
	return o
}
