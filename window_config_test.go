package ggui

import (
	"image"
	"testing"

	"github.com/ironpark/ggfx"
	"github.com/ironpark/ggui/runtime"
)

func TestWindowSettersBeforeOpenShapeTheConfig(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	a.SetSizeLimits(Sz(200, 100), Sz(0, 900))
	a.SetResizable(true)
	a.SetFrameless(true)
	a.SetAlwaysOnTop(true)
	a.SetIcon(icon)
	a.Maximize()
	o := a.cfg.options()
	if o.MinWidth != 200 || o.MinHeight != 100 || o.MaxHeight != 900 {
		t.Fatalf("size limits not carried to the window options: %+v", o)
	}
	if !a.cfg.Resizable || !o.Undecorated || !o.Floating || !o.Maximized || len(a.cfg.Icon) != 1 {
		t.Fatalf("config %+v, options %+v; want resizable, frameless, on top, maximized, with the icon", a.cfg, o)
	}
	a.Restore()
	a.Hide()
	if a.cfg.Maximized || a.Visible() {
		t.Fatalf("after Restore and Hide: maximized %v, visible %v", a.cfg.Maximized, a.Visible())
	}
	a.Show()
	if !a.Visible() {
		t.Fatal("Show before the window opened left it hidden")
	}
	// The rest wait for a native window and do nothing before one.
	a.Minimize()
	a.SetFullscreen(true)
	a.Focus()
	a.RequestAttention()
	if a.Screen() != (Screen{}) || Screens() != nil {
		t.Fatal("a window that is not open reports a screen")
	}
}

func TestWindowReadablesStartFromTheConfig(t *testing.T) {
	t.Parallel()
	a := New(Config{Width: 320, Height: 240, Maximized: true, Unfocused: true}, func() Widget { return Box() })
	defer a.Close()
	if Untrack(a.Focused().Get) || Untrack(a.Viewport().Get) != Sz(320, 240) || Untrack(a.State().Get) != WindowMaximized {
		t.Fatalf("focused %v, viewport %v, state %v; want unfocused, 320x240, maximized",
			Untrack(a.Focused().Get), Untrack(a.Viewport().Get), Untrack(a.State().Get))
	}
	b := New(Config{}, func() Widget { return Box() })
	defer b.Close()
	if !Untrack(b.Focused().Get) || Untrack(b.State().Get) != WindowNormal {
		t.Fatal("a window configured with nothing does not start focused and normal")
	}
}

func TestAppSetClipboardAndDialogsFallBackToThePlatform(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	clip := &runtime.MemoryClipboard{}
	dialogs := &runtime.StubFilePicker{}
	if a.SetClipboard(clip).SetDialogs(dialogs) != a {
		t.Fatal("the setters do not chain")
	}
	if a.clipboard != clip || a.dialogs != dialogs {
		t.Fatal("the main window did not take the clipboard and dialogs")
	}
	a.SetClipboard(nil).SetDialogs(nil)
	if a.clipboard == nil || a.clipboard == runtime.Clipboard(clip) || a.dialogs == nil || a.dialogs == runtime.Dialogs(dialogs) {
		t.Fatal("nil did not restore the platform clipboard and dialogs")
	}
}

func TestAppQuitEndsRunAtTheNextEvent(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	extra, err := a.OpenWindow(Config{}, func() Widget { return Box() })
	if err != nil {
		t.Fatal(err)
	}
	// Events for no window, and a wake before the app runs, do nothing.
	if err := a.handleEvent(ggfx.FrameEvent{}); err != nil {
		t.Fatalf("an event for no window gave %v", err)
	}
	if err := a.handleEvent(ggfx.WakeEvent{}); err != nil {
		t.Fatalf("a wake before Run gave %v", err)
	}
	if a.Closed() {
		t.Fatal("an ignored event closed the app")
	}
	a.Quit()
	if err := a.handleEvent(ggfx.WakeEvent{}); err != ggfx.Termination {
		t.Fatalf("the event after Quit gave %v, want Termination", err)
	}
	if !a.Closed() || !extra.Closed() || len(a.Windows()) != 0 {
		t.Fatal("Quit left a window open")
	}
}

