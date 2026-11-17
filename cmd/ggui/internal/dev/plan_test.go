package dev

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// sim drives a plan as Native.Run does, on a clock of its own, with a
// builder that takes buildTime and a watcher that finds a save foundAfter
// it was made, or at once when the last scan before a restart looks.
type sim struct {
	t          *testing.T
	buildTime  time.Duration
	foundAfter time.Duration

	saves []save // by time
	p     plan
	now   time.Duration

	running  bool
	runID    int
	runEnd   time.Duration
	runBatch []string

	log      []string // what happened, for a failure
	started  []time.Duration
	cancels  int
	discards int
	restarts [][]string // the batches the app was restarted on
}

type save struct {
	at    time.Duration
	paths []string
}

var epoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func (s *sim) logf(format string, args ...any) {
	s.log = append(s.log, fmt.Sprintf("%6dms ", s.now.Milliseconds())+fmt.Sprintf(format, args...))
}

// change is the loop's handling of what the watcher found.
func (s *sim) change(paths []string) {
	if s.p.change(epoch.Add(s.now), paths) {
		s.logf("cancel build %d", s.runID)
		s.cancels++
		s.running = false
	}
}

// run plays the saves and returns once all is quiet.
func (s *sim) run() {
	for step := 0; step < 10000; step++ {
		// The next thing to happen: a save found, a build due, a build
		// done.
		next := time.Duration(-1)
		kind := ""
		consider := func(at time.Duration, k string) {
			if next < 0 || at < next {
				next, kind = at, k
			}
		}
		if len(s.saves) > 0 {
			consider(s.saves[0].at+s.foundAfter, "found")
		}
		if t, ok := s.p.due(); ok {
			consider(t.Sub(epoch), "due")
		}
		if s.running {
			consider(s.runEnd, "built")
		}
		if next < 0 {
			return
		}
		if next > s.now {
			s.now = next
		}
		switch kind {
		case "found":
			sv := s.saves[0]
			s.saves = s.saves[1:]
			s.logf("found %v", sv.paths)
			s.change(sv.paths)
		case "due":
			id, batch, _ := s.p.start()
			s.logf("build %d starts: %v", id, batch)
			s.started = append(s.started, s.now)
			s.running, s.runID, s.runEnd, s.runBatch = true, id, s.now+s.buildTime, batch
		case "built":
			s.running = false
			// The last scan before a restart finds what was saved but
			// not yet found.
			var late []string
			for len(s.saves) > 0 && s.saves[0].at <= s.now {
				late = append(late, s.saves[0].paths...)
				s.saves = s.saves[1:]
			}
			if len(late) > 0 {
				s.logf("the last scan finds %v", late)
				s.change(late)
			}
			if !s.p.done(s.runID) {
				s.logf("build %d thrown away", s.runID)
				s.discards++
				continue
			}
			s.logf("restart on build %d", s.runID)
			s.restarts = append(s.restarts, s.runBatch)
		}
	}
	s.t.Fatal("the simulation did not settle")
}

func (s *sim) failf(format string, args ...any) {
	s.t.Helper()
	s.t.Errorf(format+"\n%s", append(args, strings.Join(s.log, "\n"))...)
}

func newSim(t *testing.T, saves ...save) *sim {
	return &sim{t: t, buildTime: 800 * time.Millisecond, foundAfter: 20 * time.Millisecond, saves: saves}
}

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestPlanOneSave(t *testing.T) {
	s := newSim(t, save{0, []string{"a.go"}})
	s.run()
	if !slices.Equal(s.started, []time.Duration{ms(20) + quietTime}) || len(s.restarts) != 1 {
		s.failf("started %v, restarts %d; want one build quietTime after the save was found", s.started, len(s.restarts))
	}
}

