package ggui

import "time"

// clock is the time source every animation reads; SetClock replaces it.
var clock = time.Now

// Now returns the current time as animations see it: time.Now, unless a
// test has installed a clock with SetClock. Widgets that ease a value
// should read Now rather than time.Now so tests can step them.
func Now() time.Time { return clock() }

// SetClock replaces the time source that Now and every animation use, and
// returns a function that restores the previous one. It is for tests:
//
//	restore := ggui.SetClock(func() time.Time { return now })
//	defer restore()
func SetClock(fn func() time.Time) (restore func()) {
	prev := clock
	clock = fn
	return func() { clock = prev }
}
