package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

func TestToastExitIsAnimatedAndInert(t *testing.T) {
	t.Parallel()
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	id := host.Push(Toast("Saved", "Your changes are stored.").Duration(0))
	p.Advance(0)
	p.Advance(toastSlide)
	host.Dismiss(id)
	p.Advance(0)
	if host.Len() != 0 || len(host.entries) != 1 {
		t.Fatal("dismiss should remove the active notice but retain its exit frame")
	}
	if _, ok := p.Find("Dismiss Saved"); ok {
		t.Fatal("exiting toast is still interactive")
	}
	p.Advance(toastSlide / 5)
	v := host.entries[0].enter.Value(ggui.Now())
	if v <= 0 || v >= 1 {
		t.Fatalf("exit progress = %v, want an intermediate value", v)
	}
	p.Advance(toastSlide)
	if len(host.entries) != 0 {
		t.Fatal("exit did not release the notice")
	}
}

func TestToastReducedMotionAndQueueReplacement(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

// settle runs the frames a stack of notices takes to come to rest.
func settleToasts(p *ggui.Probe) {
	for range 4 {
		p.Advance(toastSlide / 2)
	}
}

func toastRect(t *testing.T, p *ggui.Probe, title string) ggui.Rect {
	t.Helper()
	n, ok := p.Semantics().Find(ggui.RoleStatus, title)
	if !ok {
		t.Fatalf("no notice titled %q", title)
	}
	return n.Full
}

func TestToastTypesPickTheLeadingIcon(t *testing.T) {
	t.Parallel()
	host := NewToaster().Limit(8)
	p := ggui.NewProbe(host, ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	for _, m := range []ToastMessage{
		Toast("Plain", ""), Toast("Ok", "").Success(), Toast("Note", "").Info(), Toast("Careful", "").Warning(),
		Toast("Broke", "").Error(), Toast("Old", "").Destructive(), Toast("Wait", "").Loading(),
	} {
		host.Push(m)
	}
	p.Advance(0)
	want := []ToastType{ToastPlain, ToastSuccess, ToastInfo, ToastWarning, ToastError, ToastError, ToastLoading}
	for i, e := range host.entries {
		if e.panel.kind != want[i] {
			t.Fatalf("%s is of type %v, want %v", e.title, e.panel.kind, want[i])
		}
		// The icon is the row's first part, and only a typed notice has one.
		_, marked := e.panel.parts[0].(*toastMark)
		if marked != (want[i] != ToastPlain) {
			t.Fatalf("%s leads with an icon: %v", e.title, marked)
		}
	}
	if last := host.entries[len(host.entries)-1]; !last.persistent {
		t.Fatal("a loading notice counts down")
	}
}

func TestToastStackCollapsesBehindTheNewestAndSpreadsUnderThePointer(t *testing.T) {
	t.Parallel()
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(600, 500))
	defer p.Close()
	p.Advance(0)
	for _, title := range []string{"First", "Second", "Third"} {
		host.Push(Toast(title, "Stays until dismissed").Duration(0))
		p.Advance(0)
	}
	settleToasts(p)
	front, mid, back := toastRect(t, p, "Third"), toastRect(t, p, "Second"), toastRect(t, p, "First")
	if front.Size.W != toastWidth || front.Origin.X+front.Size.W != 600-16 || front.Origin.Y+front.Size.H != 500-16 {
		t.Fatalf("the front notice is at %v, want %v wide at the bottom right", front, toastWidth)
	}
	if got, want := mid.Size.W, toastWidth*.9; got != want {
		t.Fatalf("the second notice is %v wide, want %v", got, want)
	}
	if got, want := back.Size.W, toastWidth*.8; got < want-.001 || got > want+.001 {
		t.Fatalf("the third notice is %v wide, want %v", got, want)
	}
	if got := front.Origin.Y - mid.Origin.Y; got < 11.999 || got > 12.001 {
		t.Fatalf("the second notice peeks out by %v, want 12", got)
	}
	if got := mid.Origin.Y - back.Origin.Y; got < 11.999 || got > 12.001 {
		t.Fatalf("the third notice peeks out by %v, want 12", got)
	}
	if _, ok := p.Find("Dismiss First"); ok {
		t.Fatal("a notice behind the front one still takes input")
	}
	if _, ok := p.Find("Dismiss Third"); !ok {
		t.Fatal("the front notice has no close button")
	}

	p.Move(ggui.Pt(front.Origin.X+40, front.Origin.Y+front.Size.H/2))
	settleToasts(p)
	front, mid, back = toastRect(t, p, "Third"), toastRect(t, p, "Second"), toastRect(t, p, "First")
	for _, r := range []ggui.Rect{mid, back} {
		if r.Size.W != toastWidth {
			t.Fatalf("spread out, a notice is %v wide, want %v", r.Size.W, toastWidth)
		}
	}
	if got := front.Origin.Y - (mid.Origin.Y + mid.Size.H); got < 11.999 || got > 12.001 {
		t.Fatalf("spread out, the notices are %v apart, want 12", got)
	}
	if got := mid.Origin.Y - (back.Origin.Y + back.Size.H); got < 11.999 || got > 12.001 {
		t.Fatalf("spread out, the notices are %v apart, want 12", got)
	}
	if _, ok := p.Find("Dismiss First"); !ok {
		t.Fatal("spread out, the oldest notice cannot be closed")
	}
	// The gap between two notices belongs to the stack.
	p.Move(ggui.Pt(front.Origin.X+40, front.Origin.Y-6))
	settleToasts(p)
	if !host.expanded {
		t.Fatal("crossing the gap between two notices collapsed the stack")
	}
	p.Move(ggui.Pt(5, 5))
	settleToasts(p)
	if got := toastRect(t, p, "First").Size.W; got > toastWidth*.8+.001 {
		t.Fatalf("with the pointer gone the oldest notice is %v wide, want it collapsed", got)
	}
}

func TestToastComesInFromBelowAndRests(t *testing.T) {
	t.Parallel()
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(600, 500))
	defer p.Close()
	p.Advance(0)
	host.Push(Toast("Saved", "").Duration(0))
	p.Advance(0)
	p.Advance(toastSlide / 10)
	early := toastRect(t, p, "Saved")
	settleToasts(p)
	rest := toastRect(t, p, "Saved")
	if early.Origin.X != rest.Origin.X || early.Origin.Y <= rest.Origin.Y {
		t.Fatalf("the notice came from %v to %v, want straight up", early.Origin, rest.Origin)
	}
}

