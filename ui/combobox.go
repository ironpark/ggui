package ui

import (
	"image/color"
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// comboboxListHeight is the most a combobox's rows take before its list
// scrolls: 288, less the search above them and the padding around them.
const comboboxListHeight = 244

// ComboboxWidget is a searchable selection popup. Keep it mounted to retain its query.
type ComboboxWidget[T comparable] struct {
	props       property.Owner
	value       ggui.Binding[T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	button      *ButtonWidget
	face        *comboboxFace
	content     *ggui.BoxWidget // the list, as wide as the trigger
	popup       *ggui.PopupWidget
	search      *CommandWidget
	query       *ggui.StateValue[string]
	onChange    func(T)
	env         ggui.Env
}

// Combobox creates a closed dropdown: an outline trigger showing the value
// and a muted chevron, opening a list as wide as itself with a search field
// at its top and a check on the chosen option. The popup supports typing,
// Up/Down, Enter, pointer selection and Escape cancellation. The options
// slice is shallow-copied.
func Combobox[T comparable](value ggui.Binding[T]) *ComboboxWidget[T] {
	c := &ComboboxWidget[T]{value: value, label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.face = &comboboxFace{text: ggui.Text("").NoWrap().Weight(ggui.WeightRegular)}
	c.button = ButtonOf(c.face, func() {
		if c.popup.IsOpen() {
			c.popup.Hide()
		} else {
			c.query.Set("")
			c.search.initialized = false
			c.search.filter()
			for i, v := range c.options {
				if v == ggui.Untrack(c.value.Get) {
					c.search.highlight = i
					break
				}
			}
			c.popup.Show()
		}
	}).Outline()
	c.button.Role = ggui.RoleCombobox
	c.button.defaultName = "Choose option"
	c.button.value = func() string { return c.face.value }
	if c.button.HitID() == nil {
		c.button.Key(c)
	}
	c.search = Command(c.query).Name("Search options").Placeholder("Search options…").Height(comboboxListHeight)
	c.search.menuRows = true
	c.content = ggui.Box(c.search)
	c.popup = ggui.Popup(c.button, c.content).Owner(c.button)
	c.button.Expands(c.popup.IsOpen)
	return c
}

type comboboxKey struct {
	part string
	id   any
}

// Key gives the trigger, popup and search editor distinct identities derived
// from k, retaining focus and popup state across rebuilds that move the control.
// Keep the control mounted to retain its search query.
func (c *ComboboxWidget[T]) Key(k any) *ComboboxWidget[T] {
	c.button.Key(comboboxKey{"trigger", k})
	c.popup.Key(comboboxKey{"popup", k})
	c.search.field.Key(comboboxKey{"search", k})
	return c
}

// Options replaces the options with a shallow copy, including after mount,
// and removes any BindOptions binding. The popup and search editor stay
// mounted: open state and query are preserved, matches are refreshed and
// the first match is highlighted. The value is never written and OnChange
// is not called; an absent value displays Placeholder.
func (c *ComboboxWidget[T]) Options(options []T) *ComboboxWidget[T] {
	changed := c.setOptions(options)
	detached := c.optionProp.Set(c.options)
	if changed || detached {
		c.props.Changed()
	}
	return c
}

// BindOptions follows r at layout without rebuilding the control. Changes
// use the same snapshot and selection rules as Options. The last setting
// wins; r must be non-nil. Use Options to detach the source.
func (c *ComboboxWidget[T]) BindOptions(r ggui.Readable[[]T]) *ComboboxWidget[T] {
	if c.optionProp.Bind(r, "BindOptions") {
		c.props.Changed()
	}
	return c
}

func (c *ComboboxWidget[T]) setOptions(options []T) bool {
	if slices.Equal(c.options, options) {
		return false
	}
	c.options = slices.Clone(options)
	entries := make([]CommandEntry, len(options))
	for i, v := range c.options {
		entries[i] = CommandItem(c.label(v), func() { setChanged(c.value, v, c.onChange); c.popup.Hide() })
	}
	c.search.setEntries(entries)
	for i, v := range c.options {
		c.search.items[i].BindChecked(ggui.Map(c.value, func(x T) bool { return x == v }))
	}
	return true
}

// Format formats the options and selected value. Configure it before layout.
func (c *ComboboxWidget[T]) Format(fn func(T) string) *ComboboxWidget[T] {
	c.label = fn
	for i, v := range c.options {
		label := fn(v)
		c.search.entries[i].label = label
		c.search.items[i].text.Content(label)
		c.search.items[i].SetName(label)
	}
	c.search.initialized = false
	return c
}

// Name names the trigger; the selected value remains visible on it.
func (c *ComboboxWidget[T]) Name(s string) *ComboboxWidget[T] { c.button.SetName(s); return c }

// Placeholder sets the trigger text when value is not in options.
func (c *ComboboxWidget[T]) Placeholder(s string) *ComboboxWidget[T] {
	defer property.Watch(&c.props, &c.placeholder)()
	c.placeholder = s
	return c
}

// Disabled disables the trigger and closes the popup.
func (c *ComboboxWidget[T]) Disabled(v bool) *ComboboxWidget[T] {
	c.button.Disabled(v)
	if v {
		c.popup.Hide()
	}
	return c
}

// OnChange fires after a user chooses a different value.
func (c *ComboboxWidget[T]) OnChange(fn func(T)) *ComboboxWidget[T] { c.onChange = fn; return c }

// Popup exposes the underlying popup for programmatic opening and closing.
func (c *ComboboxWidget[T]) Popup() *ggui.PopupWidget { return c.popup }

// Layout implements ggui.Widget.
func (c *ComboboxWidget[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.env = env
	c.setOptions(c.optionProp.Get())
	c.button.Sync()
	if c.button.IsInert() {
		c.popup.Hide()
	}
	label, chosen := c.placeholder, false
	for _, v := range c.options {
		if v == ggui.Untrack(c.value.Get) {
			label, chosen = c.label(v), true
			break
		}
	}
	t := uitheme.Of(env)
	c.face.text.Content(label)
	c.face.placeholder, c.face.value = !chosen, pick(chosen, label, "")
	c.face.inert = c.button.IsInert()
	// A trigger stretched wider than its label keeps the label at the
	// start and the chevron at the end.
	c.face.least = max(0, cs.MinW-ButtonDefault.Style(*t).Pad.Horizontal()-2*t.BorderWidth)
	c.popup.Gap(t.Space * .75)
	size := c.popup.Layout(cs, env)
	// As wide as the trigger, and never narrower than 200, room for the
	// search and a few words.
	c.content.Width(max(size.W, t.Space*25))
	return size
}

// comboboxFace is a combobox trigger's content: the value, or a muted
// placeholder, at the start, and a muted chevron at the end.
type comboboxFace struct {
	text               *ggui.TextWidget
	value              string // the chosen option's label, the trigger's accessible value
	placeholder, inert bool
	least              float64 // the width it takes at least, when its trigger is stretched
	textSize           ggui.Size
	theme              *uitheme.Theme
	env                ggui.Env
}

func (f *comboboxFace) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	f.theme, f.env = t, env
	var ink color.Color = t.Fg
	if f.placeholder {
		ink = t.MutedFg
	}
	if f.inert {
		ink = t.Disabled(ink)
	}
	trail := t.Space + t.IconSize
	f.textSize = f.text.Color(ink).Layout(ggui.Loose(ggui.Sz(max(0, c.MaxW-trail), c.MaxH)), env)
	return c.Constrain(ggui.Sz(max(f.textSize.W+trail, f.least), max(f.textSize.H, t.IconSize)))
}

func (f *comboboxFace) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := f.theme
	dst.Paint(f.text, ggui.Rct(ggui.Pt(r.Origin.X, r.Origin.Y+(r.Size.H-f.textSize.H)/2), f.textSize))
	ink := t.MutedFg
	if f.inert {
		ink = t.Disabled(ink)
	}
	paintIconAt(dst, f.env, icons.ChevronDown, ggui.Pt(r.Origin.X+r.Size.W-t.IconSize/2, r.Origin.Y+r.Size.H/2), t.IconSize, ink, 0)
}

// Paint implements ggui.Widget.
func (c *ComboboxWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }

// SetName names the trigger for Field.
func (c *ComboboxWidget[T]) SetName(s string) { c.Name(s) }

// HasName reports whether the trigger has an explicit name.
func (c *ComboboxWidget[T]) HasName() bool { return c.button.HasName() }

// Semantics reports the trigger's role and resolved name.
func (c *ComboboxWidget[T]) Semantics() (ggui.Role, string) { return c.button.Semantics() }

// BindDisabled follows r and closes the popup while disabled.
func (c *ComboboxWidget[T]) BindDisabled(r ggui.Readable[bool]) *ComboboxWidget[T] {
	c.button.BindDisabled(r)
	return c
}

// BindName follows the accessible name on the same targets as Name.
func (c *ComboboxWidget[T]) BindName(r ggui.Readable[string]) *ComboboxWidget[T] {
	c.button.BindName(r)
	return c
}

// ComboboxMultipleWidget is a field of removable chips, one per selected
// value, that opens a searchable list of options with a check on each
// selected one. Selection writes fresh slices and never mutates the
// caller's slice. Values absent from Options remain visible as removable
// chips.
type ComboboxMultipleWidget[T comparable] struct {
	props       property.Owner
	values      ggui.Binding[[]T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	button      *ButtonWidget // the field's opening part, after the chips
	popup       *ggui.PopupWidget
	content     *ggui.BoxWidget // the list, as wide as the field
	query       *ggui.StateValue[string]
	search      *CommandWidget
	field       *comboboxChips[T]
	chips       []*comboboxChip
	shown       []T
	built       bool // the chips and entries follow label
	onChange    func([]T)
	key         any
}

// ComboboxMultiple creates a searchable multiple selection control. Enter
// toggles the highlighted result without closing; Up/Down move through matches.
// Empty-search Backspace removes the last chip, and Escape dismisses the popup.
func ComboboxMultiple[T comparable](values ggui.Binding[[]T]) *ComboboxMultipleWidget[T] {
	property.Require(values, "ComboboxMultiple")
	c := &ComboboxMultipleWidget[T]{values: values, label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.button = ButtonOf(ggui.Box(), c.toggle)
	c.button.Role = ggui.RoleCombobox
	c.button.defaultName = "Choose options"
	c.button.value = func() string {
		labels := make([]string, 0, len(ggui.Untrack(values.Get)))
		for _, v := range ggui.Untrack(values.Get) {
			labels = append(labels, c.label(v))
		}
		return strings.Join(labels, ", ")
	}
	c.search = Command(c.query).Name("Search options").Placeholder("Search options…").Height(comboboxListHeight)
	c.search.menuRows, c.search.keepOpen = true, true
	c.search.onKey = c.handleKey
	c.field = &comboboxChips[T]{c: c, hint: ggui.Text("").NoWrap()}
	c.content = ggui.Box(c.search)
	c.popup = ggui.Popup(c.field, c.content).Owner(c.button).Modal(false)
	c.button.Expands(c.popup.IsOpen).Opens(c)
	return c
}

// toggle opens the list with an empty search, or closes it.
func (c *ComboboxMultipleWidget[T]) toggle() {
	if c.popup.IsOpen() {
		c.popup.Hide()
		return
	}
	c.query.Set("")
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
	c.label = fn
	c.built = false
	c.props.Changed()
	return c
}

// Name sets the accessible name of the control's trigger.
func (c *ComboboxMultipleWidget[T]) Name(v string) *ComboboxMultipleWidget[T] {
	c.button.Name(v)
	return c
}

// BindName follows the accessible name of the trigger.
func (c *ComboboxMultipleWidget[T]) BindName(v ggui.Readable[string]) *ComboboxMultipleWidget[T] {
	c.button.BindName(v)
	return c
}

// SetName names the trigger for Field.
func (c *ComboboxMultipleWidget[T]) SetName(v string) { c.button.SetName(v) }

// HasName reports whether the trigger has an accessible name.
func (c *ComboboxMultipleWidget[T]) HasName() bool { return c.button.HasName() }

// Placeholder is the muted text after the chips that opens the list.
func (c *ComboboxMultipleWidget[T]) Placeholder(v string) *ComboboxMultipleWidget[T] {
	defer property.Watch(&c.props, &c.placeholder)()
	c.placeholder = v
	return c
}

// Key gives the trigger, popup and search editor stable identities.
func (c *ComboboxMultipleWidget[T]) Key(k any) *ComboboxMultipleWidget[T] {
	c.key = k
	c.button.Key(comboboxKey{"trigger", k})
	c.popup.Key(comboboxKey{"popup", k})
	c.search.field.Key(comboboxKey{"search", k})
	c.built = false
	return c
}

// Disabled disables selection and chip removal and closes the popup.
func (c *ComboboxMultipleWidget[T]) Disabled(v bool) *ComboboxMultipleWidget[T] {
	c.button.Disabled(v)
	if v {
		c.popup.Hide()
	}
	return c
}

// BindDisabled follows whether this control is disabled.
func (c *ComboboxMultipleWidget[T]) BindDisabled(v ggui.Readable[bool]) *ComboboxMultipleWidget[T] {
	c.button.BindDisabled(v)
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
func (c *ComboboxMultipleWidget[T]) Popup() *ggui.PopupWidget { return c.popup }

// Act supports accessible expansion and collapse.
func (c *ComboboxMultipleWidget[T]) Act(a ggui.Action) bool {
	if c.button.IsInert() {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		c.popup.Show()
	case ggui.ActionCollapse:
		c.popup.Hide()
	default:
		return false
	}
	return true
}

func (c *ComboboxMultipleWidget[T]) selectValue(v T, on bool) {
	if c.button.IsInert() {
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

// handleKey takes Backspace in an empty search, which removes the last
// chip; the list handles the rest.
func (c *ComboboxMultipleWidget[T]) handleKey(e ggui.KeyEvent) bool {
	if c.button.IsInert() || e.Kind != ggui.KeyPress || e.Key != ggui.KeyBackspace {
		return false
	}
	values := ggui.Untrack(c.values.Get)
	if ggui.Untrack(c.query.Get) == "" && len(values) > 0 {
		c.selectValue(values[len(values)-1], false)
		return true
	}
	return false
}

// Layout implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.button.Sync()
	inert := c.button.IsInert()
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
			c.search.items[j].BindChecked(ggui.Map(c.values, func(vs []T) bool { return slices.Contains(vs, v) }))
		}
	}
	values := c.values.Get()
	if !c.built || !slices.Equal(values, c.shown) {
		c.shown = slices.Clone(values)
		c.chips = c.chips[:0]
		for j, v := range values {
			label := c.label(v)
			remove := ButtonOf(ggui.Box(), func() { c.selectValue(v, false) }).Ghost().Size(ButtonIconExtraSmall).Name("Remove " + label)
			if c.key != nil {
				remove.Key(childKey{owner: c.key, part: j})
			}
			c.chips = append(c.chips, &comboboxChip{text: caption(ggui.Text(label)).NoWrap().Weight(ggui.WeightMedium), remove: remove})
		}
	}
	c.built = true
	for _, chip := range c.chips {
		chip.remove.Disabled(inert)
	}
	if inert {
		c.popup.Hide()
	}
	c.field.hint.Content(c.placeholder)
	c.popup.Gap(uitheme.Of(env).Space * .75)
	size := c.popup.Layout(cs, env)
	c.content.Width(size.W)
	return size
}

// Paint implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }

// comboboxChips is a multiple combobox's field: its chips wrapped across
// lines, then the part that opens the list, taking the rest of the last
// line or a line of its own.
type comboboxChips[T comparable] struct {
	c        *ComboboxMultipleWidget[T]
	hint     *ggui.TextWidget // the placeholder in the opening part
	hintSize ggui.Size
	rects    []ggui.Rect // each chip's, from the field's origin
	open     ggui.Rect   // the opening part's
	theme    *uitheme.Theme
}

// comboboxChipHeight is a chip's height: 21, as shadcn's.
const comboboxChipHeight = 21

func (f *comboboxChips[T]) pad(t *uitheme.Theme) ggui.EdgeInsets {
	// The field is narrower at the sides once a chip is in it.
	side := pick(len(f.c.chips) > 0, t.Space/2, t.FieldPad.Left)
	return ggui.Insets(t.Space/2, side)
}

func (f *comboboxChips[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	t := uitheme.Of(env)
	f.theme = t
	c := f.c
	pad, gap := f.pad(t), t.Space/2
	ink := pick(c.button.IsInert(), t.Disabled(t.MutedFg), t.MutedFg)
	f.hintSize = f.hint.Color(ink).Layout(ggui.Constraints{MaxW: ggui.Unbounded, MaxH: ggui.Unbounded}, env)
	least := t.Space * 8 // the opening part's narrowest
	natural := pad.Horizontal() + max(least, f.hintSize.W)
	sizes := make([]ggui.Size, len(c.chips))
	for i, chip := range c.chips {
		sizes[i] = chip.Layout(ggui.Loose(ggui.Sz(max(0, cs.MaxW-pad.Horizontal()), comboboxChipHeight)), env)
		natural += sizes[i].W + gap
	}
	w := clamp(max(natural, t.Space*30), cs.MinW, cs.MaxW)
	inner := max(0, w-pad.Horizontal())
	line := max(comboboxChipHeight, f.hintSize.H)
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
	f.open = ggui.Rct(ggui.Pt(pad.Left+x, pad.Top+y), ggui.Sz(max(0, inner-x), line))
	h := max(t.ControlSize, y+line+pad.Vertical())
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
	inert := c.button.IsInert()
	focused := c.button.Focused
	for _, chip := range c.chips {
		focused = focused || chip.remove.Focused && chip.remove.FocusVisible
	}
	fill, border := t.Input, pick(focused, t.Ring, t.InputBorder)
	if inert {
		fill, border = fade(t.InputBorder, pick(isDark(t.Bg), .8, .5)), t.Disabled(t.InputBorder)
	}
	dst.FillRoundRect(r, t.Radius, fill)
	dst.StrokeRoundRect(r, t.Radius, t.BorderWidth, border)
	if focused && !inert {
		fieldHalo(dst, t, r, t.Radius, t.Ring)
	}
	// The opening part first, so the chips' buttons sit over it.
	open := f.open
	open.Origin = open.Origin.Add(r.Origin)
	c.button.Hit(dst, open, c.button, ggui.CursorShapePointer)
	dst.Paint(f.hint, ggui.Rct(ggui.Pt(open.Origin.X, open.Origin.Y+(open.Size.H-f.hintSize.H)/2), f.hintSize))
	for i, chip := range c.chips {
		at := f.rects[i]
		at.Origin = at.Origin.Add(r.Origin)
		dst.Paint(chip, at)
	}
}

// comboboxChip is one selected value in a multiple combobox's field: its
// label on a muted pill, and a button that removes it.
type comboboxChip struct {
	text     *ggui.TextWidget
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
	paintIconAt(dst, ch.env, icons.Close, at.Center(), t.Space*1.5, ink, 0)
}
