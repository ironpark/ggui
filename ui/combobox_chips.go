package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

type containsReader[T comparable] struct {
	r ggui.Readable[[]T]
	v T
}

func (c containsReader[T]) Get() bool { return slices.Contains(c.r.Get(), c.v) }

// ComboboxMultipleWidget is a field of removable chips, one per selected
// value, followed by an input that filters a list of the options, with a
// check on each selected one. Selection writes fresh slices and never
// mutates the caller's slice. Values absent from Options remain visible
// as removable chips.
type ComboboxMultipleWidget[T comparable] struct {
	props          property.Owner
	values         ggui.Binding[[]T]
	options        []T
	optionProp     property.Value[[]T]
	label          func(T) string
	placeholder    string                  // as Placeholder set it
	placeholderSet bool                    // Placeholder was called; otherwise the Strings give it
	words          *Strings                // the Strings of the last layout
	input          *widget.TextInputWidget // after the chips, filtering the list
	popup          *widget.PopupWidget
	content        *widget.BoxWidget // the list, as wide as the field
	query          *ggui.StateValue[string]
	search         *CommandWidget
	field          *comboboxChips[T]
	chips          []*comboboxChip
	shown          []T
	built          bool // the chips and entries follow label
	onChange       func([]T)
	key            any
}

// ComboboxMultiple creates a searchable multiple selection control, as
// shadcn's ComboboxChips with a ComboboxChipsInput: typing in the input
// after the chips filters the list and opens it, and the keyboard stays in
// the input. Enter toggles the highlighted result without closing;
// Up/Down move through matches. Backspace in an empty input removes the
// last chip, Escape closes the list, then empties the input, and leaving
// the input empties it too.
func ComboboxMultiple[T comparable](values ggui.Binding[[]T]) *ComboboxMultipleWidget[T] {
	property.Require(values, "ComboboxMultiple")
	c := &ComboboxMultipleWidget[T]{values: values, label: sprint[T], query: ggui.State("")}
	c.input = widget.TextInput(c.query).MinWidth(0)
	c.input.OnKey(c.handleKey).OnChange(func(string) { c.open() }).OnCommit(func(string) {
		// Leaving the input closes the list and drops what was typed.
		if !c.input.Focused() {
			c.popup.SetOpen(false)
			c.query.Set("")
		}
	})
	c.input.OnDescribe(c.describe).OnAction(c.Act)
	c.search = Command(c.query).Height(comboboxListHeight)
	c.search.menuRows, c.search.keepOpen, c.search.bare = true, true, true
	c.field = &comboboxChips[T]{c: c}
	c.content = widget.Box(c.search)
	c.popup = widget.Popup(c.field, menuSurface(&comboboxList{input: c.input, content: c.content})).
		Owner(c.input).Modal(false).FocusOnOpen(false)
	return c
}

// open shows the list, highlighting its first match.
func (c *ComboboxMultipleWidget[T]) open() {
	if c.inert() || c.popup.IsOpen() {
		return
	}
	c.search.initialized = false
	c.popup.Show()
}

// Options snapshots the available choices without changing selected values.
func (c *ComboboxMultipleWidget[T]) Options(v []T) *ComboboxMultipleWidget[T] {
	if c.optionProp.Set(slices.Clone(v)) {
		c.props.Changed()
	}
	return c
}

// BindOptions follows a source of available choices.
func (c *ComboboxMultipleWidget[T]) BindOptions(v ggui.Readable[[]T]) *ComboboxMultipleWidget[T] {
	property.Bind(&c.props, &c.optionProp, v, "BindOptions")
	return c
}

// Format formats options and selected chips.
func (c *ComboboxMultipleWidget[T]) Format(fn func(T) string) *ComboboxMultipleWidget[T] {
	if fn == nil {
		fn = sprint[T]
	}
	c.label = fn
	c.built = false
	c.props.Changed()
	return c
}

// Name sets the accessible name of the input.
func (c *ComboboxMultipleWidget[T]) Name(v string) *ComboboxMultipleWidget[T] {
	c.input.Name(v)
	return c
}

// BindName follows the accessible name of the input.
func (c *ComboboxMultipleWidget[T]) BindName(v ggui.Readable[string]) *ComboboxMultipleWidget[T] {
	c.input.BindName(v)
	return c
}

// SetName names the input for Field.
func (c *ComboboxMultipleWidget[T]) SetName(v string) { c.input.SetName(v) }

