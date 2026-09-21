package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

// TestControls drives the catalog headlessly: the theme switch flips its
// signal and changing the selection swaps the demo without a restart.
func TestControls(t *testing.T) {
	m := newModel("carousel-demo", false)
	p := ggui.ProbeBuilder(m.build, ggui.Sz(680, 700))
	defer p.Close()

	p.Tap("Dark theme")
	if !ggui.Untrack(m.Dark.Get) {
		t.Fatal("dark theme switch did not flip the signal")
	}
	if _, ok := p.Find("Slides"); !ok {
		t.Fatal("the carousel demo is not in the tree")
	}
	m.Selected.Set("input-otp-separator")
	p.Frame()
	if _, ok := p.Find("Verification code"); !ok {
		t.Fatal("changing the selection did not swap in the OTP demo")
	}
	if _, ok := p.Find("Slides"); ok {
		t.Fatal("the carousel demo survived the selection change")
	}
}