func TestToastPositionAnchorsTheStack(t *testing.T) {
	t.Parallel()
	host := NewToaster().Position(ToastTopLeft)
	p := ggui.NewProbe(host, ggui.Sz(600, 500))
	defer p.Close()
	p.Advance(0)
	host.Push(Toast("Older", "").Duration(0))
	p.Advance(0)
	host.Push(Toast("Newer", "").Duration(0))
	p.Advance(0)
	p.Advance(toastSlide / 10)
	early := toastRect(t, p, "Newer")
	settleToasts(p)
	front, back := toastRect(t, p, "Newer"), toastRect(t, p, "Older")
	if front.Origin != ggui.Pt(16, 16) {
		t.Fatalf("the front notice is at %v, want the top left", front.Origin)
	}
	if early.Origin.Y >= front.Origin.Y {
		t.Fatalf("the notice came in from %v, want from above %v", early.Origin.Y, front.Origin.Y)
	}
	if got := (back.Origin.Y + back.Size.H) - (front.Origin.Y + front.Size.H); got < 11.999 || got > 12.001 {
		t.Fatalf("the older notice peeks out below by %v, want 12", got)
	}
	host.Position(ToastBottomCenter)
	settleToasts(p)
	front = toastRect(t, p, "Newer")
	if got, want := front.Origin.X, (600-toastWidth)/2; got != want {
		t.Fatalf("centered, the notice starts at %v, want %v", got, want)
	}
}

func TestToastUpdateReplacesANoticeInPlace(t *testing.T) {
	t.Parallel()
	host := NewToaster()
	p := ggui.NewProbe(host, ggui.Sz(600, 500))
	defer p.Close()
	p.Advance(0)
	id := host.Push(Toast("Uploading", "").Loading())
	settleToasts(p)
	p.Advance(10 * time.Second)
	if host.Len() != 1 {
		t.Fatal("a loading notice expired")
	}
	if !host.Update(id, Toast("Uploaded", "report.pdf").Success().Duration(time.Second)) {
		t.Fatal("Update did not find the notice")
	}
	p.Advance(0)
	if _, ok := p.Semantics().Find(ggui.RoleStatus, "Uploaded"); !ok || host.Len() != 1 {
		t.Fatal("the notice was not replaced where it stood")
	}
	p.Advance(2 * time.Second)
	if host.Len() != 0 {
		t.Fatal("the updated notice did not expire")
	}
	if host.Update(id, Toast("Late", "")) {
		t.Fatal("Update reached a notice that had gone")
	}
}