// HasName reports whether the input has an accessible name.
func (c *ComboboxMultipleWidget[T]) HasName() bool { return c.input.HasName() }

// Placeholder is the muted text the input after the chips shows while it
// is empty.
func (c *ComboboxMultipleWidget[T]) Placeholder(v string) *ComboboxMultipleWidget[T] {
	defer property.Watch(&c.props, &c.placeholder)()
	c.placeholder, c.placeholderSet = v, true
	c.input.Placeholder(v)
	return c
}

// Empty sets what the list says when nothing matches what was typed; the
// Strings' NoItemsFound, "No items found.", by default.
func (c *ComboboxMultipleWidget[T]) Empty(s string) *ComboboxMultipleWidget[T] {
	c.search.Empty(s)
	return c
}

// Key gives the input, the popup and the chips' buttons stable identities.
func (c *ComboboxMultipleWidget[T]) Key(k any) *ComboboxMultipleWidget[T] {
	c.key = k
	c.input.Key(comboboxKey{"input", k})
	c.popup.Key(comboboxKey{"popup", k})
	c.search.field.Key(comboboxKey{"search", k})
	c.built = false
	return c
}

// Disabled disables selection and chip removal and closes the popup.
func (c *ComboboxMultipleWidget[T]) Disabled(v bool) *ComboboxMultipleWidget[T] {
	c.field.SetInert(v)
	if v {
		c.popup.Hide()
	}
	return c
}

// BindDisabled follows whether this control is disabled.
func (c *ComboboxMultipleWidget[T]) BindDisabled(v ggui.Readable[bool]) *ComboboxMultipleWidget[T] {
	c.field.BindInert(v)
	return c
}

// OnChange fires after user selection or chip removal, with an isolated slice.
func (c *ComboboxMultipleWidget[T]) OnChange(fn func([]T)) *ComboboxMultipleWidget[T] {
	c.onChange = fn
	return c
}

// OnValuesChange is OnChange with a name parallel to ToggleGroupMultiple.
func (c *ComboboxMultipleWidget[T]) OnValuesChange(fn func([]T)) *ComboboxMultipleWidget[T] {
	return c.OnChange(fn)
}

// Popup exposes the underlying popup.
func (c *ComboboxMultipleWidget[T]) Popup() *widget.PopupWidget { return c.popup }

// Act supports accessible expansion and collapse.
func (c *ComboboxMultipleWidget[T]) Act(a ggui.Action) bool {
	if c.inert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		c.open()
	case ggui.ActionCollapse:
		c.popup.Hide()
	default:
		return false
	}
	return true
}

// describe makes the input a combobox that reports whether its list shows
// and, as its description, the selected values.
func (c *ComboboxMultipleWidget[T]) describe(n *ggui.Node) {
	n.Role = ggui.RoleCombobox
	if !c.input.HasName() {
		n.Name = c.words.orDefault().ChooseOptions
	}
	open := c.popup.IsOpen()
	n.Expanded = ggui.Expandable(open)
	n.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
	values := ggui.Untrack(c.values.Get)
	labels := make([]string, 0, len(values))
	for _, v := range values {
		labels = append(labels, c.label(v))
	}
	n.Description = strings.Join(labels, ", ")
}

// Semantics reports the input's role and resolved name.
func (c *ComboboxMultipleWidget[T]) Semantics() (ggui.Role, string) {
	if c.input.HasName() {
		return ggui.RoleCombobox, c.input.SemanticName()
	}
	return ggui.RoleCombobox, c.words.orDefault().ChooseOptions
}

// inert reports whether the control takes no input.
func (c *ComboboxMultipleWidget[T]) inert() bool { return c.field.IsInert() || c.input.IsDisabled() }

func (c *ComboboxMultipleWidget[T]) selectValue(v T, on bool) {
	if c.inert() {
		return
	}
	current := ggui.Untrack(c.values.Get)
	if slices.Contains(current, v) == on {
		return
	}
	next := withValue(current, v, on)
	c.values.Set(next)
	if c.onChange != nil {
		c.onChange(slices.Clone(next))
	}
}

