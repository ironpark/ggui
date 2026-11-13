package ui

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// The toggle variants of shadcn/ui, which Toggle and ToggleGroup draw
// with. They differ from the ghost and outline buttons on a dark page: a
// toggle hovers on the whole Muted surface, and its outline is the
// InputBorder around nothing. Restyle one to change every toggle under a
// theme.
var (
	// ToggleDefault has no surface until the pointer is over it or it is on.
	ToggleDefault = NewVariant("toggle", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Hover: t.Muted, Label: t.Fg, HoverLabel: t.Fg}
	})
	// ToggleOutline adds the InputBorder around ToggleDefault.
	ToggleOutline = NewVariant("toggle-outline", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Hover: t.Muted, Label: t.Fg, HoverLabel: t.Fg, Border: t.InputBorder}
	})
)

// ToggleWidget is a button whose pressed state is controlled by a binding.
// Use ToggleGroupMultiple when several independent choices belong together.
//
// It is at least as wide as it is tall, so one holding a single icon is
// square, and unlike a Button it does not move while it is held down.
type ToggleWidget struct {
	*ButtonWidget
	pressed  ggui.Binding[bool]
	onChange func(bool)
}

// Toggle creates a two-state button. Space, Enter and accessibility activation
// change pressed; disabled controls preserve the binding.
func Toggle(pressed ggui.Binding[bool], label string) *ToggleWidget {
	t := &ToggleWidget{pressed: pressed}
	t.ButtonWidget = Button(label, t.flip)
	return t.init()
}

// ToggleOf creates a Toggle around any content instead of a text label,
// most often one icon. Give it a name with Name, since nothing on it says
// what it is.
//
//	ui.ToggleOf(bold, ui.Icon(icons.Bold)).Name("Bold")
func ToggleOf(pressed ggui.Binding[bool], child ggui.Widget) *ToggleWidget {
	t := &ToggleWidget{pressed: pressed}
	t.ButtonWidget = ButtonOf(child, t.flip)
	return t.init()
}

func (t *ToggleWidget) init() *ToggleWidget {
	t.ButtonWidget.Variant(ToggleDefault)
	t.toggleValue = func() bool { return ggui.Untrack(t.pressed.Get) }
	return t
}

func (t *ToggleWidget) flip() {
	setChanged(t.pressed, !ggui.Untrack(t.pressed.Get), t.onChange)
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

// Invalid marks the toggle as failing validation: a Destructive border in
// the outline variant, and a Destructive ring while it is focused.
func (t *ToggleWidget) Invalid(v bool) *ToggleWidget { t.ButtonWidget.Invalid(v); return t }

// BindInvalid follows r for Invalid without a rebuild.
func (t *ToggleWidget) BindInvalid(r ggui.Readable[bool]) *ToggleWidget {
	t.ButtonWidget.BindInvalid(r)
	return t
}

// Outline draws the toggle in ToggleOutline: a border around it, on or off.
func (t *ToggleWidget) Outline() *ToggleWidget { t.ButtonWidget.Variant(ToggleOutline); return t }

// Variant draws the toggle in v: ToggleDefault, ToggleOutline or a variant
// of your own.
func (t *ToggleWidget) Variant(v *ButtonVariant) *ToggleWidget { t.ButtonWidget.Variant(v); return t }

// Size selects the same size variants as Button; shadcn/ui's toggle has
// ButtonSmall, ButtonDefault and ButtonLarge.
func (t *ToggleWidget) Size(size *ButtonSizeVariant) *ToggleWidget {
	t.ButtonWidget.Size(size)
	return t
}

// Icon puts an icon before the label; see ButtonWidget.Icon.
func (t *ToggleWidget) Icon(icon *icons.Widget) *ToggleWidget { t.ButtonWidget.Icon(icon); return t }

// TrailingIcon puts an icon after the label; see ButtonWidget.TrailingIcon.
func (t *ToggleWidget) TrailingIcon(icon *icons.Widget) *ToggleWidget {
	t.ButtonWidget.TrailingIcon(icon)
	return t
}

// BindName follows an accessible-name reader.
func (t *ToggleWidget) BindName(name ggui.Readable[string]) *ToggleWidget {
	t.ButtonWidget.BindName(name)
	return t
}
