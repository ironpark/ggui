package ui

import (
	"slices"

	"github.com/ironpark/ggui"
)

// ComboboxWidget is a searchable selection popup. Keep it mounted to retain its query.
type ComboboxWidget[T comparable] struct {
	value       ggui.Binding[T]
	options     []T
	optionsWhen ggui.Readable[[]T]
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
func Combobox[T comparable](value ggui.Binding[T], options []T) *ComboboxWidget[T] {
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
	c.search = Command(c.query).Named("Search options").Placeholder("Search options…")
	c.popup = ggui.Popup(c.button, ggui.Box(c.search).Width(280)).Owner(c.button)
	c.button.Expands(c.popup.IsOpen)
	return c.Options(options)
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
// and removes any OptionsWhen binding. The popup and search editor stay
// mounted: open state and query are preserved, matches are refreshed and
// the first match is highlighted. The value is never written and OnChange
// is not called; an absent value displays Placeholder.
func (c *ComboboxWidget[T]) Options(options []T) *ComboboxWidget[T] {
	wasBound := c.optionsWhen != nil
	c.optionsWhen = nil
	if c.setOptions(options) || wasBound {
		ggui.Invalidate(c.env)
	}
	return c
}

// OptionsWhen follows r at layout without rebuilding the control. Changes
// use the same snapshot and selection rules as Options. The last setting
// wins; nil stops following and keeps the current snapshot.
func (c *ComboboxWidget[T]) OptionsWhen(r ggui.Readable[[]T]) *ComboboxWidget[T] {
	if sameReadable(c.optionsWhen, r) {
		return c
	}
	c.optionsWhen = r
	ggui.Invalidate(c.env)
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
		c.search.items[i].text.Set(label)
		c.search.items[i].Name = label
	}
	c.search.initialized = false
	return c
}

// Named names the trigger; the selected value remains visible on it.
func (c *ComboboxWidget[T]) Named(s string) *ComboboxWidget[T] { c.button.Name = s; return c }

// Placeholder sets the trigger text when value is not in options.
func (c *ComboboxWidget[T]) Placeholder(s string) *ComboboxWidget[T] { c.placeholder = s; return c }

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
	c.env = env
	if c.optionsWhen != nil {
		c.setOptions(c.optionsWhen.Get())
	}
	c.button.Sync()
	if c.button.Inert {
		c.popup.Hide()
	}
	label := c.placeholder
	for _, v := range c.options {
		if v == ggui.Untrack(c.value.Get) {
			label = c.label(v)
			break
		}
	}
	c.button.label.Set(label + "  ▾")
	return c.popup.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ComboboxWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }

// SetName names the trigger for Field.
func (c *ComboboxWidget[T]) SetName(s string) { c.Named(s) }

// HasName reports whether the trigger has an explicit name.
func (c *ComboboxWidget[T]) HasName() bool { return c.button.HasName() }

// Semantics reports the trigger's role and resolved name.
func (c *ComboboxWidget[T]) Semantics() (ggui.Role, string) { return c.button.Semantics() }

// DisabledWhen follows r and closes the popup while disabled.
func (c *ComboboxWidget[T]) DisabledWhen(r ggui.Readable[bool]) *ComboboxWidget[T] {
	c.button.DisabledWhen(r)
	return c
}