// handleKey takes the list's keys before the editor does, and Backspace in
// an empty input, which removes the last chip.
func (c *ComboboxMultipleWidget[T]) handleKey(e ggui.KeyEvent) bool {
	if c.inert() || e.Kind != ggui.KeyPress || e.Mods != (ggui.Mods{}) {
		return false
	}
	open := c.popup.IsOpen()
	switch e.Key {
	case ggui.KeyBackspace:
		values := ggui.Untrack(c.values.Get)
		if ggui.Untrack(c.query.Get) == "" && len(values) > 0 {
			c.selectValue(values[len(values)-1], false)
			return true
		}
	case ggui.KeyArrowDown, ggui.KeyArrowUp:
		if !open {
			c.open()
			return true
		}
		return c.search.key(e)
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		if open {
			return c.search.key(e)
		}
	case ggui.KeyEscape:
		if open {
			c.popup.Hide()
			return true
		}
		if ggui.Untrack(c.query.Get) != "" {
			c.query.Set("")
			return true
		}
	}
	return false
}

// Layout implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.words = stringsOf(env)
	if !c.placeholderSet {
		c.input.Placeholder(c.words.ComboboxPlaceholder)
	}
	c.field.Sync()
	inert := c.field.IsInert()
	options := c.optionProp.Get()
	if !slices.Equal(c.options, options) || !c.built {
		c.options = slices.Clone(options)
		entries := make([]CommandEntry, len(options))
		for j, v := range c.options {
			entries[j] = CommandItem(c.label(v), func() {
				c.selectValue(v, !slices.Contains(ggui.Untrack(c.values.Get), v))
			})
		}
		c.search.Entries(entries...)
		for j, v := range c.options {
			c.search.items[j].Role = ggui.RoleOption
			c.search.items[j].BindChecked(containsReader[T]{r: c.values, v: v})
		}
	}
	values := c.values.Get()
	if !c.built || !slices.Equal(values, c.shown) {
		c.shown = slices.Clone(values)
		c.chips = c.chips[:0]
		for j, v := range values {
			label := c.label(v)
			remove := ButtonOf(widget.Box(), func() { c.selectValue(v, false) }).Ghost().Size(ButtonIconExtraSmall)
			if c.key != nil {
				remove.Key(childKey{owner: c.key, part: j})
			}
			c.chips = append(c.chips, &comboboxChip{label: label, text: caption(widget.Text(label)).NoWrap().Weight(ggui.WeightMedium), remove: remove})
		}
	}
	c.built = true
	for _, chip := range c.chips {
		chip.remove.Disabled(inert).Name(fmt.Sprintf(c.words.RemoveFormat, chip.label))
	}
	if inert {
		c.popup.Hide()
	}
	c.popup.Gap(uitheme.Of(env).Space * .75)
	size := c.popup.Layout(cs, env)
	c.content.Width(size.W)
	return size
}

// Paint implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }

// comboboxChips is a multiple combobox's field: its chips wrapped across
// lines, then the input, taking the rest of the last line or a line of
// its own.
type comboboxChips[T comparable] struct {
	ggui.Interactive // only disabled is used; the input tracks its own focus
	c                *ComboboxMultipleWidget[T]
	rects            []ggui.Rect // each chip's, from the field's origin
	open             ggui.Rect   // the input's
	theme            *uitheme.Theme
	motion           time.Duration
	invalid          bool
}

// comboboxChipHeight is a chip's height: 21, as shadcn's.
const comboboxChipHeight = 21

func (f *comboboxChips[T]) pad(t *uitheme.Theme) ggui.EdgeInsets {
	// The field is narrower at the sides once a chip is in it.
	side := pick(len(f.c.chips) > 0, t.Space/2, t.FieldPad.Left)
	return ggui.Insets(t.Space/2, side)
}

