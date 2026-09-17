package ggui

import "testing"

func TestAnnounceQueuesWithoutAConsumer(t *testing.T) {
	a := New(Config{}, func() Widget { return Box() })
	a.Announce("Saved", Polite)
	a.Announce("", Assertive) // nothing to say, nothing queued
	a.Announce("Disk full", Assertive)
	got := a.takeAnnouncements()
	if len(got) != 2 || got[0] != (Announcement{Text: "Saved"}) ||
		got[1] != (Announcement{Text: "Disk full", Politeness: Assertive}) {
		t.Fatalf("queue = %v", got)
	}
	if rest := a.takeAnnouncements(); rest != nil {
		t.Errorf("taking the queue left %v behind", rest)
	}
}

func TestPerformDoesNotWaitForTheUIGoroutine(t *testing.T) {
	// An action is queued and returns at once: the platform thread that
	// asked is blocked, and so may the UI goroutine be, so nothing may
	// wait on anything. Without a frame, nothing has run.
	a := New(Config{}, func() Widget { return Box() })
	done := make(chan struct{})
	go func() {
		a.Perform(NodeID{Rect: Rct(Pt(0, 0), Sz(10, 10))}, Action{Kind: ActionPress})
		close(done)
	}()
	<-done
	a.postMu.Lock()
	queued := len(a.posted)
	a.postMu.Unlock()
	if queued != 1 {
		t.Fatalf("%d posted, want the action waiting for the UI goroutine", queued)
	}
}

func TestPerformFocusesAndScrollsIntoView(t *testing.T) {
	w := &twice{}
	w.Role, w.Name = RoleTextField, "Name"
	tall := Column(Box().Size(40, 200), w)
	p := NewProbe(Scroll(tall), Sz(100, 60))
	defer p.Close()
	field, ok := p.Semantics().Find(RoleTextField, "Name")
	if !ok {
		t.Fatal("no field")
	}
	if !field.Offscreen {
		t.Fatal("the field should start out of view")
	}
	p.Perform(field.ID, Action{Kind: ActionFocus})
	if !p.Focused() {
		t.Fatal("the field did not take focus")
	}
	// Focus moved by the keyboard reveals, and the action goes through the
	// same path, so the scroll came along.
	if now, _ := p.Semantics().Find(RoleTextField, "Name"); now.Offscreen {
		t.Error("the field was focused but not scrolled into view")
	}
}
