package ui

import (
	"math"
	"strconv"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// NumberInputWidget is a number field with − and + steppers: the number is
// typed into a text editor and committed on Enter or when the field loses
// focus, and stepped by the buttons, ArrowUp and ArrowDown (with Shift, ten
// steps at a time) or an assistive technology. A screen reader sees one
// spin button with its range, which is the editor: its caret and characters
// read as any field's do. Build one with NumberInput.
type NumberInputWidget struct {
	props property.Owner
	ggui.Interactive
	value    ggui.Binding[float64]
	text     *ggui.StateValue[string]
	input    *ggui.TextInputWidget
	lo, hi   float64
	step     float64
	digits   int // decimals shown; -1 follows the step
	onChange func(float64)

	dec, inc *numberStepper
	row      *ggui.RowWidget
	box      *ggui.BoxWidget
	theme    uitheme.Theme
	disabled bool // own or inherited, as of the last Layout
	invalid  bool // inside a Field showing an error
}

// NumberInput creates a number field bound to value, stepping by 1 with no
// bounds and showing as many decimals as the step has.
//
//	ui.Field("Quantity", ui.NumberInput(qty).Range(0, 99))
func NumberInput(value ggui.Binding[float64]) *NumberInputWidget {
	n := &NumberInputWidget{value: value, step: 1, digits: -1, lo: math.Inf(-1), hi: math.Inf(1)}
	n.text = ggui.State(n.format(ggui.Untrack(value.Get)))
	n.input = ggui.TextInput(n.text).MinWidth(64).Filter(numberChars).OnKey(n.key).OnCommit(func(string) { n.commit() }).
		OnDescribe(n.describe).OnAction(n.act)
	n.dec = &numberStepper{n: n, dir: -1}
	n.inc = &numberStepper{n: n, dir: 1}
	n.dec.Role, n.inc.Role = ggui.RoleButton, ggui.RoleButton
	n.dec.SetName("Decrement")
	n.inc.SetName("Increment")
	// The text follows the value, so a write from outside -- or a commit
	// that normalised what was typed -- shows at once.
	reactive.Observe(func() {
		v := n.value.Get()
		n.text.Set(ggui.Untrack(func() string { return n.format(v) }))
	})
	return n
}

// Range bounds the value to [lo, hi]; typed and stepped values are clamped
// to it and the stepper that would leave it is disabled.
func (n *NumberInputWidget) Range(lo, hi float64) *NumberInputWidget {
	defer property.Watch(&n.props, &n.hi)()
	defer property.Watch(&n.props, &n.lo)()
	n.lo, n.hi = min(lo, hi), max(lo, hi)
	return n
}

// Step sets how far one step moves the value; the default is 1. Without a
// Precision the field shows as many decimals as the step has.
func (n *NumberInputWidget) Step(step float64) *NumberInputWidget {
	defer property.Watch(&n.props, &n.step)()
	if step <= 0 || math.IsNaN(step) || math.IsInf(step, 0) {
		step = 1
	}
	n.step = step
	n.reformat()
	return n
}

// Precision sets how many decimals are shown, and what a committed value
// is rounded to.
func (n *NumberInputWidget) Precision(digits int) *NumberInputWidget {
	defer property.Watch(&n.props, &n.digits)()
	n.digits = max(digits, 0)
	n.reformat()
	return n
}

// Name names the field for Probe.Find and the inspector.
func (n *NumberInputWidget) Name(s string) *NumberInputWidget { n.input.Name(s); return n }

// SetName names the field, as a Field's label does.
func (n *NumberInputWidget) SetName(s string) { n.input.Name(s) }

// HasName reports whether the field has a name of its own.
func (n *NumberInputWidget) HasName() bool { return n.input.HasName() }

// BindName binds the field's name to r; see ggui.Interactive.BindName.
func (n *NumberInputWidget) BindName(r ggui.Readable[string]) *NumberInputWidget {
	n.input.BindName(r)
	return n
}

// Key gives the field an identity, so a rebuilt one that also moved keeps
// its caret and focus.
func (n *NumberInputWidget) Key(k any) *NumberInputWidget { n.input.Key(k); return n }

// Disabled greys the field out and ignores input while v is true.
func (n *NumberInputWidget) Disabled(v bool) *NumberInputWidget { n.SetInert(v); return n }

// BindDisabled follows r for Disabled without a rebuild.
func (n *NumberInputWidget) BindDisabled(r ggui.Readable[bool]) *NumberInputWidget {
	n.BindInert(r)
	return n
}

// OnChange fires with the new value after a commit or a step changed it.
func (n *NumberInputWidget) OnChange(fn func(float64)) *NumberInputWidget { n.onChange = fn; return n }

// Input returns the editor inside, for Focused and its text setters.
func (n *NumberInputWidget) Input() *ggui.TextInputWidget { return n.input }

// precision is the number of decimals shown.
func (n *NumberInputWidget) precision() int {
	if n.digits >= 0 {
		return n.digits
	}
	s := strconv.FormatFloat(n.step, 'f', -1, 64)
	if i := strings.IndexByte(s, '.'); i >= 0 {
		return min(len(s)-i-1, 10)
	}
	return 0
}

// format writes v as the field shows it.
func (n *NumberInputWidget) format(v float64) string {
	s := strconv.FormatFloat(v, 'f', n.precision(), 64)
	if strings.Trim(s, "-0.") == "" {
		s = strings.TrimPrefix(s, "-") // no "-0"
	}
	return s
}

// reformat rewrites the text after the precision changed.
func (n *NumberInputWidget) reformat() {
	if n.text != nil {
		n.text.Set(n.format(ggui.Untrack(n.value.Get)))
	}
}

// normalize rounds v to the precision shown and clamps it to the range.
func (n *NumberInputWidget) normalize(v float64) float64 {
	if r, err := strconv.ParseFloat(strconv.FormatFloat(v, 'f', n.precision(), 64), 64); err == nil {
		v = r
	}
	return clamp(v, n.lo, n.hi) + 0 // + 0 turns -0 into 0
}

// parse reads typed text as a number, accepting a Unicode minus and a
// comma decimal separator.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "−", "-"))
	if !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// numberChars drops what cannot be part of a number as it is typed.
func numberChars(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '0' && r <= '9', r == '.', r == ',', r == '-', r == '+', r == 'e', r == 'E', r == '−':
			return r
		}
		return -1
	}, s)
}

