package ggui

import (
	"testing"

	"github.com/ironpark/ggui/internal/reactive"
)

// Each Probe builds its tree in a reactive runtime of its own, so a frame
// of one runs none of the other's bindings, even when both read the same
// signal. User effects were already kept to their own loop; the widget
// bindings a tree makes with Observe were not.
func TestProbesDoNotRunEachOthersEffects(t *testing.T) {
	n := State(0)
	var inA, inB int
	a := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); inA++ })
	})
	defer a.Close()
	b := ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); inB++ })
	})
	defer b.Close()
	a.Frame()
	b.Frame()
	inA, inB = 0, 0

	n.Set(1)
	a.Frame()
	if inA != 1 || inB != 0 {
		t.Fatalf("a's frame ran a=%d b=%d, want 1 0", inA, inB)
	}
	b.Frame()
	if inA != 1 || inB != 1 {
		t.Fatalf("b's frame ran a=%d b=%d, want 1 1", inA, inB)
	}
}

// A probe a test forgets to close keeps its effects to itself: the next
// probe's frames never run them.
func TestALeakedProbeStaysOutOfTheNextOnesFrames(t *testing.T) {
	n := State(0)
	leaked := 0
	ProbeBuilder(func() Widget { return Box() }, Sz(10, 10)).Setup(func() {
		reactive.Observe(func() { n.Get(); leaked++ })
	}).Frame()
	leaked = 0

	p := NewProbe(Box(), Sz(100, 20))
	defer p.Close()
	n.Set(1)
	p.Frame()
	if leaked != 0 {
		t.Fatalf("a leaked probe's effect ran %d times in another probe's frame", leaked)
	}
}

// A widget built before its probe, outside any tree, still follows its
// signals: what has no owner belongs to the process's runtime, which every
// probe's frame flushes too.
func TestAWidgetBuiltBeforeItsProbeStillFollowsItsSignals(t *testing.T) {
	v := State("a")
	in := TextInput(v)
	p := NewProbe(in, Sz(200, 30))
	defer p.Close()
	p.Frame()
	v.Set("changed")
	p.Frame()
	if in.ed.Text != "changed" {
		t.Fatalf("editor shows %q, want the signal's new value", in.ed.Text)
	}
}
