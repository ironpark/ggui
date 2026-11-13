package ui

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// comboboxListHeight is the most a combobox's rows take before its list
// scrolls: shadcn's 288 less the 36 it keeps for a search, 252, less the
// padding around the rows.
const comboboxListHeight = 244

// ComboboxWidget is a text field that filters a list of options as it is
// typed in, and chooses one of them. Keep it mounted to retain what was
// typed.
type ComboboxWidget[T comparable] struct {
	props       property.Owner
	value       ggui.Binding[T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	text        *ggui.StateValue[string] // what the input shows
	query       *ggui.StateValue[string] // what the list is filtered by
	typed       bool                     // text was typed since a value was last shown in the input
	input       *ggui.TextInputWidget
	field       *comboboxField
	chevron     *comboboxAddon
	clear       *comboboxAddon
	clearable   bool
	content     *ggui.BoxWidget // the list, as wide as the field
	popup       *ggui.PopupWidget
	search      *CommandWidget
	onChange    func(T)
}

// Combobox creates a field holding an editable input and a muted chevron,
// as shadcn's ComboboxInput is. Typing filters a list of the options that
// opens below the field as wide as it, with a check on the chosen one; the
// list also opens on Up or Down, on a click in the input and on the
// chevron. The keyboard stays in the input throughout: Up and Down move
// the highlight, Enter chooses the highlighted option, Escape closes the
// list and then puts back what was typed over, and Tab moves on. Choosing
// puts the option's label in the input; leaving the input after typing
// puts back the chosen option's label. The options slice is
// shallow-copied.
func Combobox[T comparable](value ggui.Binding[T]) *ComboboxWidget[T] {
	c := &ComboboxWidget[T]{value: value, label: sprint[T], placeholder: "Select…", text: ggui.State(""), query: ggui.State("")}
	c.input = ggui.TextInput(c.text).Placeholder(c.placeholder).MinWidth(0)
	c.input.OnKey(c.key).OnChange(c.typing).OnCommit(func(string) {
		if !c.input.Focused() {
			c.left()
		}
	})
	c.input.OnDescribe(c.describe).OnAction(c.act)
	c.chevron = &comboboxAddon{icon: icons.ChevronDown, onTap: c.toggle, chevron: true}
	c.chevron.Role = ggui.RoleButton
	c.chevron.SetName("Show options")
	c.chevron.expands = func() bool { return c.popup.IsOpen() }
	c.clear = &comboboxAddon{icon: icons.Close, onTap: c.clearValue}
	c.clear.Role = ggui.RoleButton
	c.clear.SetName("Clear")
	c.field = &comboboxField{input: c.input, open: c.toggleFromInput}
	c.search = Command(c.query).Name("Search options").Height(comboboxListHeight).Empty("No items found.")
	c.search.menuRows, c.search.bare = true, true
	c.content = ggui.Box(c.search)
	c.popup = ggui.Popup(c.field, menuSurface(&comboboxList{input: c.input, content: c.content})).
		Owner(c.input).Modal(false).FocusOnOpen(false)
	return c
}

type comboboxKey struct {
	part string
	id   any
}

// Key gives the input and the popup distinct identities derived from k,
// retaining focus, what was typed and the popup's state across rebuilds
// that move the control.
func (c *ComboboxWidget[T]) Key(k any) *ComboboxWidget[T] {
	c.input.Key(comboboxKey{"input", k})
	c.popup.Key(comboboxKey{"popup", k})
	c.search.field.Key(comboboxKey{"search", k})
	return c
}

// Options replaces the options with a shallow copy, including after mount,
// and removes any BindOptions binding. The popup and input stay mounted:
// open state and what was typed are preserved, matches are refreshed and
// the first match is highlighted. The value is never written and OnChange
// is not called; an absent value leaves the input empty, showing
// Placeholder.
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
		entries[i] = CommandItem(c.label(v), func() { c.choose(v) })
	}
	c.search.setEntries(entries)
	for i, v := range c.options {
		c.search.items[i].Role = ggui.RoleOption
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

// Name names the input; what it holds is its value.
func (c *ComboboxWidget[T]) Name(s string) *ComboboxWidget[T] { c.input.Name(s); return c }

// Placeholder sets the muted text the input shows while it is empty: when
// the value is not among the options and nothing is typed.
func (c *ComboboxWidget[T]) Placeholder(s string) *ComboboxWidget[T] {
	defer property.Watch(&c.props, &c.placeholder)()
	c.placeholder = s
	c.input.Placeholder(s)
	return c
}

// Empty sets what the list says when nothing matches what was typed; "No
// items found." by default.
func (c *ComboboxWidget[T]) Empty(s string) *ComboboxWidget[T] { c.search.Empty(s); return c }

// ClearButton shows a button at the end of the field that clears the
// value, as shadcn's showClear does, in place of the chevron while a value
// is chosen. Clearing writes T's zero value and calls OnChange with it.
func (c *ComboboxWidget[T]) ClearButton(v bool) *ComboboxWidget[T] {
	defer property.Watch(&c.props, &c.clearable)()
	c.clearable = v
	return c
}

// Disabled disables the input and closes the popup.
func (c *ComboboxWidget[T]) Disabled(v bool) *ComboboxWidget[T] {
	c.field.SetInert(v)
	if v {
		c.popup.Hide()
	}
	return c
}

// OnChange fires after a user chooses a different value.
func (c *ComboboxWidget[T]) OnChange(fn func(T)) *ComboboxWidget[T] { c.onChange = fn; return c }

// Popup exposes the underlying popup for programmatic opening and closing.
func (c *ComboboxWidget[T]) Popup() *ggui.PopupWidget { return c.popup }

// chosen returns the label of the value when it is among the options.
func (c *ComboboxWidget[T]) chosen() (string, bool) {
	v := ggui.Untrack(c.value.Get)
	for _, o := range c.options {
		if o == v {
			return c.label(o), true
		}
	}
	return "", false
}

// show puts the chosen option's label, or nothing, in the input, and
// filters the list by nothing.
func (c *ComboboxWidget[T]) show() {
	label, _ := c.chosen()
	c.typed = false
	c.query.Set("")
	c.text.Set(label)
	// The caret goes to the end of the label, as a browser's does.
	c.input.Select(len(label), len(label))
}

// open shows the list. Unless something was typed it lists every option
// with the chosen one highlighted.
func (c *ComboboxWidget[T]) open() {
	if c.inert() || c.popup.IsOpen() {
		return
	}
	if !c.typed {
		c.query.Set("")
	}
	c.search.initialized = false
	c.search.filter()
	if !c.typed {
		v := ggui.Untrack(c.value.Get)
		for i, o := range c.options {
			if o == v {
				c.search.highlight = i
				break
			}
		}
	}
	c.popup.Show()
}

// toggle opens a closed list and closes an open one, for the chevron.
func (c *ComboboxWidget[T]) toggle() {
	if c.popup.IsOpen() {
		c.popup.Hide()
		return
	}
	c.open()
}

// toggleFromInput opens the list on a press in the input.
func (c *ComboboxWidget[T]) toggleFromInput() { c.open() }

// choose makes v the value and puts its label in the input; the list has
// closed already.
func (c *ComboboxWidget[T]) choose(v T) {
	setChanged(c.value, v, c.onChange)
	c.show()
}

// clearValue writes T's zero value and empties the input.
func (c *ComboboxWidget[T]) clearValue() {
	var zero T
	setChanged(c.value, zero, c.onChange)
	c.show()
}

// typing follows an edit in the input: the list is filtered by it, and
// opens if it was closed.
func (c *ComboboxWidget[T]) typing(s string) {
	c.typed = true
	c.query.Set(s)
	if !c.popup.IsOpen() {
		c.open()
	}
}

// left follows the input losing the keyboard: the list closes, and what
// was typed gives way to the chosen option's label again.
func (c *ComboboxWidget[T]) left() {
	c.popup.SetOpen(false)
	if c.typed {
		c.show()
	}
}

// key takes the list's keys before the editor does.
func (c *ComboboxWidget[T]) key(ev ggui.KeyEvent) bool {
	if ev.Mods != (ggui.Mods{}) || c.inert() {
		return false
	}
	open := c.popup.IsOpen()
	switch ev.Key {
	case ggui.KeyArrowDown, ggui.KeyArrowUp:
		if !open {
			c.open()
			return true
		}
		return c.search.key(ev)
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		if !open {
			return false
		}
		c.search.filter()
		c.search.choose(c.search.highlight)
		return true
	case ggui.KeyEscape:
		if open {
			c.popup.Hide()
			return true
		}
		if label, _ := c.chosen(); ggui.Untrack(c.text.Get) != label {
			c.show()
			return true
		}
	}
	return false
}

// describe makes the input a combobox that reports whether its list shows.
func (c *ComboboxWidget[T]) describe(n *ggui.Node) {
	n.Role = ggui.RoleCombobox
	if !c.input.HasName() {
		n.Name = "Choose option"
	}
	open := c.popup.IsOpen()
	n.Expanded = ggui.Expandable(open)
	n.Actions |= pick(open, ggui.ActionCollapse, ggui.ActionExpand)
}

// act carries out the expand and collapse actions on the input.
func (c *ComboboxWidget[T]) act(a ggui.Action) bool {
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

// inert reports whether the control takes no input.
func (c *ComboboxWidget[T]) inert() bool { return c.field.IsInert() || c.input.IsDisabled() }

// Layout implements ggui.Widget.
func (c *ComboboxWidget[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.setOptions(c.optionProp.Get())
	c.field.Sync()
	if c.field.IsInert() {
		c.popup.Hide()
	}
	label, chosen := c.chosen()
	if !c.typed && ggui.Untrack(c.text.Get) != label {
		c.text.Set(label)
	}
	// The clear button stands in for the chevron while there is a value
	// to clear.
	c.field.addon = c.chevron
	if c.clearable && chosen {
		c.field.addon = c.clear
	}
	t := uitheme.Of(env)
	c.popup.Gap(t.Space * .75)
	size := c.popup.Layout(cs, env)
	c.content.Width(size.W)
	return size
}

// Paint implements ggui.Widget.
func (c *ComboboxWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }

// SetName names the input for Field.
func (c *ComboboxWidget[T]) SetName(s string) { c.Name(s) }

// HasName reports whether the input has an explicit name.
func (c *ComboboxWidget[T]) HasName() bool { return c.input.HasName() }

// Semantics reports the input's role and resolved name.
func (c *ComboboxWidget[T]) Semantics() (ggui.Role, string) {
	if c.input.HasName() {
		return ggui.RoleCombobox, c.input.SemanticName()
	}
	return ggui.RoleCombobox, "Choose option"
}

// BindDisabled follows r and closes the popup while disabled.
func (c *ComboboxWidget[T]) BindDisabled(r ggui.Readable[bool]) *ComboboxWidget[T] {
	c.field.BindInert(r)
	return c
}

// BindName follows the accessible name on the same targets as Name.
func (c *ComboboxWidget[T]) BindName(r ggui.Readable[string]) *ComboboxWidget[T] {
	c.input.BindName(r)
	return c
}

// comboboxField is a combobox's field: the field chrome around its input,
// with the chevron or the clear button at the end, as shadcn's InputGroup
// around ComboboxInput.
type comboboxField struct {
	ggui.Interactive // only disabled is used; the editor tracks its own focus
	input            *ggui.TextInputWidget
	addon            *comboboxAddon
	open             func() // a press in the input opens the list
	inputRect        ggui.Rect
	addonRect        ggui.Rect
	theme            *uitheme.Theme
	motion           time.Duration
	sides            JoinedSides
	invalid          bool
}

func (f *comboboxField) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	f.Sync()
	t := uitheme.Of(env)
	f.theme = t
	f.motion = env.Motion(t.MotionFast)
	f.sides, env = takeJoinedSides(env)
	f.invalid, _ = env.Get(fieldInvalid)
	inherited, _ := env.Get(ggui.InputDisabledKey)
	env = env.With(ggui.InputDisabledKey, f.IsInert() || inherited)
	// shadcn's input keeps its px-2.5 at the start and pr-1.5 before the
	// addon, which sits pr-2 less its button's 0.3rem from the end: a
	// field's padding, three quarters of a Space and half of one.
	button := buttonExtraSmall(*t).Height
	left, gap, right := t.FieldPad.Left, t.Space*.75, t.Space/2
	h := clamp(20+t.FieldPad.Top+t.FieldPad.Bottom, cs.MinH, cs.MaxH)
	// As wide as it is given, or 200, room for a few words, when the
	// width is left to it.
	w := clamp(t.Space*25, cs.MinW, cs.MaxW)
	f.addonRect = ggui.Rct(ggui.Pt(w-right-button, (h-button)/2), ggui.Sz(button, button))
	f.addon.Layout(ggui.Tight(f.addonRect.Size), env)
	inner := max(0, w-left-gap-button-right)
	size := f.input.Layout(ggui.Constraints{MinW: inner, MaxW: inner, MaxH: h}, env)
	f.inputRect = ggui.Rct(ggui.Pt(left, (h-size.H)/2), size)
	return ggui.Sz(w, h)
}

func (f *comboboxField) Baseline() (float64, bool) {
	if b, ok := f.input.Baseline(); ok {
		return f.inputRect.Origin.Y + b, true
	}
	return 0, false
}

func (f *comboboxField) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := f.theme
	// The whole field, padding included, focuses and clicks into the
	// input, and a press anywhere in it opens the list.
	inert := f.input.IsDisabled()
	if inert {
		dst.Describe(r, f.input)
	} else {
		f.Hit(dst, r, f.input, ggui.CursorShapeText)
	}
	at := ggui.Anchor{Rect: r, ID: f.input.HitID()}
	fieldSurface(dst, at, t, r, t.Radius, fieldState{f.input.Focused(), f.invalid, inert}, f.motion, f.sides.within(dst))
	place := func(at ggui.Rect) ggui.Rect { at.Origin = at.Origin.Add(r.Origin); return at }
	dst.Paint(f.input, place(f.inputRect))
	if !inert {
		// Over the input's own region, which would take the press first.
		dst.HitPointer(r, comboboxPress{open: f.open})
	}
	dst.Paint(f.addon, place(f.addonRect))
}

// comboboxPress opens the list on a press in the field, then lets the
// press through to the input, which places the caret.
type comboboxPress struct{ open func() }

func (p comboboxPress) HandlePointer(ev ggui.PointerEvent) bool {
	if ev.Kind == ggui.PointerDown && ev.Button == ggui.MouseButtonLeft {
		p.open()
	}
	return false
}

// comboboxList is a combobox's list in its popup. Its region takes the
// keyboard for the input, so a click on a row leaves the keyboard there:
// the list is never focused itself.
type comboboxList struct {
	input   *ggui.TextInputWidget
	content ggui.Widget
}

func (l *comboboxList) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	return l.content.Layout(cs, env)
}

