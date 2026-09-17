package ui

import (
	"github.com/ironpark/ggui"
	"testing"
	"time"
)

func TestToastExitIsAnimatedAndInert(t *testing.T) {
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	id := host.Push(Toast("Saved", "Your changes are stored.").Duration(0))
	p.Advance(0)
	p.Advance(ggui.DefaultTheme().MotionSlow)
	host.Dismiss(id)
	p.Advance(0)
	if host.Len() != 0 || len(host.entries) != 1 {
		t.Fatal("dismiss should remove the active notice but retain its exit frame")
	}
	if _, ok := p.Find("Dismiss Saved"); ok {
		t.Fatal("exiting toast is still interactive")
	}
	p.Advance(ggui.DefaultTheme().MotionSlow / 2)
	v := host.entries[0].motion.Value(ggui.Now())
	if v <= 0 || v >= 1 {
		t.Fatalf("exit progress = %v, want an intermediate value", v)
	}
	p.Advance(ggui.DefaultTheme().MotionSlow)
	if len(host.entries) != 0 {
		t.Fatal("exit did not release the notice")
	}
}

func TestToastReducedMotionAndQueueReplacement(t *testing.T) {
	host := NewToaster().Limit(2)
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, host), ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	first := host.Push(Toast("First", "").Duration(0))
	host.Push(Toast("Second", "").Duration(0))
	p.Advance(0)
	host.Dismiss(first)
	host.Push(Toast("Third", "").Duration(0))
	if host.Len() != 2 {
		t.Fatal("exiting notice evicted an active notice")
	}
	host.Dismiss(host.entries[0].id)
	p.Advance(time.Millisecond)
	if len(host.entries) != 1 {
		t.Fatal("reduced motion retained an exit animation")
	}
}

func TestToastLiveRegionKeepsIdentityWhileMoving(t *testing.T) {
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	host.Push(Toast("Saved", "Changes stored").Duration(0))
	before, ok := p.Semantics().Find(ggui.RoleStatus, "Saved")
	if !ok {
		t.Fatal("missing live region")
	}
	p.Advance(120 * time.Millisecond)
	after, ok := p.Semantics().Find(ggui.RoleStatus, "Saved")
	if !ok || before.ID.ID == nil || before.ID.ID != after.ID.ID {
		t.Fatal("animation changed the live region identity")
	}
	if before.Full.Origin == after.Full.Origin {
		t.Fatal("notice did not slide into place")
	}
}