// A formatter that rewrites the file 150 ms after the save: the build
// that started meanwhile is cancelled, and the app restarts once, on both.
func TestPlanFormatterResave(t *testing.T) {
	s := newSim(t, save{0, []string{"a.go"}}, save{ms(150), []string{"a.go", "b.go"}})
	s.run()
	if len(s.restarts) != 1 || s.cancels != 1 || len(s.started) != 2 {
		s.failf("restarts %d, cancels %d, builds %d; want 1, 1, 2", len(s.restarts), s.cancels, len(s.started))
	}
	if !slices.Equal(s.restarts[0], []string{"a.go", "b.go"}) {
		s.failf("restarted on %v, want the changes of both saves", s.restarts[0])
	}
}

// A save while a build runs cancels it, and one more build follows.
func TestPlanChangeDuringBuild(t *testing.T) {
	s := newSim(t, save{0, []string{"a.go"}}, save{ms(400), []string{"c.go"}})
	s.run()
	if s.cancels != 1 || len(s.started) != 2 || len(s.restarts) != 1 {
		s.failf("cancels %d, builds %d, restarts %d; want 1, 2, 1", s.cancels, len(s.started), len(s.restarts))
	}
	if want := ms(420) + quietTime; s.started[1] != want {
		s.failf("the second build started at %v, want %v: the usual wait after the change", s.started[1], want)
	}
}

// A burst, as a branch switch or a save-all makes, is one build, after
// the longer wait for many files.
func TestPlanBurst(t *testing.T) {
	var saves []save
	for i := range 20 {
		saves = append(saves, save{ms(i * 3), []string{fmt.Sprintf("f%02d.go", i)}})
	}
	s := newSim(t, saves...)
	s.run()
	if len(s.started) != 1 || len(s.restarts) != 1 || len(s.restarts[0]) != 20 {
		s.failf("builds %d, restarts %v; want one build of all 20", len(s.started), s.restarts)
	}
	if want := ms(57+20) + quietTimeMany; s.started[0] != want {
		s.failf("the build started at %v, want %v", s.started[0], want)
	}
}

// A stream of changes that never goes quiet starts a build when the cap
// is reached; changes keep cancelling builds while it lasts, and the app
// restarts once it ends, on everything.
func TestPlanStream(t *testing.T) {
	var saves []save
	for i := range 100 { // 3 s, every 30 ms
		saves = append(saves, save{ms(i * 30), []string{fmt.Sprintf("f%03d.go", i%7)}})
	}
	s := newSim(t, saves...)
	s.run()
	if len(s.started) == 0 || s.started[0] != ms(20)+debounceCap {
		s.failf("builds started at %v, want the first at the cap, %v", s.started, ms(20)+debounceCap)
	}
	if len(s.restarts) != 1 || len(s.restarts[0]) != 7 {
		s.failf("restarts %v, want one, on all 7 files", s.restarts)
	}
	last := saves[len(saves)-1].at + s.foundAfter
	if got := s.started[len(s.started)-1]; got <= last || got > last+quietTimeMany {
		s.failf("the last build started at %v, want after the last change, %v, by at most %v", got, last, quietTimeMany)
	}
}

// A save in the last moments of a build, not yet found when it ends, is
// found by the last scan: that build is thrown away and the app restarts
// once, on a build that has the save.
func TestPlanStaleBuildDiscarded(t *testing.T) {
	s := newSim(t, save{0, []string{"a.go"}}, save{ms(20) + quietTime + ms(795), []string{"b.go"}})
	s.run()
	if s.discards != 1 || len(s.restarts) != 1 || !slices.Equal(s.restarts[0], []string{"a.go", "b.go"}) {
		s.failf("discards %d, restarts %v; want 1 and one restart on a.go and b.go", s.discards, s.restarts)
	}
}

func TestPlanIdle(t *testing.T) {
	var p plan
	if !p.idle() {
		t.Error("a new plan is not idle")
	}
	p.change(epoch, []string{"a.go"})
	if p.idle() {
		t.Error("idle with a change pending")
	}
	id, _, _ := p.start()
	if p.idle() {
		t.Error("idle while building")
	}
	if p.change(epoch, nil) {
		t.Error("no change cancelled the build")
	}
	if !p.done(id) || !p.idle() {
		t.Error("not idle after the build")
	}
	if p.done(id) {
		t.Error("a build was done twice")
	}
}