func TestEventWindowIsTheWindowEachEventNames(t *testing.T) {
	t.Parallel()
	nw := new(ggfx.Window)
	for _, ev := range []ggfx.Event{
		ggfx.FrameEvent{Window: nw}, ggfx.ResizeEvent{Window: nw}, ggfx.FocusEvent{Window: nw},
		ggfx.WindowStateEvent{Window: nw}, ggfx.CloseEvent{Window: nw}, ggfx.KeyEvent{Window: nw}, ggfx.TextEvent{Window: nw},
		ggfx.CompositionEvent{Window: nw}, ggfx.MouseMoveEvent{Window: nw}, ggfx.MouseButtonEvent{Window: nw},
		ggfx.ScrollEvent{Window: nw}, ggfx.TouchEvent{Window: nw}, ggfx.DragEvent{Window: nw}, ggfx.DropEvent{Window: nw},
	} {
		if got := eventWindow(ev); got != nw {
			t.Errorf("eventWindow(%T) = %p, want the event's window", ev, got)
		}
	}
	for _, ev := range []ggfx.Event{ggfx.StartEvent{}, ggfx.WakeEvent{}} {
		if got := eventWindow(ev); got != nil {
			t.Errorf("eventWindow(%T) = %p, want none", ev, got)
		}
	}
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	if a.windowFor(nw) != nil {
		t.Fatal("a native window the app never made was matched to one of its windows")
	}
}

func TestWindowStateFollowsTheStateEvents(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	for _, c := range []struct {
		from ggfx.WindowState
		want WindowState
	}{
		{ggfx.WindowStateMaximized, WindowMaximized},
		{ggfx.WindowStateMinimized, WindowMinimized},
		{ggfx.WindowStateFullscreen, WindowFullscreen},
		{ggfx.WindowStateNormal, WindowNormal},
	} {
		if err := a.handle(ggfx.WindowStateEvent{State: c.from}); err != nil {
			t.Fatal(err)
		}
		a.runPosted()
		if got := Untrack(a.State().Get); got != c.want {
			t.Errorf("after a WindowStateEvent of %d, state %v; want %v", c.from, got, c.want)
		}
	}
}

func TestAWindowBuildsTheTreeOnlyOnceSomethingReadsIt(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	defer a.Close()
	if a.semWanted.Load() {
		t.Fatal("a window nobody asked builds its accessibility tree")
	}
	c := &Canvas{}
	c.fs().semOff = true
	c.Leaf(Rct(Pt(0, 0), Sz(10, 10)), Node{Role: RoleText, Name: "x"})
	if len(c.fs().sem) != 0 {
		t.Fatal("a frame with the tree off recorded a node")
	}
	a.canvas.fs().semOff = true
	a.Perform(NodeID{Role: RoleButton}, Action{Kind: ActionPress})
	if a.performs.Load() != 1 {
		t.Fatal("a pending Perform does not keep the tree on")
	}
	a.runPosted()
	if !a.hasPosted() {
		t.Fatal("Perform ran against a frame that built no tree instead of waiting for one")
	}
	a.canvas.fs().semOff = false // the frame the pending Perform asked for
	a.runPosted()
	if a.hasPosted() || a.performs.Load() != 0 {
		t.Fatal("Perform kept waiting once a frame built the tree")
	}
	if a.semWanted.Load() {
		t.Fatal("a Perform left the tree on after it ran")
	}
	b := New(Config{}, func() Widget { return Box() })
	defer b.Close()
	b.Semantics()
	if !b.semWanted.Load() {
		t.Fatal("Semantics did not turn the tree on")
	}
}
