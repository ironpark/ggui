package ui

import (
	"slices"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/property"
)

// DateRange is an inclusive interval of civil dates. A zero Start means no
// selection; a nonzero Start with zero End is an unfinished range. CalendarRange
// interprets both dates in its Location and emits local midnights.
type DateRange struct{ Start, End time.Time }

// CalendarRange binds an inclusive range. A first click selects a single day
// (zero nights), or starts an unfinished range when RangeNights requires more.
// Further clicks extend or shorten it from its start. ResetOnSelect instead
// starts a new range after a completed one. Clicking a selected single-day
// range clears it unless Required is true.
func CalendarRange(value ggui.Binding[DateRange]) *CalendarWidget {
	property.Require(value, "CalendarRange")
	c := newCalendar()
	c.rangeValue = value
	return c
}

// CalendarMultiple toggles independent civil dates. User edits produce a
// sorted, duplicate-free slice of local midnights. Input and callback slices
// never alias each other. External selections remain authoritative.
func CalendarMultiple(value ggui.Binding[[]time.Time]) *CalendarWidget {
	property.Require(value, "CalendarMultiple")
	c := newCalendar()
	c.multipleValue = value
	return c
}

// OnRangeChange reports requested range edits from CalendarRange. A controlled
// binding may refuse them; the calendar always renders its getter's selection.
func (c *CalendarWidget) OnRangeChange(fn func(DateRange)) *CalendarWidget {
	c.onRangeChange = fn
	return c
}

// OnMultipleChange reports a copy of requested CalendarMultiple dates.
func (c *CalendarWidget) OnMultipleChange(fn func([]time.Time)) *CalendarWidget {
	c.onMultipleChange = fn
	return c
}

// Required prevents clearing a selected range or removing the last selected
// date in multiple mode. It does not invent a selection for an empty binding.
func (c *CalendarWidget) Required(v bool) *CalendarWidget {
	defer property.Watch(&c.props, &c.required)()
	c.required = v
	return c
}

// SelectionLimits limits the number of dates in multiple mode. Minimum
// prevents removing dates once reached; selections may start below minimum.
// A maximum of zero is unlimited.
func (c *CalendarWidget) SelectionLimits(minimum, maximum int) *CalendarWidget {
	if minimum < 0 || maximum < 0 || (maximum > 0 && maximum < minimum) {
		panic("ui: invalid calendar selection limits")
	}
	defer property.Watch(&c.props, &c.minSelected)()
	defer property.Watch(&c.props, &c.maxSelected)()
	c.minSelected, c.maxSelected = minimum, maximum
	return c
}

// RangeNights constrains the difference between range endpoints in civil days,
// independent of daylight saving time. Zero maximum is unlimited. A candidate
// outside these limits starts a new unfinished range at the clicked day.
func (c *CalendarWidget) RangeNights(minimum, maximum int) *CalendarWidget {
	if minimum < 0 || maximum < 0 || (maximum > 0 && maximum < minimum) {
		panic("ui: invalid calendar range nights")
	}
	defer property.Watch(&c.props, &c.minNights)()
	defer property.Watch(&c.props, &c.maxNights)()
	c.minNights, c.maxNights = minimum, maximum
	return c
}

// ExcludeDisabled restarts an unfinished range at the clicked day when a
// proposed interval contains a disabled date. Disabled endpoints are always
// unselectable; by default disabled interior dates may belong to a range.
func (c *CalendarWidget) ExcludeDisabled(v bool) *CalendarWidget {
	defer property.Watch(&c.props, &c.excludeDisabled)()
	c.excludeDisabled = v
	return c
}

// ResetOnSelect starts a new range on the next click after a completed range.
func (c *CalendarWidget) ResetOnSelect(v bool) *CalendarWidget {
	defer property.Watch(&c.props, &c.resetOnSelect)()
	c.resetOnSelect = v
	return c
}

func (c *CalendarWidget) normalizedRange(v DateRange) DateRange {
	v.Start, v.End = c.date(v.Start), c.date(v.End)
	if v.Start.IsZero() {
		return DateRange{}
	}
	if !v.End.IsZero() && v.End.Before(v.Start) {
		v.Start, v.End = v.End, v.Start
	}
	return v
}
func (c *CalendarWidget) normalizedDates(v []time.Time) []time.Time {
	dates := make([]time.Time, 0, len(v))
	for _, t := range v {
		if !t.IsZero() {
			dates = append(dates, c.date(t))
		}
	}
	slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
	return slices.CompactFunc(dates, func(a, b time.Time) bool { return a.Equal(b) })
}

