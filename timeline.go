package ggui

import (
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// TimelineValue is the progress of one animation from zero to one, played
// forward and back on command: what several values move by together, so
// they start, reverse and stop as one. It is a Readable; derive the values
// it drives with Interval, Stagger and Interpolate. Build one with
// Timeline.
//
//	tl := ggui.Timeline(400 * time.Millisecond)
//	fade := tl.Interval(0, 0.5, ggui.EaseOut)
//	rise := ggui.Interpolate(tl.Interval(0.2, 1, ggui.EaseOutBack), 24.0, 0.0, ggui.LerpFloat)
//	...
//	tl.Play()
//
// Its progress itself is linear in time; the curves belong to what reads it.
// When the application's environment asks for reduced motion (SetEnv with
// ReducedMotionKey), Play and Reverse land on their end at once.
type TimelineValue struct {
	w         *world // whose frames step it, from creation
	sig       *StateValue[float64]
	duration  time.Duration
	repeats   int // extra cycles each play runs, negative for ever
	alternate bool
	done      []func()

	from, dir float64 // progress the current cycle started at, and its direction
	left      int     // cycles still to run after this one
	start     time.Time
	running   bool
	disposed  bool
}

// Timeline creates a TimelineValue at zero that takes d to play through.
// Like Tween, one created under an owner stops when the owner is disposed;
// create it in setup.
func Timeline(d time.Duration) *TimelineValue {
	tl := &TimelineValue{w: ownerWorld(), sig: State(0.0), duration: d, dir: 1}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			tl.disposed, tl.running = true, false
			tl.w.anims.remove(tl)
		})
	}
	return tl
}

// Duration sets how long playing from zero to one takes.
func (tl *TimelineValue) Duration(d time.Duration) *TimelineValue { tl.duration = d; return tl }

// Repeat makes each Play or Reverse run n more cycles after the first,
// starting each again from its beginning; a negative n repeats until Stop.
func (tl *TimelineValue) Repeat(n int) *TimelineValue { tl.repeats = n; return tl }

// Alternate makes repeated cycles turn around at each end instead of
// starting over, for a pulse or a ping-pong.
func (tl *TimelineValue) Alternate() *TimelineValue { tl.alternate = true; return tl }

// OnDone calls fn each time a play or reverse runs to its end, every
// repeat included. Stop and Seek do not call it.
func (tl *TimelineValue) OnDone(fn func()) *TimelineValue { tl.done = append(tl.done, fn); return tl }

// Get returns the progress, from zero to one, and subscribes the running
// Effect.
func (tl *TimelineValue) Get() float64 { return tl.sig.Get() }

// GetAny returns the progress as any and subscribes, for Sprintf.
func (tl *TimelineValue) GetAny() any { return tl.Get() }

// Running reports whether the timeline is playing, without subscribing.
func (tl *TimelineValue) Running() bool { return tl.running }

// Play runs the progress forward to one from where it is, or from zero
// when it is already there.
func (tl *TimelineValue) Play() { tl.run(1) }

// Reverse runs the progress back to zero from where it is, or from one
// when it is already there.
func (tl *TimelineValue) Reverse() { tl.run(-1) }

// Stop freezes the progress where it is.
func (tl *TimelineValue) Stop() {
	tl.running = false
	tl.w.anims.remove(tl)
}

// Seek stops the timeline and puts its progress at p, held to [0, 1].
func (tl *TimelineValue) Seek(p float64) {
	if tl.disposed {
		return
	}
	tl.Stop()
	tl.sig.Set(clamp(p, 0, 1))
}

func (tl *TimelineValue) run(dir float64) {
	if tl.disposed {
		return
	}
	end := pick(dir > 0, 1.0, 0.0)
	if tl.duration <= 0 || Untrack(tl.w.environment().Get).ReducedMotion() {
		tl.Seek(end)
		tl.finish()
		return
	}
	p := Untrack(tl.sig.Get)
	if p == end {
		p = 1 - end
	}
	tl.from, tl.dir, tl.left = p, dir, tl.repeats
	tl.start = time.Time{} // taken from the first step
	tl.running = true
	tl.w.anims.add(tl)
}

func (tl *TimelineValue) finish() {
	for _, fn := range tl.done {
		fn()
	}
}

func (tl *TimelineValue) active() bool { return tl.running }

func (tl *TimelineValue) step(now time.Time) bool {
	if !tl.running {
		return false
	}
	if tl.duration <= 0 {
		// Duration was cut to zero mid-run: no cycle would ever advance the
		// clock, so land on the end as run does, rather than loop for ever.
		tl.running = false
		tl.sig.Set(pick(tl.dir > 0, 1.0, 0.0))
		tl.finish()
		return false
	}
	if tl.start.IsZero() {
		tl.start = now
	}
	d := float64(tl.duration)
	for {
		end := pick(tl.dir > 0, 1.0, 0.0)
		left := (end - tl.from) * tl.dir // the part of the run still ahead
		elapsed := float64(now.Sub(tl.start)) / d
		if elapsed < left {
			tl.sig.Set(tl.from + tl.dir*elapsed)
			return true
		}
		if tl.left == 0 {
			tl.running = false
			tl.sig.Set(end)
			tl.finish()
			return false
		}
		// The cycle ended inside this frame: start the next where it did.
		tl.start = tl.start.Add(time.Duration(left * d))
		if tl.left > 0 {
			tl.left--
		}
		tl.sig.Set(end) // what a done callback reads
		tl.finish()
		if !tl.running || tl.start.IsZero() {
			// A done callback stopped it, or played it again from where it
			// stands: that new run takes its start from the next step.
			return tl.running
		}
		if tl.alternate {
			tl.from, tl.dir = end, -tl.dir
		} else {
			tl.from = 1 - end
		}
	}
}

// Interval returns the timeline's progress through the stretch from begin
// to end, eased by e: zero before it, one after. See the package Interval.
func (tl *TimelineValue) Interval(begin, end float64, e Easing) Readable[float64] {
	return curved{tl, Interval(begin, end, e)}
}

// Stagger returns the progress of item i of n, eased by e, when each item
// plays for span of the timeline and their starts spread evenly across the
// rest: span 1 moves them all together, 1/n one after another. Use it for
// the rows of a list arriving in turn.
//
//	for i, row := range rows {
//		p := tl.Stagger(i, len(rows), 0.4, ggui.EaseOut)
//		... ggui.Interpolate(p, 12.0, 0.0, ggui.LerpFloat) ...
//	}
func (tl *TimelineValue) Stagger(i, n int, span float64, e Easing) Readable[float64] {
	span = clamp(span, 0, 1)
	if n <= 1 || span >= 1 {
		return tl.Interval(0, 1, e)
	}
	begin := float64(i) * (1 - span) / float64(n-1)
	return tl.Interval(begin, begin+span, e)
}

// curved is a progress read through an easing.
type curved struct {
	src  Readable[float64]
	ease Easing
}

func (c curved) Get() float64 { return c.ease(c.src.Get()) }

// Interpolate returns the value lerp gives between from and to at progress
// p: a Readable to bind a widget's property to, so it moves as p does.
//
//	box.BindWidth(ggui.Interpolate(tl, 40.0, 240.0, ggui.LerpFloat))
func Interpolate[T any](p Readable[float64], from, to T, lerp func(a, b T, t float64) T) Readable[T] {
	return interpolated[T]{p, from, to, lerp}
}

type interpolated[T any] struct {
	p        Readable[float64]
	from, to T
	lerp     func(a, b T, t float64) T
}

func (v interpolated[T]) Get() T { return v.lerp(v.from, v.to, v.p.Get()) }
