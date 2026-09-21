package main

import (
	"errors"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// TestSettings edits through the lenses, saves through a worker and waits
// for the posted result to land on the probe's frame.
func TestSettings(t *testing.T) {
	var stored Profile
	fail := false
	m := newModel(func(p Profile) error {
		if fail {
			return errors.New("offline")
		}
		stored = p
		return nil
	})
	var p *ggui.Probe
	p = ggui.ProbeBuilder(func() ggui.Widget { return m.build(p.Post) }, ggui.Sz(520, 480))
	defer p.Close()
	p.Frame()

	p.Tap("Email me about activity")
	draft := ggui.Untrack(m.Draft.Get)
	if draft.Notify {
		t.Fatal("switch did not write through the lens")
	}
	if ggui.Untrack(m.Saved.Get).Notify != true {
		t.Fatal("saved copy changed before saving")
	}
	p.Tap("Save")
	waitFor(t, p, func() bool { return !ggui.Untrack(m.Saving.Get) })
	if stored != draft || ggui.Untrack(m.Saved.Get) != draft {
		t.Fatalf("persisted %+v, saved %+v, want %+v", stored, ggui.Untrack(m.Saved.Get), draft)
	}

	m.Draft.Set(Profile{Name: "Ada", Email: "not-an-address"})
	before := ggui.Untrack(m.Saved.Get)
	// Save is disabled while the address is invalid: a disabled button
	// takes no input, so it has no region for Tap to find.
	if _, ok := p.Find("Save"); ok {
		p.Tap("Save")
	}
	m.save(p.Post) // the shortcut path checks the same validation
	p.Frame()
	if ggui.Untrack(m.Saving.Get) || ggui.Untrack(m.Saved.Get) != before {
		t.Fatal("an invalid draft was saved")
	}
	p.Tap("Reset")
	if ggui.Untrack(m.Draft.Get) != before {
		t.Fatal("reset did not restore the saved copy")
	}

	fail = true
	m.Draft.Set(Profile{Name: "Grace", Email: "grace@example.com"})
	p.Tap("Save")
	waitFor(t, p, func() bool { return !ggui.Untrack(m.Saving.Get) })
	if ggui.Untrack(m.Status.Get) != "Could not save: offline" {
		t.Fatalf("status = %q after a failed save", ggui.Untrack(m.Status.Get))
	}
}

// waitFor runs frames until cond holds: posted work lands before a frame.
func waitFor(t *testing.T, p *ggui.Probe, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the worker")
		}
		time.Sleep(5 * time.Millisecond)
		p.Frame()
	}
}
