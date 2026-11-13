package ui

import (
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// DatePickerWidget opens a Calendar in a focus-trapped popup. Build one with
// DatePicker for a single date or DateRangePicker for a range.
type DatePickerWidget struct {
	props       property.Owner
	value       ggui.Binding[time.Time]
	rangeValue  ggui.Binding[DateRange]
	formatRange func(DateRange) string
	calendar    *CalendarWidget
	button      *ButtonWidget
	popup       *ggui.PopupWidget
	panel       *ggui.BoxWidget
	format      func(time.Time) string
	placeholder string
}

// DatePicker selects a single date and closes once one is picked. Zero
// displays "Choose date".
func DatePicker(value ggui.Binding[time.Time]) *DatePickerWidget {
	d := newDatePicker(Calendar(value), "Choose date")
	d.value = value
	d.calendar.onPick = d.popup.Hide
	return d
}

// DateRangePicker selects an inclusive range of dates from a CalendarRange
// showing two months. Its popup stays open while the range is picked;
// Escape or a click outside closes it. An empty range displays "Choose
// dates". Configure the range's limits on Calendar.
func DateRangePicker(value ggui.Binding[DateRange]) *DatePickerWidget {
	d := newDatePicker(CalendarRange(value).Months(2), "Choose dates")
	d.rangeValue = value
	d.formatRange = func(r DateRange) string {
		if r.End.IsZero() || r.End.Equal(r.Start) {
			return d.format(r.Start)
		}
		return d.format(r.Start) + " – " + d.format(r.End)
	}
	return d
}

func newDatePicker(calendar *CalendarWidget, placeholder string) *DatePickerWidget {
	d := &DatePickerWidget{calendar: calendar, placeholder: placeholder, format: func(t time.Time) string { return t.Format("2006-01-02") }}
	d.button = Button("", func() { d.popup.Toggle() }).Outline()
	d.button.defaultName = placeholder
	d.button.value = func() string { return ggui.Untrack(d.text) }
	// The panel is as wide as its calendar, as shadcn's w-auto popover is,
	// and lines up with the trigger's start.
	d.panel = ggui.Box(calendar)
	d.popup = ggui.Popup(d.button, d.panel).Owner(d.button).MatchAnchorWidth(false)
	d.button.Expands(d.popup.IsOpen).Opens(d)
	return d
}

// text is the selection as the trigger shows it, or "" for none. It reads
// the binding in the caller's tracking scope.
func (d *DatePickerWidget) text() string {
	if d.rangeValue != nil {
		if r := d.calendar.normalizedRange(d.rangeValue.Get()); !r.Start.IsZero() {
			return d.formatRange(r)
		}
		return ""
	}
	if v := d.value.Get(); !v.IsZero() {
		return d.format(d.calendar.date(v))
	}
	return ""
}

// datePickerKey keeps the trigger, calendar and popup keys distinct while all
// three derive from the one key the caller gave.
type datePickerKey struct {
	part string
	id   any
}

// Calendar exposes date bounds, localization, week start and change callbacks.
// DatePicker owns its disabled state; configure Disabled/BindDisabled on the picker.
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

// Name names the trigger for accessibility and Probe.
func (d *DatePickerWidget) Name(s string) *DatePickerWidget {
	d.button.Name(s)
	d.calendar.Name(s)
	return d
}

// Placeholder sets the text shown for a zero date.
func (d *DatePickerWidget) Placeholder(s string) *DatePickerWidget {
	defer property.Watch(&d.props, &d.placeholder)()
	d.placeholder = s
	return d
}

// Format customizes the selected date's display.
func (d *DatePickerWidget) Format(fn func(time.Time) string) *DatePickerWidget {
	if fn != nil {
		d.format = fn
	}
	return d
}

// FormatRange customizes how a DateRangePicker displays its range; nil keeps
// the current formatter. The range it is given is normalized: Start is set,
// and End is zero while the range is unfinished. Without it each end is
// shown through Format.
func (d *DatePickerWidget) FormatRange(fn func(DateRange) string) *DatePickerWidget {
	if fn != nil {
		d.formatRange = fn
		d.props.Changed()
	}
	return d
}

// OnChange reports user selections of a different date.
func (d *DatePickerWidget) OnChange(fn func(time.Time)) *DatePickerWidget {
	d.calendar.OnChange(fn)
	return d
}

// OnRangeChange reports the range edits a DateRangePicker's user asks for.
func (d *DatePickerWidget) OnRangeChange(fn func(DateRange)) *DatePickerWidget {
	d.calendar.OnRangeChange(fn)
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
	if d.button.IsInert() {
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
	defer d.props.Layout().EndLayout()
	d.button.Sync()
	if d.button.IsInert() {
		d.popup.Hide()
	}
	d.calendar.Disabled(d.button.IsInert())
	t := uitheme.Of(env)
	// panelBox's surface without its padding: the calendar brings its own,
	// as shadcn's date picker sets p-0 on the popover around it.
	panelShadow(d.panel, t).Fill(t.Popover).Border(t.BorderWidth, fade(t.Fg, .1)).Radius(t.Radius).Padding(ggui.EdgeInsets{})
	text := d.text()
	d.button.label.Content(pick(text == "", d.placeholder, text))
	return d.popup.Layout(c, env)
}

// Paint implements ggui.Widget.
func (d *DatePickerWidget) Paint(dst *ggui.Canvas, r ggui.Rect) { dst.Paint(d.popup, r) }

// SetName names the trigger for Field.
func (d *DatePickerWidget) SetName(s string) { d.Name(s) }

// HasName reports whether the trigger has an explicit name.
func (d *DatePickerWidget) HasName() bool { return d.button.HasName() }

// Semantics reports the trigger's role and resolved name.
func (d *DatePickerWidget) Semantics() (ggui.Role, string) { return d.button.Semantics() }

// BindDisabled follows r and closes the popup while disabled.
func (d *DatePickerWidget) BindDisabled(r ggui.Readable[bool]) *DatePickerWidget {
	d.button.BindDisabled(r)
	return d
}

// BindName follows the accessible name on the same targets as Name.
func (d *DatePickerWidget) BindName(r ggui.Readable[string]) *DatePickerWidget {
	d.button.BindName(r)
	d.calendar.BindName(r)
	return d
}
