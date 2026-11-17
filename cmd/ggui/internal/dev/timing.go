package dev

import "time"

// The durations of the way from a save to a build, in one place. They were
// tuned on an Apple silicon Mac, where FSEvents reports a save about 20 ms
// after it is written and ggui's counter example builds in 0.8 to 1 s.
const (
	// settleTime is how long the watcher gathers what the operating
	// system reports before it scans: an editor's save is several
	// events, a few milliseconds apart.
	settleTime = 15 * time.Millisecond
	// pollInterval is the time between scans where the operating system
	// says nothing about changes, or is not trusted to (GGUI_WATCH=poll).
	pollInterval = 250 * time.Millisecond
	// safetyNetInterval is the time between scans when it does: a
	// notification lost, as on a network file system, is caught by the
	// next.
	safetyNetInterval = 2 * time.Second

	// quietTime is how long the sources must go without a real change
	// before a build starts: a formatter that rewrites a file just saved,
	// or an editor that saves several, is then one build.
	quietTime = 50 * time.Millisecond
	// quietTimeMany replaces quietTime once manyChanges files have
	// changed since the last build, as a branch switch or a save-all
	// does, whose files land over a longer spell.
	quietTimeMany = 200 * time.Millisecond
	manyChanges   = 5
	// debounceCap is the longest a build waits for quiet after the first
	// change, so that a stream of changes cannot hold it off.
	debounceCap = 500 * time.Millisecond

	// syntaxRetry is how long a Go file that does not parse is given
	// before it is read again and its errors shown, in case it was read
	// half-written.
	syntaxRetry = 100 * time.Millisecond
)
