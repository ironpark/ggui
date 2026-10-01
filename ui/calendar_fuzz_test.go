package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// calendarKeys are the keys the calendar grid navigates with, indexed by a
// fuzz byte.
var calendarKeys = []ggui.KeyboardKey{
	ggui.KeyArrowLeft, ggui.KeyArrowRight, ggui.KeyArrowUp, ggui.KeyArrowDown,
	ggui.KeyHome, ggui.KeyEnd, ggui.KeyPageUp, ggui.KeyPageDown,
}

// FuzzCalendarKeyboardDateMath drives the calendar grid's keys from random
// dates and week starts against a reference of the documented moves: a day
// or a week with the arrows, the ends of the displayed week with Home and
// End, and the same day of the next or previous month, clamped to its
// length, with PageUp and PageDown. Enter then picks the browsed date only
// inside the bounds.
func FuzzCalendarKeyboardDateMath(f *testing.F) {
	f.Add(int16(2024), uint8(1), uint8(31), uint8(0), []byte{7, 7, 6}, uint8(3))
	f.Add(int16(2023), uint8(12), uint8(31), uint8(1), []byte{1, 5, 4, 3, 2, 0}, uint8(0))
	f.Add(int16(2000), uint8(2), uint8(29), uint8(6), []byte{6, 6, 6, 6, 7}, uint8(40))
	f.Add(int16(100), uint8(1), uint8(1), uint8(3), []byte{0, 2, 6}, uint8(10))
	f.Fuzz(func(t *testing.T, year int16, month, day, week uint8, keys []byte, reach uint8) {
		// Keep well inside the years time.Date handles, and clear of
		// 0001-01-01: that is time.Time{}, which the calendar reads as no date.
		if year < 100 || year > 9800 || len(keys) > 64 {
			t.Skip()
		}
		start := time.Date(int(year), time.Month(month%12+1), int(day%28+1), 0, 0, 0, 0, time.UTC)
		weekStart := time.Weekday(week % 7)
		// Bounds reach a few days either side of the start, or none at all.
		var lo, hi time.Time
		if reach > 0 {
			lo, hi = start.AddDate(0, 0, -int(reach%64)), start.AddDate(0, 0, int(reach%64))
		}
		value := ggui.State(start)
		changes := 0
		c := Calendar(value).Location(time.UTC).WeekStartsOn(weekStart).Bounds(lo, hi).OnChange(func(time.Time) { changes++ })
		c.Layout(ggui.Loose(ggui.Sz(300, 400)), ggui.Env{}) // resolves the bounds, as mounting does
		want := start
		for _, b := range keys {
			key := calendarKeys[int(b)%len(calendarKeys)]
			c.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: key})
			offset := (int(want.Weekday()) - int(weekStart) + 7) % 7
			switch key {
			case ggui.KeyArrowLeft:
				want = want.AddDate(0, 0, -1)
			case ggui.KeyArrowRight:
				want = want.AddDate(0, 0, 1)
			case ggui.KeyArrowUp:
				want = want.AddDate(0, 0, -7)
			case ggui.KeyArrowDown:
				want = want.AddDate(0, 0, 7)
			case ggui.KeyHome:
				want = want.AddDate(0, 0, -offset)
			case ggui.KeyEnd:
				want = want.AddDate(0, 0, 6-offset)
			case ggui.KeyPageUp, ggui.KeyPageDown:
				n := map[ggui.KeyboardKey]int{ggui.KeyPageUp: -1, ggui.KeyPageDown: 1}[key]
				first := time.Date(want.Year(), want.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
				last := first.AddDate(0, 1, -1).Day()
				want = first.AddDate(0, 0, min(want.Day(), last)-1)
			}
			if !c.active.Equal(want) {
				t.Fatalf("from %v (week starts %v), %v browsed to %v, want %v", start.Format(time.DateOnly), weekStart, key, c.active.Format(time.DateOnly), want.Format(time.DateOnly))
			}
			if key == ggui.KeyHome && c.active.Weekday() != weekStart {
				t.Fatalf("Home landed on a %v, want the week start %v", c.active.Weekday(), weekStart)
			}
		}
		c.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEnter})
		inside := lo.IsZero() || !want.Before(lo) && !want.After(hi)
		got := ggui.Untrack(value.Get)
		switch {
		case inside && !got.Equal(want):
			t.Fatalf("Enter on %v inside the bounds chose %v", want.Format(time.DateOnly), got.Format(time.DateOnly))
		case !inside && !got.Equal(start):
			t.Fatalf("Enter on %v outside %v..%v changed the date to %v", want.Format(time.DateOnly), lo.Format(time.DateOnly), hi.Format(time.DateOnly), got.Format(time.DateOnly))
		}
		if wantChanges := map[bool]int{true: 1, false: 0}[inside && !want.Equal(start)]; changes != wantChanges {
			t.Fatalf("OnChange ran %d times, want %d", changes, wantChanges)
		}
	})
}
