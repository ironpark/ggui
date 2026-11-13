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
// length, with PageUp and PageDown, each repeated past the dates outside
// the bounds and held to the months the bounds are in. Enter then picks
// the browsed date.
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
		inside := func(d time.Time) bool { return lo.IsZero() || !d.Before(lo) && !d.After(hi) }
		want := start
		for _, b := range keys {
			key := calendarKeys[int(b)%len(calendarKeys)]
			c.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: key})
			move := func(d time.Time) time.Time {
				offset := (int(d.Weekday()) - int(weekStart) + 7) % 7
				switch key {
				case ggui.KeyArrowLeft:
					return d.AddDate(0, 0, -1)
				case ggui.KeyArrowRight:
					return d.AddDate(0, 0, 1)
				case ggui.KeyArrowUp:
					return d.AddDate(0, 0, -7)
				case ggui.KeyArrowDown:
					return d.AddDate(0, 0, 7)
				case ggui.KeyHome:
					return d.AddDate(0, 0, -offset)
				case ggui.KeyEnd:
					return d.AddDate(0, 0, 6-offset)
				}
				n := map[ggui.KeyboardKey]int{ggui.KeyPageUp: -1, ggui.KeyPageDown: 1}[key]
				first := time.Date(d.Year(), d.Month()+time.Month(n), 1, 0, 0, 0, 0, time.UTC)
				last := first.AddDate(0, 1, -1).Day()
				return first.AddDate(0, 0, min(d.Day(), last)-1)
			}
			// The move repeats past the days outside the bounds, and is
			// given up when a year of them reaches none inside.
			// A move past the months of the bounds stops at the first
			// day of the first, or the last day of the last.
			moved, held := false, false
			for d, n := want, 0; n < 366; n++ {
				next := move(d)
				if !lo.IsZero() {
					first := time.Date(lo.Year(), lo.Month(), 1, 0, 0, 0, 0, time.UTC)
					last := time.Date(hi.Year(), hi.Month()+1, 0, 0, 0, 0, 0, time.UTC)
					if next.Before(d) && next.Before(first) {
						next, held = first, true
					} else if next.After(d) && next.After(last) {
						next, held = last, true
					}
				}
				if next.Equal(d) {
					break
				}
				if d = next; inside(d) {
					want, moved = d, true
					break
				}
			}
			if !c.active.Equal(want) {
				t.Fatalf("from %v (week starts %v, bounds %v..%v), %v browsed to %v, want %v", start.Format(time.DateOnly), weekStart, lo.Format(time.DateOnly), hi.Format(time.DateOnly), key, c.active.Format(time.DateOnly), want.Format(time.DateOnly))
			}
			if key == ggui.KeyHome && moved && !held && c.active.Weekday() != weekStart {
				t.Fatalf("Home landed on a %v, want the week start %v", c.active.Weekday(), weekStart)
			}
		}
		// The keys never rest outside the bounds, so Enter picks the date.
		c.HandleKey(ggui.KeyEvent{Kind: ggui.KeyPress, Key: ggui.KeyEnter})
		if got := ggui.Untrack(value.Get); !inside(want) || !got.Equal(want) {
			t.Fatalf("Enter on %v within %v..%v chose %v", want.Format(time.DateOnly), lo.Format(time.DateOnly), hi.Format(time.DateOnly), got.Format(time.DateOnly))
		}
		if wantChanges := map[bool]int{true: 1, false: 0}[!want.Equal(start)]; changes != wantChanges {
			t.Fatalf("OnChange ran %d times, want %d", changes, wantChanges)
		}
	})
}
