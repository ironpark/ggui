package ui_test

import (
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func civil(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
func calendarProbe(t *testing.T, w ggui.Widget) *ggui.Probe {
	t.Helper()
	p := ggui.NewProbe(w, ggui.Sz(294, 320))
	t.Cleanup(p.Close)
	p.Frame()
	return p
}
func calendarSelected(t *testing.T, p *ggui.Probe, date string, want bool) {
	t.Helper()
	n, ok := p.Semantics().Find(ggui.RoleOption, date)
	if !ok || n.Selected != want {
		t.Fatalf("date %s selected=%v found=%v want %v", date, n.Selected, ok, want)
	}
}

func TestCalendarRangeSelectsAcrossMonthsAndKeyboard(t *testing.T) {
	t.Parallel()
	start := civil(2026, time.January, 30)
	value := ggui.State(ui.DateRange{Start: start, End: start})
	changes := 0
	c := ui.CalendarRange(value).Location(time.UTC).WeekStartsOn(time.Monday).OnRangeChange(func(ui.DateRange) { changes++ })
	p := calendarProbe(t, c)
	p.Tap("Next month")
	p.Tap("2026-02-02")
	got := ggui.Peek(value)
	if !got.Start.Equal(start) || !got.End.Equal(civil(2026, time.February, 2)) || changes != 1 {
		t.Fatalf("range=%v changes=%d", got, changes)
	}
	for _, day := range []string{"2026-01-30", "2026-01-31", "2026-02-01", "2026-02-02"} {
		calendarSelected(t, p, day, true)
	}
	calendarSelected(t, p, "2026-02-03", false)
	// Clicking a date focuses the grid; navigation alone leaves the selection.
	p.Key("right")
	if !ggui.Peek(value).End.Equal(got.End) {
		t.Fatal("navigation selected a date")
	}
	p.Key("enter")
	if !ggui.Peek(value).End.Equal(civil(2026, time.February, 3)) {
		t.Fatalf("keyboard range=%v", ggui.Peek(value))
	}
}

func TestCalendarRangeResetClearAndRequired(t *testing.T) {
	t.Parallel()
	day := civil(2026, time.March, 10)
	value := ggui.State(ui.DateRange{Start: day, End: day.AddDate(0, 0, 4)})
	c := ui.CalendarRange(value).Location(time.UTC).ResetOnSelect(true)
	p := calendarProbe(t, c)
	p.Tap("2026-03-20")
	got := ggui.Peek(value)
	if !got.Start.Equal(civil(2026, time.March, 20)) || !got.End.Equal(got.Start) {
		t.Fatalf("reset=%v", got)
	}
	c.Required(true)
	p.Tap("2026-03-20")
	if ggui.Peek(value).Start.IsZero() {
		t.Fatal("required range cleared")
	}
	c.Required(false)
	p.Tap("2026-03-20")
	if !ggui.Peek(value).Start.IsZero() {
		t.Fatal("range did not clear")
	}
}

func TestCalendarRangeBoundsDisabledAndNightLimits(t *testing.T) {
	t.Parallel()
	start := civil(2026, time.March, 10)
	value := ggui.State(ui.DateRange{Start: start})
	c := ui.CalendarRange(value).Location(time.UTC).RangeNights(2, 5).Bounds(start, start.AddDate(0, 0, 20)).DisabledDate(func(d time.Time) bool { return d.Day() == 12 }).ExcludeDisabled(true)
	p := calendarProbe(t, c)
	calendarClick(t, p, "2026-03-09")
	if !ggui.Peek(value).Start.Equal(start) {
		t.Fatal("out-of-bounds date selected")
	}
	calendarClick(t, p, "2026-03-12")
	if !ggui.Peek(value).End.IsZero() {
		t.Fatal("disabled endpoint selected")
	}
	p.Tap("2026-03-14")
	got := ggui.Peek(value)
	if !got.Start.Equal(civil(2026, time.March, 14)) || !got.End.IsZero() {
		t.Fatalf("excluded interior should restart: %v", got)
	}
	p.Tap("2026-03-15")
	if !ggui.Peek(value).Start.Equal(civil(2026, time.March, 15)) || !ggui.Peek(value).End.IsZero() {
		t.Fatal("minimum nights accepted a one-night range")
	}
	p.Tap("2026-03-17")
	if !ggui.Peek(value).End.Equal(civil(2026, time.March, 17)) {
		t.Fatal("two-night range rejected")
	}
	p.Tap("2026-03-25")
	if !ggui.Peek(value).Start.Equal(civil(2026, time.March, 25)) || !ggui.Peek(value).End.IsZero() {
		t.Fatal("maximum nights not enforced")
	}
}

func TestCalendarRangeCivilNightsAcrossDST(t *testing.T) {
	t.Parallel()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, time.March, 7, 0, 0, 0, 0, loc)
	value := ggui.State(ui.DateRange{Start: start})
	p := calendarProbe(t, ui.CalendarRange(value).Location(loc).RangeNights(2, 2))
	calendarClick(t, p, "2026-03-09")
	got := ggui.Peek(value)
	if got.End.IsZero() || got.End.Sub(got.Start) != 47*time.Hour {
		t.Fatalf("civil DST range=%v", got)
	}
}

func TestCalendarMultipleTogglesWithLimitsAndSliceIsolation(t *testing.T) {
	t.Parallel()
	first := civil(2026, time.June, 10)
	input := []time.Time{first, first.Add(12 * time.Hour), civil(2026, time.June, 15)}
	value := ggui.State(input)
	changes := 0
	c := ui.CalendarMultiple(value).Location(time.UTC).SelectionLimits(1, 3).OnMultipleChange(func(v []time.Time) {
		changes++
		if len(v) > 0 {
			v[0] = time.Time{}
		}
	})
	p := calendarProbe(t, c)
	calendarSelected(t, p, "2026-06-10", true)
	calendarSelected(t, p, "2026-06-15", true)
	p.Tap("2026-06-10")
	if got := ggui.Peek(value); len(got) != 1 || !got[0].Equal(civil(2026, time.June, 15)) {
		t.Fatalf("toggle normalized duplicates: %v", got)
	}
	if len(input) != 3 || input[0].IsZero() {
		t.Fatal("edited input backing slice")
	}
	p.Tap("2026-06-15")
	if len(ggui.Peek(value)) != 1 || changes != 1 {
		t.Fatal("minimum selection not enforced")
	}
	p.Tap("2026-06-12")
	p.Tap("2026-06-20")
	p.Tap("2026-06-22")
	got := ggui.Peek(value)
	if len(got) != 3 || changes != 3 || !got[0].Equal(civil(2026, time.June, 12)) || got[0].IsZero() {
		t.Fatalf("max/sort/callback copy: %v changes=%d", got, changes)
	}
	calendarSelected(t, p, "2026-06-22", false)
	c.SelectionLimits(0, 0).Required(true)
	p.Tap("2026-06-12")
	p.Tap("2026-06-20")
	p.Tap("2026-06-15")
	if len(ggui.Peek(value)) != 1 {
		t.Fatal("required removed last date")
	}
}

func TestCalendarSelectionControlledRejectionAndDisabled(t *testing.T) {
	t.Parallel()
	day := civil(2026, time.July, 10)
	original := ui.DateRange{Start: day, End: day}
	var requested ui.DateRange
	p := calendarProbe(t, ui.CalendarRange(ggui.Controlled(original, func(v ui.DateRange) { requested = v })).Location(time.UTC))
	p.Tap("2026-07-15")
	if !requested.End.Equal(civil(2026, time.July, 15)) {
		t.Fatalf("request=%v", requested)
	}
	calendarSelected(t, p, "2026-07-15", false)

	dates := []time.Time{day}
	var requestedDates []time.Time
	disabled := ggui.State(false)
	c := ui.CalendarMultiple(ggui.Controlled(dates, func(v []time.Time) { requestedDates = v })).Location(time.UTC).BindDisabled(disabled)
	q := calendarProbe(t, c)
	act(t, q, ggui.RoleOption, "2026-07-15", ggui.Action{Kind: ggui.ActionSelect})
	if len(requestedDates) != 2 || !requestedDates[1].Equal(civil(2026, time.July, 15)) {
		t.Fatalf("requested dates=%v", requestedDates)
	}
	calendarSelected(t, q, "2026-07-15", false)
	requestedDates[0] = time.Time{}
	if !dates[0].Equal(day) {
		t.Fatal("controlled source alias")
	}
	requestedDates = nil
	disabled.Set(true)
	calendarClick(t, q, "2026-07-16")
	if requestedDates != nil {
		t.Fatal("disabled calendar requested selection")
	}
}

func TestCalendarMultipleKeyboardAndRTL(t *testing.T) {
	t.Parallel()
	start := civil(2026, time.August, 10)
	values := ggui.State([]time.Time{start})
	c := ui.CalendarMultiple(values).Location(time.UTC)
	p := calendarProbe(t, ggui.Directionality(ggui.DirRTL, c))
	p.Tap("2026-08-10") // toggles off, leaves this date browsed
	p.Key("left", "enter")
	got := ggui.Peek(values)
	if !slices.EqualFunc(got, []time.Time{civil(2026, time.August, 11)}, func(a, b time.Time) bool { return a.Equal(b) }) {
		t.Fatalf("RTL keyboard selected=%v", got)
	}
	p.Key("space")
	if len(ggui.Peek(values)) != 0 {
		t.Fatal("Space did not toggle selected date")
	}
}

func calendarClick(t *testing.T, p *ggui.Probe, date string) {
	t.Helper()
	n, ok := p.Semantics().Find(ggui.RoleOption, date)
	if !ok {
		t.Fatalf("no date %s", date)
	}
	p.Click(n.Rect.Center())
}

func TestCalendarFunctionSettersInvalidateCachedLayout(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"single", "range", "multiple"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			start := civil(2026, time.June, 10)
			var c *ui.CalendarWidget
			switch mode {
			case "single":
				c = ui.Calendar(ggui.State(start))
			case "range":
				c = ui.CalendarRange(ggui.State(ui.DateRange{Start: start, End: start}))
			case "multiple":
				c = ui.CalendarMultiple(ggui.State([]time.Time{start}))
			}
			c.Location(time.UTC)
			p := calendarProbe(t, ggui.Column(c))
			p.Frame()
			p.Frame() // ensure the parent is reusing its child's measurement
			c.DisabledDate(func(d time.Time) bool { return d.Day() == 12 })
			n, ok := p.Semantics().Find(ggui.RoleOption, "2026-06-12")
			if !ok || !n.Disabled {
				t.Fatalf("predicate replacement did not disable date: %+v", n)
			}
			c.DisabledDate(nil)
			n, ok = p.Semantics().Find(ggui.RoleOption, "2026-06-12")
			if !ok || n.Disabled {
				t.Fatalf("cleared predicate did not enable date: %+v", n)
			}
			c.MonthLabel(func(time.Time) string { return "Custom month" })
			if _, ok := p.Semantics().Find(ggui.RoleText, "Custom month"); !ok {
				t.Fatal("month formatter replacement was not laid out")
			}
			c.MonthLabel(nil)
			if _, ok := p.Semantics().Find(ggui.RoleText, "Custom month"); !ok {
				t.Fatal("nil formatter replaced the current formatter")
			}
		})
	}
}