func (f *comboboxChips[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	f.Sync()
	t := uitheme.Of(env)
	f.theme = t
	f.motion = env.Motion(t.MotionFast)
	f.invalid, _ = env.Get(fieldInvalid)
	inherited, _ := env.Get(ggui.InputDisabledKey)
	env = env.With(ggui.InputDisabledKey, f.IsInert() || inherited)
	c := f.c
	pad, gap := f.pad(t), t.Space/2
	least := t.Space * 8 // the input's narrowest, shadcn's min-w-16
	natural := pad.Horizontal() + least
	sizes := make([]ggui.Size, len(c.chips))
	for i, chip := range c.chips {
		sizes[i] = chip.Layout(ggui.Loose(ggui.Sz(max(0, cs.MaxW-pad.Horizontal()), comboboxChipHeight)), env)
		natural += sizes[i].W + gap
	}
	w := clamp(max(natural, t.Space*30), cs.MinW, cs.MaxW)
	inner := max(0, w-pad.Horizontal())
	line := float64(comboboxChipHeight)
	f.rects = f.rects[:0]
	x, y := 0.0, 0.0
	for _, s := range sizes {
		if x > 0 && x+s.W > inner {
			x, y = 0, y+line+gap
		}
		f.rects = append(f.rects, ggui.Rct(ggui.Pt(pad.Left+x, pad.Top+y+(line-s.H)/2), s))
		x += s.W + gap
	}
	if x > 0 && inner-x < least {
		x, y = 0, y+line+gap
	}
	// The input after the last chip is a field's padding in from it, as
	// the text of an empty field is from its edge.
	start := pick(x > 0, t.Space/4, t.FieldPad.Left-pad.Left)
	width := max(0, inner-x-start)
	text := c.input.Layout(ggui.Constraints{MinW: width, MaxW: width, MaxH: ggui.Unbounded}, env)
	f.open = ggui.Rct(ggui.Pt(pad.Left+x+start, pad.Top+y+(line-text.H)/2), text)
	h := max(t.ControlSize, y+line+pad.Vertical())
	h = max(h, 20+t.FieldPad.Top+t.FieldPad.Bottom)
	if h > y+line+pad.Vertical() {
		// One line in a field taller than it: centered.
		shift := (h - (y + line + pad.Vertical())) / 2
		for i := range f.rects {
			f.rects[i].Origin.Y += shift
		}
		f.open.Origin.Y += shift
	}
	return cs.Constrain(ggui.Sz(w, h))
}

func (f *comboboxChips[T]) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t, c := f.theme, f.c
	inert := c.input.IsDisabled()
	focused := c.input.Focused()
	for _, chip := range c.chips {
		focused = focused || chip.remove.Focused && chip.remove.FocusVisible
	}
	// The whole field focuses and clicks into the input, and a press
	// anywhere in it opens the list; the chips' buttons sit over both.
	if inert {
		dst.Describe(r, c.input)
	} else {
		f.Hit(dst, r, c.input, ggui.CursorShapeText)
	}
	at := ggui.Anchor{Rect: r, ID: c.input.HitID()}
	fieldSurface(dst, at, r, fieldLook{theme: t, focused: focused, invalid: f.invalid, inert: inert, motion: f.motion})
	open := f.open
	open.Origin = open.Origin.Add(r.Origin)
	dst.Paint(c.input, open)
	if !inert {
		dst.HitPointer(r, comboboxPress{open: c.open})
	}
	for i, chip := range c.chips {
		at := f.rects[i]
		at.Origin = at.Origin.Add(r.Origin)
		dst.Paint(chip, at)
	}
}

// comboboxChip is one selected value in a multiple combobox's field: its
// label on a muted pill, and a button that removes it.
type comboboxChip struct {
	label    string // the value's, which names the remove button
	text     *widget.TextWidget
	remove   *ButtonWidget
	textSize ggui.Size
	theme    *uitheme.Theme
	env      ggui.Env
}

func (ch *comboboxChip) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	ch.theme, ch.env = t, env
	ink := pick(ch.remove.IsInert(), t.Disabled(t.Fg), t.Fg)
	ch.textSize = ch.text.Color(ink).Layout(ggui.Loose(ggui.Sz(max(0, c.MaxW-comboboxChipHeight-t.Space*.75), c.MaxH)), env)
	ch.remove.Layout(ggui.Tight(ggui.Sz(comboboxChipHeight, comboboxChipHeight)), env)
	return c.Constrain(ggui.Sz(t.Space*.75+ch.textSize.W+comboboxChipHeight, comboboxChipHeight))
}

func (ch *comboboxChip) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := ch.theme
	inert := ch.remove.IsInert()
	dst.FillRoundRect(r, t.Radius*.6, pick(inert, t.Disabled(t.Muted), t.Muted))
	dst.Paint(ch.text, ggui.Rct(ggui.Pt(r.Origin.X+t.Space*.75, r.Origin.Y+(r.Size.H-ch.textSize.H)/2), ch.textSize))
	// The cross is half strength until the pointer is on it.
	at := ggui.Rct(ggui.Pt(r.Origin.X+r.Size.W-comboboxChipHeight, r.Origin.Y), ggui.Sz(comboboxChipHeight, comboboxChipHeight))
	dst.Paint(ch.remove, at)
	ink := fade(t.Fg, pick(ch.remove.Hovered && !inert, 1.0, .5))
	if inert {
		ink = t.Disabled(fade(t.Fg, .5))
	}
	controlkit.PaintIconAt(dst, ch.env, controlkit.IconMark{Role: icons.Close, Center: at.Center(), Size: t.Space * 1.5, Color: ink})
}
