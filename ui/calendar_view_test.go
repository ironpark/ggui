package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// calendarDates counts the described days named date.
func calendarDates(p *ggui.Probe, date string) int {
	n := 0
	for _, node := range p.Semantics().Nodes(ggui.RoleOption) {
		if node.Name == date {
			n++
		}
	}
	return n
}

func TestCalendarMeasuresFromItsCells(t *testing.T) {
	t.Parallel()
	c := ui.Calendar(ggui.State(civil(2026, time.June, 10))).Location(time.UTC)
	p := ggui.NewProbe(ggui.Column(c).Align(ggui.AlignStart), ggui.Sz(600, 600))
	defer p.Close()
	// Seven 28px cells between 8px of padding.
	group := semNode(t, p, ggui.RoleGroup, "Calendar")
	if group.Rect.Size.W != 7*28+16 {
		t.Fatalf("calendar is %v wide, want 212", group.Rect.Size.W)
	}
	// A day's hit target is its whole column and the row with its gap.
	a, b := semNode(t, p, ggui.RoleOption, "2026-06-10"), semNode(t, p, ggui.RoleOption, "2026-06-17")
	if a.Rect.Size != ggui.Sz(28, 36) || b.Rect.Origin.Y-a.Rect.Origin.Y != 36 {
		t.Fatalf("day targets %v then %v, want 28x36 a row of 36 apart", a.Rect, b.Rect)
	}
	// Stretched, the columns share the width and the targets stay adjacent.
	wide := ggui.NewProbe(ui.Calendar(ggui.State(civil(2026, time.June, 10))).Location(time.UTC), ggui.Sz(296, 320))
	defer wide.Close()
	a, b = semNode(t, wide, ggui.RoleOption, "2026-06-10"), semNode(t, wide, ggui.RoleOption, "2026-06-11")
	if a.Rect.Size.W != 40 || b.Rect.Origin.X != a.Rect.Origin.X+40 {
		t.Fatalf("stretched day targets %v then %v, want 40 wide and adjacent", a.Rect, b.Rect)
	}
}

func TestCalendarMonthsShowsEachDateOnceAndSlidesByOne(t *testing.T) {
	t.Parallel()
	value := ggui.State(ui.DateRange{Start: civil(2026, time.June, 29), End: civil(2026, time.July, 2)})
	c := ui.CalendarRange(value).Location(time.UTC).Months(2)
	p := ggui.NewProbe(ggui.Column(c).Align(ggui.AlignStart), ggui.Sz(600, 400))
	defer p.Close()
	if w := semNode(t, p, ggui.RoleGroup, "Calendar").Rect.Size.W; w != 2*7*28+16+16 {
		t.Fatalf("two months are %v wide, want 424", w)
	}
	// June's trailing days are July's own, and described there alone.
	for _, date := range []string{"2026-06-01", "2026-06-30", "2026-07-01", "2026-07-31", "2026-05-31", "2026-08-01"} {
		if n := calendarDates(p, date); n != 1 {
			t.Fatalf("%s is described %d times, want once", date, n)
		}
	}
	calendarSelected(t, p, "2026-07-02", true)
	// Browsing with the keys inside the months shown leaves them in place.
	p.Tap("2026-06-30")
	p.Key("right")
	if calendarDates(p, "2026-06-01") != 1 || calendarDates(p, "2026-07-31") != 1 {
		t.Fatal("moving to the second month changed the months shown")
	}
	p.Key("pagedown", "pagedown")
	if calendarDates(p, "2026-08-31") != 1 || calendarDates(p, "2026-09-30") != 1 || calendarDates(p, "2026-07-15") != 0 {
		t.Fatalf("PageDown twice did not show August and September:\n%s", p.Semantics())
	}
	p.Tap("Previous month")
	if calendarDates(p, "2026-07-15") != 1 || calendarDates(p, "2026-08-15") != 1 || calendarDates(p, "2026-09-15") != 0 {
		t.Fatal("Previous month did not slide the months by one")
	}
	// Too narrow for two side by side, the months stack.
	narrow := ggui.NewProbe(ggui.Column(ui.Calendar(ggui.State(civil(2026, time.June, 10))).Location(time.UTC).Months(2)).Align(ggui.AlignStart), ggui.Sz(300, 700))
	defer narrow.Close()
	june, july := semNode(t, narrow, ggui.RoleOption, "2026-06-10"), semNode(t, narrow, ggui.RoleOption, "2026-07-10")
	if july.Rect.Origin.Y <= june.Rect.Origin.Y+200 {
		t.Fatalf("in 300px July sits at %v beside June at %v, want below", july.Rect, june.Rect)
	}
}