// set stores v, normalised, shows it and reports a change.
func (n *NumberInputWidget) set(v float64) {
	v = n.normalize(v)
	setChanged(n.value, v, n.onChange)
	n.text.Set(n.format(v))
}

// commit parses what was typed into the value; text that is not a number
// reverts to the value it replaced.
func (n *NumberInputWidget) commit() {
	if v, ok := parseNumber(ggui.Untrack(n.text.Get)); ok {
		n.set(v)
		return
	}
	n.reformat()
}

// current is the value a step starts from: what is typed, while it reads
// as a number, else the value.
func (n *NumberInputWidget) current() float64 {
	if v, ok := parseNumber(ggui.Untrack(n.text.Get)); ok {
		return v
	}
	return ggui.Untrack(n.value.Get)
}

// canStep reports whether a step in dir would move the value.
func (n *NumberInputWidget) canStep(dir int) bool {
	if n.disabled || n.IsInert() {
		return false
	}
	v := ggui.Untrack(n.value.Get)
	if dir > 0 {
		return v < n.hi
	}
	return v > n.lo
}

// stepBy moves the value by k steps.
func (n *NumberInputWidget) stepBy(k float64) {
	n.set(n.current() + k*n.step)
}

// key steps on ArrowUp and ArrowDown, ten steps with Shift.
func (n *NumberInputWidget) key(ev ggui.KeyEvent) bool {
	if ev.Kind != ggui.KeyPress || ev.Mods.Ctrl || ev.Mods.Alt || ev.Mods.Meta {
		return false
	}
	k := 1.0
	if ev.Mods.Shift {
		k = 10
	}
	switch ev.Key {
	case ggui.KeyArrowUp:
		n.stepBy(k)
	case ggui.KeyArrowDown:
		n.stepBy(-k)
	case ggui.KeyPageUp:
		n.stepBy(10)
	case ggui.KeyPageDown:
		n.stepBy(-10)
	default:
		return false
	}
	return true
}

