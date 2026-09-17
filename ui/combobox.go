package ui

import "github.com/ironpark/ggui"

// ComboboxWidget is a searchable selection popup. Keep it mounted to retain its query.
type ComboboxWidget[T comparable] struct {
	value       ggui.Binding[T]
	options     []T
	label       func(T) string
	placeholder string
	button      *ButtonWidget
	popup       *ggui.PopupWidget
	search      *CommandWidget
	query       *ggui.Signal[string]
	onChange    func(T)
}

// Combobox creates a closed dropdown; its popup focuses a search field and
// supports typing, Up/Down, Enter, pointer selection and Escape cancellation.
func Combobox[T comparable](value ggui.Binding[T], options []T) *ComboboxWidget[T] {
	c := &ComboboxWidget[T]{value: value, options: append([]T(nil), options...), label: sprint[T], placeholder: "Select…", query: ggui.State("")}
	c.button = Button("", func() {
		if c.popup.IsOpen() {
			c.popup.Hide()
		} else {
			c.query.Set("")
			c.search.initialized = false
			c.search.filter()
			for i, v := range c.options {
				if v == c.value.Peek() {
					c.search.highlight = i
					break
				}
			}
			c.popup.Show()
		}
	}).Secondary()
	c.button.Role = ggui.RoleCombobox
	if c.button.HitID() == nil {
		c.button.Key(c)
	}
	entries := make([]CommandEntry, len(options))
	for i, v := range options {
		entries[i] = CommandItem(c.label(v), func() { setChanged(c.value, v, c.onChange); c.popup.Hide() })
	}
	c.search = Command(c.query, entries...).Label("Search options").Placeholder("Search options…")
	c.popup = ggui.Popup(c.button, ggui.Box(c.search).Width(280)).Owner(c.button)
	c.button.Expands(c.popup.IsOpen)
	return c
}

// Label formats the options and selected value. Configure it before layout.
func (c *ComboboxWidget[T]) Label(fn func(T) string) *ComboboxWidget[T] {
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
	label := c.placeholder
	for _, v := range c.options {
		if v == c.value.Peek() {
			label = c.label(v)
			break
		}
	}
	c.button.label.Set(label + "  ▾")
	if c.button.Name == "" {
		c.button.Name = "Choose option"
	}
	return c.popup.Layout(cs, env)
}

// Paint implements ggui.Widget.
func (c *ComboboxWidget[T]) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(c.popup, r) }