func (l *comboboxList) Paint(dst *ggui.Canvas, r ggui.Rect) {
	dst.Paint(l.content, r)
	// After the rows, so the region is not merged into the popup's own
	// over the same Rect, which would hide it from the popup.
	if !l.input.IsDisabled() {
		dst.HitKey(r, l.input)
	}
}

// comboboxAddon is a button at the end of a combobox's field: the chevron
// that opens the list, or the button that clears the value. It is shadcn's
// ghost, extra small InputGroupButton, muted, and like Base UI's it is not
// a Tab stop: the keyboard stays in the input, which opens the list with
// Down and empties with Escape.
type comboboxAddon struct {
	ggui.Interactive
	icon    icons.Role
	chevron bool // the full-size, always muted chevron rather than a cross
	expands func() bool
	onTap   func()
	inert   bool
	theme   *uitheme.Theme
	env     ggui.Env
}

func (a *comboboxAddon) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	a.Sync()
	a.theme, a.env = uitheme.Of(env), env
	disabled, _ := env.Get(ggui.InputDisabledKey)
	a.inert = a.IsInert() || disabled
	return cs.Constrain(ggui.Sz(cs.MaxW, cs.MaxH))
}

// Describe implements ggui.Describer.
func (a *comboboxAddon) Describe() ggui.Node {
	n := ggui.Node{Role: ggui.RoleButton, Name: a.SemanticName(), Disabled: a.inert, Actions: ggui.ActionPress}
	if a.expands != nil {
		open := a.expands()
		n.Expanded = ggui.Expandable(open)
	}
	return n
}

