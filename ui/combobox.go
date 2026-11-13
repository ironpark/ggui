package ui

import (
	"slices"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// ComboboxWidget is a searchable selection popup. Keep it mounted to retain its query.
type ComboboxWidget[T comparable] struct {
	props       property.Owner
	value       ggui.Binding[T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	button      *ButtonWidget
	popup       *ggui.PopupWidget
	search      *CommandWidget
	query       *ggui.StateValue[string]
	onChange    func(T)
	env         ggui.Env
}

// Combobox creates a closed dropdown; its popup focuses a search field and
// supports typing, Up/Down, Enter, pointer selection and Escape cancellation.
// The options slice is shallow-copied.
func Combobox[T comparable](value ggui.Binding[T]) *ComboboxWidget[T] {
	c := &ComboboxWidget[T]{value: value, label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.button = Button("", func() {
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
	if c.button.HitID() == nil {
		c.button.Key(c)
	}
	c.search = Command(c.query).Name("Search options").Placeholder("Search options…")
	c.popup = ggui.Popup(c.button, ggui.Box(c.search).Width(280)).Owner(c.button)
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
	label := c.placeholder
	for _, v := range c.options {
		if v == ggui.Untrack(c.value.Get) {
			label = c.label(v)
			break
		}
	}
	c.button.label.Content(label + "  ▾")
	return c.popup.Layout(cs, env)
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

// ComboboxMultipleWidget combines a searchable checkbox popup with removable
// selected-value chips. Selection writes fresh slices and never mutates the
// caller's slice. Values absent from Options remain visible as removable chips.
type ComboboxMultipleWidget[T comparable] struct {
	props       property.Owner
	values      ggui.Binding[[]T]
	options     []T
	optionProp  property.Value[[]T]
	label       func(T) string
	placeholder string
	button      *ButtonWidget
	popup       *ggui.PopupWidget
	query       *ggui.StateValue[string]
	field       *TextFieldWidget
	rows        *comboboxMultipleRows[T]
	scroll      *ggui.ScrollWidget
	panel       *ggui.BoxWidget
	root        *ggui.WrapWidget
	chips       []*ButtonWidget
	shown       []T
	onChange    func([]T)
	key         any
}

// ComboboxMultiple creates a searchable multiple selection control. Enter
// toggles the highlighted result without closing; Up/Down move through matches.
// Empty-search Backspace removes the last chip, and Escape dismisses the popup.
func ComboboxMultiple[T comparable](values ggui.Binding[[]T]) *ComboboxMultipleWidget[T] {
	property.Require(values, "ComboboxMultiple")
	c := &ComboboxMultipleWidget[T]{values: values, label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.button = Button("", func() {
		if c.popup.IsOpen() {
			c.popup.Hide()
			return
		}
		c.query.Set("")
		c.rows.highlight = 0
		c.popup.Show()
	}).Outline()
	c.button.Role = ggui.RoleCombobox
	c.button.defaultName = "Choose options"
	c.button.value = func() string {
		labels := make([]string, 0, len(ggui.Untrack(values.Get)))
		for _, v := range ggui.Untrack(values.Get) {
			labels = append(labels, c.label(v))
		}
		return strings.Join(labels, ", ")
	}
	c.field = TextField(c.query).Name("Search options").Placeholder("Search options…").OnKey(c.handleKey)
	c.rows = &comboboxMultipleRows[T]{owner: c}
	c.scroll = ggui.Scroll(c.rows).Shrink()
	c.panel = ggui.Box(ggui.Column(c.field, ggui.Box(c.scroll).MaxHeight(200)).Gap(4).Align(ggui.AlignStretch)).Width(280)
	c.popup = ggui.Popup(c.button, c.panel).Owner(c.button).Modal(false)
	c.button.Expands(c.popup.IsOpen).Opens(c)
	return c
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
	c.root = nil
	c.rows.checks = nil
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

// Placeholder labels the add-selection trigger.
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
	c.field.Key(comboboxKey{"search", k})
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
func (c *ComboboxMultipleWidget[T]) handleKey(e ggui.KeyEvent) bool {
	if c.button.IsInert() || e.Kind != ggui.KeyPress || e.Mods != (ggui.Mods{}) {
		return false
	}
	switch e.Key {
	case ggui.KeyBackspace:
		values := ggui.Untrack(c.values.Get)
		if ggui.Untrack(c.query.Get) == "" && len(values) > 0 {
			c.selectValue(values[len(values)-1], false)
			return true
		}
	case ggui.KeyArrowDown, ggui.KeyArrowUp:
		if len(c.rows.matches) > 0 {
			dir := pick(e.Key == ggui.KeyArrowDown, 1, -1)
			c.rows.highlight = max(0, min(c.rows.highlight+dir, len(c.rows.matches)-1))
			c.rows.reveal = true
		}
		return true
	case ggui.KeyEnter, ggui.KeyNumpadEnter:
		if at := c.rows.highlight; at >= 0 && at < len(c.rows.matches) {
			v := c.options[c.rows.matches[at]]
			c.selectValue(v, !slices.Contains(ggui.Untrack(c.values.Get), v))
		}
		return true
	}
	return false
}

// Layout implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	defer c.props.Layout().EndLayout()
	c.button.Sync()
	options := c.optionProp.Get()
	if !slices.Equal(c.options, options) || c.rows.checks == nil {
		c.options = slices.Clone(options)
		c.rows.checks = make([]*CheckboxWidget, len(options))
		c.rows.labels = make([]string, len(options))
		for j, v := range options {
			c.rows.labels[j] = strings.ToLower(c.label(v))
			binding := ggui.Bind(func() bool { return slices.Contains(c.values.Get(), v) }, func(on bool) { c.selectValue(v, on) })
			c.rows.checks[j] = Checkbox(binding, c.label(v))
		}
	}
	values := c.values.Get()
	if c.root == nil || !slices.Equal(values, c.shown) {
		c.shown = slices.Clone(values)
		c.chips = nil
		children := make([]ggui.Widget, 0, len(values)+1)
		for j, v := range values {
			chip := Button(c.label(v)+" ×", func() { c.selectValue(v, false) }).Secondary().Size(ButtonExtraSmall).Name("Remove " + c.label(v))
			if c.key != nil {
				chip.Key(childKey{owner: c.key, part: j})
			}
			c.chips = append(c.chips, chip)
			children = append(children, chip)
		}
		children = append(children, c.popup)
		c.root = ggui.Wrap(children...).Gap(4).RunGap(4)
	}
	for _, chip := range c.chips {
		chip.Disabled(c.button.IsInert())
	}
	for _, check := range c.rows.checks {
		check.Disabled(c.button.IsInert())
	}
	if c.button.IsInert() {
		c.popup.Hide()
	}
	c.button.label.Content(c.placeholder + "  ▾")
	panelBox(c.panel, uitheme.Of(env))
	return c.root.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ComboboxMultipleWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.root, r) }

type comboboxMultipleRows[T comparable] struct {
	owner     *ComboboxMultipleWidget[T]
	checks    []*CheckboxWidget
	labels    []string // each option's label, lower-cased for the search
	matches   []int
	sizes     []ggui.Size
	highlight int
	query     string
	reveal    bool
	theme     *uitheme.Theme
	empty     *ggui.TextWidget
}

func (r *comboboxMultipleRows[T]) Layout(cs ggui.Constraints, env ggui.Env) ggui.Size {
	r.theme = uitheme.Of(env)
	query := strings.ToLower(strings.TrimSpace(r.owner.query.Get()))
	if query != r.query {
		r.highlight = 0
		r.query = query
	}
	r.matches, r.sizes = r.matches[:0], r.sizes[:0]
	w, h := 0.0, 0.0
	for j := range r.owner.options {
		if !strings.Contains(r.labels[j], query) {
			continue
		}
		size := r.checks[j].Layout(ggui.Loose(ggui.Sz(max(0, cs.MaxW-12), ggui.Unbounded)), env)
		size.W += 12
		size.H += 8
		r.matches = append(r.matches, j)
		r.sizes = append(r.sizes, size)
		w, h = max(w, size.W), h+size.H
	}
	r.highlight = max(0, min(r.highlight, len(r.matches)-1))
	if len(r.matches) == 0 {
		if r.empty == nil {
			r.empty = ggui.Text("No results")
		}
		size := r.empty.Color(r.theme.MutedFg).Layout(cs, env)
		return cs.Constrain(size)
	}
	return cs.Constrain(ggui.Sz(w, h))
}
func (r *comboboxMultipleRows[T]) Paint(dst *ggui.Canvas, rect ggui.Rect) {
	if len(r.matches) == 0 {
		dst.Paint(r.empty, rect)
		return
	}
	y := rect.Origin.Y
	for j, index := range r.matches {
		row := ggui.Rct(ggui.Pt(rect.Origin.X, y), ggui.Sz(rect.Size.W, r.sizes[j].H))
		if j == r.highlight {
			dst.FillRoundRect(row, r.theme.RadiusSm, r.theme.Accent)
		}
		inner := ggui.Rct(row.Origin.Add(ggui.Pt(6, 4)), ggui.Sz(max(0, row.Size.W-12), max(0, row.Size.H-8)))
		dst.Paint(r.checks[index], inner)
		if r.reveal && j == r.highlight {
			dst.RequestReveal(row)
		}
		y += row.Size.H
	}
	r.reveal = false
}
