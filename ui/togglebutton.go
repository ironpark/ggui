package ui

import "github.com/ironpark/ggui"

// ToggleWidget is a button whose pressed state is controlled by a binding.
// Use ToggleGroupMultiple when several independent choices belong together.
type ToggleWidget struct {
	*ButtonWidget
	pressed  ggui.Binding[bool]
	onChange func(bool)
}

// Toggle creates a two-state button. Space, Enter and accessibility activation
// change pressed; disabled controls preserve the binding.
func Toggle(pressed ggui.Binding[bool], label string) *ToggleWidget {
	t := &ToggleWidget{pressed: pressed}
	t.ButtonWidget = Button(label, func() { setChanged(pressed, !ggui.Untrack(pressed.Get), t.onChange) }).Ghost()
	t.toggleValue = func() bool { return ggui.Untrack(pressed.Get) }
	return t
}

// OnChange runs after the pressed state changes.
func (t *ToggleWidget) OnChange(fn func(bool)) *ToggleWidget { t.onChange = fn; return t }

// Name names the button for accessibility and testing.
func (t *ToggleWidget) Name(name string) *ToggleWidget { t.ButtonWidget.Name(name); return t }

// Key assigns an identity that survives rebuilding.
func (t *ToggleWidget) Key(key any) *ToggleWidget { t.ButtonWidget.Key(key); return t }

// Disabled prevents activation while disabled is true.
func (t *ToggleWidget) Disabled(disabled bool) *ToggleWidget {
	t.ButtonWidget.Disabled(disabled)
	return t
}

// BindDisabled follows a disabled-state reader.
func (t *ToggleWidget) BindDisabled(r ggui.Readable[bool]) *ToggleWidget {
	t.ButtonWidget.BindDisabled(r)
	return t
}

// Outline adds an outline to the unpressed state.
func (t *ToggleWidget) Outline() *ToggleWidget { t.ButtonWidget.Outline(); return t }

// Size selects the same size variants as Button.
func (t *ToggleWidget) Size(size *ButtonSizeVariant) *ToggleWidget {
	t.ButtonWidget.Size(size)
	return t
}

// BindName follows an accessible-name reader.
func (t *ToggleWidget) BindName(name ggui.Readable[string]) *ToggleWidget {
	t.ButtonWidget.BindName(name)
	return t
}