// describe makes the editor's node a spin button carrying the range and
// the value; the text shown for it, the caret and the characters are the
// editor's. Min and Max are 0 without a Range.
func (n *NumberInputWidget) describe(node *ggui.Node) {
	node.Role = ggui.RoleSpinButton
	node.Now = ggui.Untrack(n.value.Get)
	node.Actions |= ggui.ActionIncrement | ggui.ActionDecrement
	if !math.IsInf(n.lo, 0) || !math.IsInf(n.hi, 0) {
		node.Min, node.Max = n.lo, n.hi
	}
}

// act increments, decrements and sets the value, from a number or from
// text; moving the caret is the editor's.
func (n *NumberInputWidget) act(a ggui.Action) bool {
	switch a.Kind {
	case ggui.ActionIncrement:
		n.stepBy(1)
	case ggui.ActionDecrement:
		n.stepBy(-1)
	case ggui.ActionSetValue:
		v := a.Num
		if a.Text != "" {
			var ok bool
			if v, ok = parseNumber(a.Text); !ok {
				return false
			}
		}
		n.set(v)
	default:
		return false
	}
	return true
}

// Layout implements ggui.Widget.
func (n *NumberInputWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	defer n.props.Layout()()
	n.Sync()
	t := uitheme.From(env)
	n.theme = t
	inherited, _ := env.Get(ggui.InputDisabledKey)
	n.disabled = n.IsInert() || inherited
	n.invalid, _ = env.Get(fieldInvalid)
	if n.row == nil {
		n.row = ggui.Row(ggui.Expanded(n.input), n.dec, n.inc)
		n.box = ggui.Box(n.row)
	}
	n.row.Gap(t.Space / 2)
	fieldBox(n.box, t, n.input.Focused(), n.disabled)
	size := n.box.Layout(c, env.With(ggui.InputDisabledKey, n.IsInert()))
	n.disabled = n.disabled || n.input.IsDisabled()
	fieldBox(n.box, t, n.input.Focused(), n.disabled)
	return size
}

// Baseline implements ggui.Baseliner: the editor's, inside the padding.
func (n *NumberInputWidget) Baseline() (float64, bool) { return n.box.Baseline() }

// Paint implements ggui.Widget.
func (n *NumberInputWidget) Paint(dst *ggui.Canvas, r ggui.Rect) {
	n.Sync()
	// The whole box, padding included, focuses and clicks into the editor,
	// which is the element: it describes itself as the spin button.
	if n.disabled {
		dst.Describe(r, n.input)
	} else {
		n.Hit(dst, r, n.input, ggui.CursorShapeText)
	}
	if n.input.Focused() && !n.disabled {
		fieldHalo(dst, r, n.theme.Radius, fieldRing(n.theme, n.invalid))
	}
	if n.invalid {
		n.box.Border(n.theme.BorderWidth, n.theme.Destructive)
	}
	dst.Paint(n.box, r)
}

// numberStepper is the − or + button beside the editor. It takes the
// pointer only: the keyboard steps from the editor, and the spin button's
// actions are what an assistive technology uses, so it is not in the Tab
// order or the accessibility tree.
type numberStepper struct {
	ggui.Interactive
	n   *NumberInputWidget
	dir int
	env ggui.Env
}

func (s *numberStepper) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	s.env = env
	return c.Constrain(ggui.Sz(20, 20))
}

func (s *numberStepper) HandlePointer(ev ggui.PointerEvent) bool {
	return s.Pointer(ev, func() {
		if s.n.canStep(s.dir) {
			s.n.stepBy(float64(s.dir))
		}
	})
}

func (s *numberStepper) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := s.n.theme
	enabled := s.n.canStep(s.dir)
	if enabled {
		dst.HitPointer(r, s)
		dst.HitCursor(r, ggui.CursorShapePointer)
		if s.Hovered {
			dst.FillRoundRect(r, t.Radius/2, colorOr(t.Accent, t.Muted))
		}
	} else {
		s.Hovered, s.Pressed = false, false
	}
	role := pick(s.dir < 0, icons.Minus, icons.Plus)
	col := pick(enabled, t.Fg, fade(t.MutedFg, .5))
	paintIcon(dst, s.env, role, ggui.Rct(r.Center().Add(ggui.Pt(-7, -7)), ggui.Sz(14, 14)), col, 0)
}