// Act implements ggui.Actor.
func (a *comboboxAddon) Act(act ggui.Action) bool {
	if a.inert || act.Kind != ggui.ActionPress {
		return false
	}
	a.onTap()
	return true
}

// HandlePointer implements ggui.PointerHandler.
func (a *comboboxAddon) HandlePointer(ev ggui.PointerEvent) bool { return a.Pointer(ev, a.onTap) }

func (a *comboboxAddon) Paint(dst *ggui.Canvas, r ggui.Rect) {
	t := a.theme
	dst.Describe(r, a)
	ink := t.MutedFg
	if a.inert {
		ink = t.Disabled(ink)
	} else {
		dst.HitPointer(r, a)
		dst.HitCursor(r, ggui.CursorShapePointer)
		if a.Hovered {
			// The ghost button's hover, with the cross brought up to the
			// foreground; the chevron stays muted.
			var hover color.Color = t.Muted
			if isDark(t.Bg) {
				hover = fade(hover, .5)
			}
			dst.FillRoundRect(r, max(t.Radius-3, 0), hover)
			if !a.chevron {
				ink = t.Fg
			}
		}
	}
	size := pick(a.chevron, t.IconSize, t.IconSize*.875)
	paintIconAt(dst, a.env, a.icon, r.Center(), size, ink, 0)
}