func TestCalendarCaptionDropdownBrowsesByMonthAndYear(t *testing.T) {
	t.Parallel()
	value := ggui.State(civil(2026, time.June, 30))
	c := ui.Calendar(value).Location(time.UTC).CaptionDropdown(true).DropdownYears(2020, 2030)
	p := ggui.NewProbe(ggui.Column(c).Align(ggui.AlignStart), ggui.Sz(600, 500))
	defer p.Close()
	if month, year := semNode(t, p, ggui.RoleSelect, "Month"), semNode(t, p, ggui.RoleSelect, "Year"); month.Value != "Jun" || year.Value != "2026" {
		t.Fatalf("dropdowns show %q %q, want Jun 2026", month.Value, year.Value)
	}
	act(t, p, ggui.RoleSelect, "Month", ggui.Action{Kind: ggui.ActionSetValue, Text: "Feb"})
	act(t, p, ggui.RoleSelect, "Year", ggui.Action{Kind: ggui.ActionSetValue, Text: "2024"})
	if calendarDates(p, "2024-02-29") != 1 {
		t.Fatalf("picking Feb 2024 did not browse there:\n%s", p.Semantics())
	}
	if !ggui.Peek(value).Equal(civil(2026, time.June, 30)) {
		t.Fatal("browsing with the dropdowns changed the selection")
	}
	// The buttons still step, and the dropdowns follow.
	p.Tap("Next month")
	if month := semNode(t, p, ggui.RoleSelect, "Month"); month.Value != "Mar" {
		t.Fatalf("after Next month the dropdown shows %q, want Mar", month.Value)
	}
	// A year outside the configured span is still offered while it is shown.
	value.Set(civil(2035, time.January, 5))
	if year := semNode(t, p, ggui.RoleSelect, "Year"); year.Value != "2035" {
		t.Fatalf("the year dropdown shows %q for a date in 2035", year.Value)
	}
	c.MonthNames([12]string{"1월", "2월", "3월", "4월", "5월", "6월", "7월", "8월", "9월", "10월", "11월", "12월"})
	if month := semNode(t, p, ggui.RoleSelect, "Month"); month.Value != "1월" {
		t.Fatalf("localized month dropdown shows %q, want 1월", month.Value)
	}
}

func TestDateRangePickerStaysOpenAndShowsTheRange(t *testing.T) {
	t.Parallel()
	value := ggui.State(ui.DateRange{})
	changes := 0
	d := ui.DateRangePicker(value).Name("Stay").OnRangeChange(func(ui.DateRange) { changes++ })
	d.Calendar().Location(time.UTC).Bounds(civil(2026, time.June, 1), time.Time{})
	p := ggui.NewProbe(ggui.Column(d).Align(ggui.AlignStart), ggui.Sz(640, 520))
	defer p.Close()
	if trigger := semNode(t, p, ggui.RoleButton, "Stay"); trigger.Value != "" {
		t.Fatalf("an empty range has the value %q", trigger.Value)
	}
	value.Set(ui.DateRange{Start: civil(2026, time.June, 10), End: civil(2026, time.June, 10)})
	p.Tap("Stay")
	// Two months, side by side in a window that holds them.
	june, july := semNode(t, p, ggui.RoleOption, "2026-06-12"), semNode(t, p, ggui.RoleOption, "2026-07-12")
	if july.Rect.Origin.X <= june.Rect.Origin.X {
		t.Fatalf("July at %v is not beside June at %v", july.Rect, june.Rect)
	}
	p.Tap("2026-07-02")
	if !d.Popup().IsOpen() {
		t.Fatal("picking the end of a range closed the picker")
	}
	got := ggui.Peek(value)
	if !got.Start.Equal(civil(2026, time.June, 10)) || !got.End.Equal(civil(2026, time.July, 2)) || changes != 1 {
		t.Fatalf("range=%v changes=%d", got, changes)
	}
	if trigger := semNode(t, p, ggui.RoleButton, "Stay"); trigger.Value != "2026-06-10 – 2026-07-02" {
		t.Fatalf("trigger value %q", trigger.Value)
	}
	d.Format(func(t time.Time) string { return t.Format("Jan 2") })
	if trigger := semNode(t, p, ggui.RoleButton, "Stay"); trigger.Value != "Jun 10 – Jul 2" {
		t.Fatalf("trigger value %q, want each end through Format", trigger.Value)
	}
	d.FormatRange(func(r ui.DateRange) string { return r.Start.Format("01/02") + "→" + r.End.Format("01/02") }).FormatRange(nil)
	if trigger := semNode(t, p, ggui.RoleButton, "Stay"); trigger.Value != "06/10→07/02" {
		t.Fatalf("trigger value %q, want the custom range format", trigger.Value)
	}
}

// semNode is the described node with role and name, failing the test without one.
func semNode(t *testing.T, p *ggui.Probe, role ggui.Role, name string) ggui.SemNode {
	t.Helper()
	n, ok := p.Semantics().Find(role, name)
	if !ok {
		t.Fatalf("no %v named %q:\n%s", role, name, p.Semantics())
	}
	return n
}

func TestDatePickerCentresItsCalendarUnderAWideTrigger(t *testing.T) {
	t.Parallel()
	d := ui.DatePicker(ggui.State(civil(2026, time.June, 10))).Name("Due")
	d.Calendar().Location(time.UTC)
	p := ggui.NewProbe(ggui.Column(d).Align(ggui.AlignStretch), ggui.Sz(600, 480))
	defer p.Close()
	p.Tap("Due")
	trigger, cal := semNode(t, p, ggui.RoleButton, "Due"), semNode(t, p, ggui.RoleGroup, "Due")
	// The popup is as wide as the trigger; the calendar inside keeps its
	// 7×28 + 2×8 and sits in the middle, with no padding of the popup's own.
	if cal.Rect.Size.W != 7*28+16 {
		t.Fatalf("calendar is %v wide, want 212", cal.Rect.Size.W)
	}
	if got, want := cal.Rect.Center().X, trigger.Rect.Center().X; got-want > .5 || want-got > .5 {
		t.Fatalf("calendar centred at %v, want under the trigger's middle %v", got, want)
	}
	if d.Popup().Rect().Size.H != cal.Rect.Size.H {
		t.Fatalf("popup %v tall around a %v calendar, want no padding of its own", d.Popup().Rect().Size.H, cal.Rect.Size.H)
	}
}
