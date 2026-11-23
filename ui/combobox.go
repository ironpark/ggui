package ui

import (
	"image/color"
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/property"
	"github.com/ironpark/ggui/ui/icons"
	"github.com/ironpark/ggui/ui/internal/controlkit"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

// comboboxListHeight is the most a combobox's rows take before its list
// scrolls: shadcn's 288 less the 36 it keeps for a search, 252, less the
// padding around the rows.
const comboboxListHeight = 244

// ComboboxWidget is a text field that filters a list of options as it is
// typed in, and chooses one of them. Keep it mounted to retain what was
// typed.
type ComboboxWidget[T comparable] struct {
	props          property.Owner
	value          ggui.Binding[T]
	options        []T
	optionProp     property.Value[[]T]
	label          func(T) string
	placeholder    string                   // as Placeholder set it
	placeholderSet bool                     // Placeholder was called; otherwise the Strings give it
	words          *Strings                 // the Strings of the last layout
	text           *ggui.StateValue[string] // what the input shows
	query          *ggui.StateValue[string] // what the list is filtered by
	typed          bool                     // text was typed since a value was last shown in the input
	input          *widget.TextInputWidget
	field          *comboboxField
	chevron        *comboboxAddon
	clear          *comboboxAddon
	clearable      bool
	content        *widget.BoxWidget // the list, as wide as the field
	popup          *widget.PopupWidget
	search         *CommandWidget
	onChange       func(T)
}

// Combobox creates a field holding an editable input and a muted chevron,
// as shadcn's ComboboxInput is. Typing filters a list of the options that
// opens below the field, a little wider than it, with a check on the chosen one; the
// list also opens on Up or Down, on a click in the input and on the
// chevron. The keyboard stays in the input throughout: Up and Down move
// the highlight, Enter chooses the highlighted option, Escape closes the
// list and then puts back what was typed over, and Tab moves on. Choosing
// puts the option's label in the input; leaving the input after typing
// puts back the chosen option's label. The options slice is
// shallow-copied.
func Combobox[T comparable](value ggui.Binding[T]) *ComboboxWidget[T] {
	c := &ComboboxWidget[T]{value: value, label: sprint[T], text: ggui.State(""), query: ggui.State("")}
	c.input = widget.TextInput(c.text).MinWidth(0)
	c.input.OnKey(c.key).OnChange(c.typing).OnCommit(func(string) {
		if !c.input.Focused() {
			c.left()
		}
	})
	c.input.OnDescribe(c.describe).OnAction(c.act)
	c.chevron = &comboboxAddon{icon: icons.ChevronDown, onTap: c.toggle, chevron: true}
	c.chevron.Role = ggui.RoleButton
	c.chevron.expands = func() bool { return c.popup.IsOpen() }
	c.clear = &comboboxAddon{icon: icons.Close, onTap: c.clearValue}
	c.clear.Role = ggui.RoleButton
	c.field = &comboboxField{input: c.input, open: c.toggleFromInput}
	c.search = Command(c.query).Height(comboboxListHeight)
	c.search.menuRows, c.search.bare = true, true
	c.content = widget.Box(c.search)
	c.popup = widget.Popup(c.field, menuSurface(&comboboxList{input: c.input, content: c.content})).
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

// equalsReader and containsReader are stateless Readable[bool] adapters that
// check membership on demand without creating owning reactive subscriptions.
type equalsReader[T comparable] struct {
	r ggui.Readable[T]
	v T
}

func (e equalsReader[T]) Get() bool { return e.r.Get() == e.v }

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
		c.search.items[i].BindChecked(equalsReader[T]{r: c.value, v: v})
	}
	return true
}

// Format formats the options and selected value. Configure it before layout.
func (c *ComboboxWidget[T]) Format(fn func(T) string) *ComboboxWidget[T] {
	if fn == nil {
		fn = sprint[T]
	}
	c.label = fn
	for i, v := range c.options {
		label := fn(v)
		c.search.entries[i].label = label
		c.search.items[i].text.Content(label)
		c.search.items[i].SetName(label)
	}
	c.search.initialized = false
	c.props.Changed()
	return c
}

// Name names the input; what it holds is its value.
func (c *ComboboxWidget[T]) Name(s string) *ComboboxWidget[T] { c.input.Name(s); return c }

// Placeholder sets the muted text the input shows while it is empty: when
// the value is not among the options and nothing is typed.
func (c *ComboboxWidget[T]) Placeholder(s string) *ComboboxWidget[T] {
	defer property.Watch(&c.props, &c.placeholder)()
	c.placeholder, c.placeholderSet = s, true
	c.input.Placeholder(s)
	return c
}

// Empty sets what the list says when nothing matches what was typed; the
// Strings' NoItemsFound, "No items found.", by default.
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
func (c *ComboboxWidget[T]) Popup() *widget.PopupWidget { return c.popup }

// chosen returns the label of the value when it is among the options.
func (c *ComboboxWidget[T]) chosen() (string, bool) {
	return c.chosenValue(ggui.Untrack(c.value.Get))
}

func (c *ComboboxWidget[T]) chosenValue(v T) (string, bool) {
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
		n.Name = c.words.orDefault().ChooseOption
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
	c.words = stringsOf(env)
	if !c.placeholderSet {
		c.input.Placeholder(c.words.ComboboxPlaceholder)
	}
	c.chevron.SetName(c.words.ShowOptions)
	c.clear.SetName(c.words.ClearValue)
	c.setOptions(c.optionProp.Get())
	c.field.Sync()
	if c.field.IsInert() {
		c.popup.Hide()
	}
	label, chosen := c.chosenValue(c.value.Get())
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
	// The list is three and a half of the theme's Space wider than the
	// field and starts at its leading edge: shadcn's
	// min-w-[calc(var(--anchor-width)+--spacing(7))], which the chips of a
	// ComboboxMultiple do not take.
	c.content.Width(size.W + t.Space*3.5)
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
	return ggui.RoleCombobox, c.words.orDefault().ChooseOption
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
	input            *widget.TextInputWidget
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
	button := buttonExtraSmall(t).Height
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
	fieldSurface(dst, at, r, fieldLook{theme: t, focused: f.input.Focused(), invalid: f.invalid, inert: inert, motion: f.motion, sides: f.sides.within(dst)})
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
	input   *widget.TextInputWidget
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
		dst.HitCursor(r, ggui.CursorShapeDefault)
		if a.Hovered {
			// The ghost button's hover, with the cross brought up to the
			// foreground; the chevron stays muted.
			var hover color.Color = t.Muted
			if controlkit.IsDark(t.Bg) {
				hover = fade(hover, .5)
			}
			dst.FillRoundRect(r, max(t.Radius-3, 0), hover)
			if !a.chevron {
				ink = t.Fg
			}
		}
	}
	size := pick(a.chevron, t.IconSize, t.IconSize*.875)
	controlkit.PaintIconAt(dst, a.env, controlkit.IconMark{Role: a.icon, Center: r.Center(), Size: size, Color: ink})
}