// ComboboxMultipleWidget is a field of removable chips, one per selected
// value, followed by an input that filters a list of the options, with a
// check on each selected one. Selection writes fresh slices and never
// mutates the caller's slice. Values absent from Options remain visible
// as removable chips.
type ComboboxMultipleWidget[T comparable] struct {
	props       property.Owner
	values      ggui.Binding[[]T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	input       *ggui.TextInputWidget // after the chips, filtering the list
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

// ComboboxMultiple creates a searchable multiple selection control, as
// shadcn's ComboboxChips with a ComboboxChipsInput: typing in the input
// after the chips filters the list and opens it, and the keyboard stays in
// the input. Enter toggles the highlighted result without closing;
// Up/Down move through matches. Backspace in an empty input removes the
// last chip, Escape closes the list, then empties the input, and leaving
// the input empties it too.
func ComboboxMultiple[T comparable](values ggui.Binding[[]T]) *ComboboxMultipleWidget[T] {
	property.Require(values, "ComboboxMultiple")
	c := &ComboboxMultipleWidget[T]{values: values, label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.input = ggui.TextInput(c.query).Placeholder(c.placeholder).MinWidth(0)
	c.input.OnKey(c.handleKey).OnChange(func(string) { c.open() }).OnCommit(func(string) {
		// Leaving the input closes the list and drops what was typed.
		if !c.input.Focused() {
			c.popup.SetOpen(false)
			c.query.Set("")
		}
	})
	c.input.OnDescribe(c.describe).OnAction(c.Act)
	c.search = Command(c.query).Name("Search options").Height(comboboxListHeight).Empty("No items found.")
	c.search.menuRows, c.search.keepOpen, c.search.bare = true, true, true
	c.field = &comboboxChips[T]{c: c}
	c.content = ggui.Box(c.search)
	c.popup = ggui.Popup(c.field, menuSurface(&comboboxList{input: c.input, content: c.content})).
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
	c.placeholder = v
	c.input.Placeholder(v)
	return c
}

// Empty sets what the list says when nothing matches what was typed; "No
// items found." by default.
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
func (c *ComboboxMultipleWidget[T]) Popup() *ggui.PopupWidget { return c.popup }

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
		n.Name = "Choose options"
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
	return ggui.RoleCombobox, "Choose options"
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
	fieldSurface(dst, at, t, r, t.Radius, fieldState{focused, f.invalid, inert}, f.motion, JoinedSides{})
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