// selectionAnchor reads the binding in the caller's tracking scope. Layout
// uses it to follow external selection changes; interaction callers untrack it.
func (c *CalendarWidget) selectionAnchor() time.Time {
	if c.rangeValue != nil {
		return c.normalizedRange(c.rangeValue.Get()).Start
	}
	if c.multipleValue != nil {
		// The earliest date, without sorting a copy of them all to find it.
		var first time.Time
		for _, t := range c.multipleValue.Get() {
			if d := c.date(t); !t.IsZero() && (first.IsZero() || d.Before(first)) {
				first = d
			}
		}
		return first
	}
	return c.date(c.value.Get())
}
func (c *CalendarWidget) selected(day time.Time) bool {
	if c.rangeValue != nil {
		v := c.normalizedRange(ggui.Untrack(c.rangeValue.Get))
		if v.Start.IsZero() {
			return false
		}
		if v.End.IsZero() {
			return day.Equal(v.Start)
		}
		return !day.Before(v.Start) && !day.After(v.End)
	}
	if c.multipleValue != nil {
		for _, t := range ggui.Untrack(c.multipleValue.Get) {
			if !t.IsZero() && c.date(t).Equal(day) {
				return true
			}
		}
		return false
	}
	return c.date(ggui.Untrack(c.value.Get)).Equal(day)
}
func (c *CalendarWidget) selectionEdge(day time.Time) bool {
	if c.rangeValue != nil {
		v := c.normalizedRange(ggui.Untrack(c.rangeValue.Get))
		return !v.Start.IsZero() && (day.Equal(v.Start) || day.Equal(v.End))
	}
	return c.selected(day)
}

func (c *CalendarWidget) chooseMultiple(day time.Time) {
	dates := c.normalizedDates(ggui.Untrack(c.multipleValue.Get))
	i := slices.IndexFunc(dates, func(t time.Time) bool { return t.Equal(day) })
	if i >= 0 {
		minimum := c.minSelected
		if c.required {
			minimum = max(1, minimum)
		}
		if len(dates) <= minimum {
			return
		}
		dates = slices.Delete(dates, i, i+1)
	} else {
		if c.maxSelected > 0 && len(dates) >= c.maxSelected {
			return
		}
		dates = append(dates, day)
		slices.SortFunc(dates, func(a, b time.Time) int { return a.Compare(b) })
	}
	c.multipleValue.Set(slices.Clone(dates))
	if c.onMultipleChange != nil {
		c.onMultipleChange(slices.Clone(dates))
	}
}

func calendarNights(start, end time.Time) int {
	a := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	b := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	return int((b.Unix() - a.Unix()) / 86400)
}
func (c *CalendarWidget) chooseRange(day time.Time) {
	current := c.normalizedRange(ggui.Untrack(c.rangeValue.Get))
	next := current
	startNew := func() DateRange {
		r := DateRange{Start: day}
		if c.minNights == 0 {
			r.End = day
		}
		return r
	}
	switch {
	case current.Start.IsZero():
		next = startNew()
	case current.Start.Equal(day) && current.End.Equal(day):
		if c.required {
			return
		}
		next = DateRange{}
	case c.resetOnSelect && !current.End.IsZero():
		next = startNew()
	case current.End.IsZero():
		if day.Equal(current.Start) {
			if c.minNights == 0 {
				next.End = day
			} else if !c.required {
				next = DateRange{}
			}
		} else if day.Before(current.Start) {
			next = DateRange{Start: day, End: current.Start}
		} else {
			next.End = day
		}
	case day.Equal(current.Start) || day.Equal(current.End):
		next = startNew()
	case day.Before(current.Start):
		next.Start = day
	default:
		next.End = day
	}
	if !next.Start.IsZero() && !next.End.IsZero() {
		nights := calendarNights(next.Start, next.End)
		if nights < c.minNights || (c.maxNights > 0 && nights > c.maxNights) {
			next = DateRange{Start: day}
		} else if c.excludeDisabled {
			for at := next.Start; !at.After(next.End); at = at.AddDate(0, 0, 1) {
				if !c.enabled(at) {
					next = DateRange{Start: day}
					break
				}
			}
		}
	}
	if next.Start.Equal(current.Start) && next.End.Equal(current.End) {
		return
	}
	c.rangeValue.Set(next)
	if c.onRangeChange != nil {
		c.onRangeChange(next)
	}
}
