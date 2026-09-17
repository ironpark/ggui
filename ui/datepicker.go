package ui

import (
	"time"

	"github.com/ironpark/ggui"
)

// DatePickerWidget opens a Calendar in a focus-trapped popup.
type DatePickerWidget struct {
	value       ggui.Binding[time.Time]
	calendar    *CalendarWidget
	button      *ButtonWidget
	popup       *ggui.PopupWidget
	panel       *ggui.BoxWidget
	format      func(time.Time) string
	placeholder string
}

// DatePicker selects a single date. Zero displays "Choose date".
func DatePicker(value ggui.Binding[time.Time]) *DatePickerWidget {
	d := &DatePickerWidget{value: value, placeholder: "Choose date", format: func(t time.Time) string { return t.Format("2006-01-02") }}
	d.calendar = Calendar(value)
	d.button = Button("", func() { d.popup.Toggle() }).Outline()
	d.button.defaultName = "Choose date"
	d.button.value = func() string {
		if v := d.value.Peek(); !v.IsZero() {
			return d.format(d.calendar.date(v))
		}
		return ""
	}
	d.panel = ggui.Box(d.calendar)
	d.popup = ggui.Popup(d.button, d.panel).Owner(d.button)
	d.button.Expands(d.popup.IsOpen).Opens(d)
	d.calendar.onPick = d.popup.Hide
	return d
}

// datePickerKey keeps the trigger, calendar and popup keys distinct while all
// three derive from the one key the caller gave.
type datePickerKey struct {
	part string
	id   any
}

// Calendar exposes date bounds, localization, week start and change callbacks.
// DatePicker owns its disabled state; configure Disabled/DisabledWhen on the picker.
func (d *DatePickerWidget) Calendar() *CalendarWidget { return d.calendar }

// Popup exposes the popup's open state.
func (d *DatePickerWidget) Popup() *ggui.PopupWidget { return d.popup }

// Key preserves trigger focus, popup state and the browsed month across rebuilds.
func (d *DatePickerWidget) Key(key any) *DatePickerWidget {
	d.button.Key(datePickerKey{"trigger", key})
	d.calendar.Key(datePickerKey{"calendar", key})
	d.popup.Key(datePickerKey{"popup", key})
	return d
}

// Named names the trigger for accessibility and Probe.
func (d *DatePickerWidget) Named(s string) *DatePickerWidget {
	d.button.Named(s)
	d.calendar.Named(s)
	return d
}

// Placeholder sets the text shown for a zero date.
func (d *DatePickerWidget) Placeholder(s string) *DatePickerWidget { d.placeholder = s; return d }

// Format customizes the selected date's display.
func (d *DatePickerWidget) Format(fn func(time.Time) string) *DatePickerWidget {
	if fn != nil {
		d.format = fn
	}
	return d
}

// OnChange reports user selections of a different date.
func (d *DatePickerWidget) OnChange(fn func(time.Time)) *DatePickerWidget {
	d.calendar.OnChange(fn)
	return d
}

// Disabled closes and disables the picker.
func (d *DatePickerWidget) Disabled(v bool) *DatePickerWidget {
	d.button.Disabled(v)
	d.calendar.Disabled(v)
	if v {
		d.popup.Hide()
	}
	return d
}

// Act implements ggui.Actor for the trigger's expand/collapse actions.
func (d *DatePickerWidget) Act(a ggui.Action) bool {
	if d.button.Inert {
		return false
	}
	switch a.Kind {
	case ggui.ActionExpand:
		d.popup.Show()
	case ggui.ActionCollapse:
		d.popup.Hide()
	default:
		return false
	}
	return true
}

// Layout implements ggui.Widget.
func (d *DatePickerWidget) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	d.button.Sync()
	if d.button.Inert {
		d.popup.Hide()
	}
	d.calendar.Disabled(d.button.Inert)
	t := env.Theme()
	panelBox(d.panel, t)
	text := d.placeholder
	if v := d.value.Get(); !v.IsZero() {
		text = d.format(d.calendar.date(v))
	}
	d.button.label.Set(text)
	return d.popup.Layout(c, env)
}

// Paint implements ggui.Widget.
func (d *DatePickerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.popup, r) }

// SetName names the trigger for Field.
func (d *DatePickerWidget) SetName(s string) { d.Named(s) }

// HasName reports whether the trigger has an explicit name.
func (d *DatePickerWidget) HasName() bool { return d.button.HasName() }

// Semantics reports the trigger's role and resolved name.
func (d *DatePickerWidget) Semantics() (ggui.Role, string) { return d.button.Semantics() }

// DisabledWhen follows r and closes the popup while disabled.
func (d *DatePickerWidget) DisabledWhen(r ggui.Reader[bool]) *DatePickerWidget {
	d.button.DisabledWhen(r)
	return d
}
