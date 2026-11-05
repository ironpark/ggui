package ggui

import (
	"image"
	"testing"
	"time"

	"github.com/ironpark/ggui/runtime"
	"github.com/ironpark/ggwd"
)

func TestConfigOptionsMapWindowFields(t *testing.T) {
	t.Parallel()
	at := Pt(10, 20)
	o := Config{Title: "t", Width: 300, Height: 200, MinWidth: 100, MaxHeight: 400, Position: &at,
		Frameless: true, AlwaysOnTop: true, Transparent: true, Hidden: true, Maximized: true, Unfocused: true}.options()
	if !o.Undecorated || !o.Floating || !o.Transparent || !o.Hidden || !o.Maximized || !o.Unfocused {
		t.Fatalf("flags not carried over: %+v", o)
	}
	if o.Position == nil || o.Position.X != 10 || o.Position.Y != 20 || o.MinWidth != 100 || o.MaxHeight != 400 {
		t.Fatalf("geometry not carried over: %+v", o)
	}
}

func TestWindowControlBeforeOpenChangesConfig(t *testing.T) {
	t.Parallel()
	a := New(Config{Title: "a"}, func() Widget { return Box() })
	defer a.Close()
	a.SetTitle("b")
	a.SetSize(Sz(320, 240))
	a.SetPosition(Pt(5, 6))
	a.Hide()
	if a.Title() != "b" || a.Size() != Sz(320, 240) || a.Position() != Pt(5, 6) || a.Visible() {
		t.Fatalf("got %q %v %v visible=%t", a.Title(), a.Size(), a.Position(), a.Visible())
	}
}

func TestOpenWindowBeforeRunWaits(t *testing.T) {
	t.Parallel()
	a := New(Config{}, func() Widget { return Box() })
	w, err := a.OpenWindow(Config{Title: "second"}, func() Widget { return Box() })
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Windows(); len(got) != 2 || got[0] != a.Window || got[1] != w || w.App() != a {
		t.Fatalf("windows = %v", got)
	}
	w.Close()
	if got := a.Windows(); len(got) != 1 {
		t.Fatalf("closed window still listed: %v", got)
	}
	a.Close()
	if !a.Closed() || len(a.Windows()) != 0 {
		t.Fatal("Close left a window open")
	}
}

func TestUseHostFindsTheProbe(t *testing.T) {
	t.Parallel()
	var built, clicked Host
	p := ProbeBuilder(func() Widget {
		built = UseHost()
		return Pointer(Box().Size(20, 20)).OnTap(func() { clicked = UseHost() })
	}, Sz(100, 100))
	defer p.Close()
	p.Click(Pt(5, 5))
	if built != Host(p) || clicked != Host(p) {
		t.Fatalf("UseHost = %v, %v; want the probe", built, clicked)
	}
	if useWindowIn(p) != nil {
		t.Fatal("UseWindow under a probe is not nil")
	}
}

// useWindowIn runs UseWindow inside p's tree.
func useWindowIn(p *Probe) (w *Window) {
	p.Post(func() { w = UseWindow() })
	p.Frame()
	return w
}

func TestAsyncDeliversOnTheUIThread(t *testing.T) {
	t.Parallel()
	result := State("")
	p := NewProbe(TextOf(result), Sz(100, 20))
	defer p.Close()
	p.Frame()
	started := make(chan struct{})
	p.Post(func() {
		Async(func() (string, error) { <-started; return "done", nil }, func(s string, _ error) { result.Set(s) })
	})
	p.Frame()
	close(started)
	deadline := time.Now().Add(2 * time.Second)
	for Untrack(result.Get) == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		p.Frame()
	}
	if got := Untrack(result.Get); got != "done" {
		t.Fatalf("result = %q", got)
	}
}

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
	if err := a.handleEvent(ggwd.FrameEvent{}); err != nil {
		t.Fatalf("an event for no window gave %v", err)
	}
	if err := a.handleEvent(ggwd.WakeEvent{}); err != nil {
		t.Fatalf("a wake before Run gave %v", err)
	}
	if a.Closed() {
		t.Fatal("an ignored event closed the app")
	}
	a.Quit()
	if err := a.handleEvent(ggwd.WakeEvent{}); err != ggwd.Termination {
		t.Fatalf("the event after Quit gave %v, want Termination", err)
	}
	if !a.Closed() || !extra.Closed() || len(a.Windows()) != 0 {
		t.Fatal("Quit left a window open")
	}
}

func TestEventWindowIsTheWindowEachEventNames(t *testing.T) {
	t.Parallel()
	nw := new(ggwd.Window)
	for _, ev := range []ggwd.Event{
		ggwd.FrameEvent{Window: nw}, ggwd.ResizeEvent{Window: nw}, ggwd.MoveEvent{Window: nw}, ggwd.FocusEvent{Window: nw},
		ggwd.WindowStateEvent{Window: nw}, ggwd.CloseEvent{Window: nw}, ggwd.ClosedEvent{Window: nw}, ggwd.KeyEvent{Window: nw},
		ggwd.TextEvent{Window: nw}, ggwd.CompositionEvent{Window: nw}, ggwd.MouseMoveEvent{Window: nw},
		ggwd.MouseEnterEvent{Window: nw}, ggwd.MouseButtonEvent{Window: nw},
		ggwd.ScrollEvent{Window: nw}, ggwd.TouchEvent{Window: nw}, ggwd.DragEvent{Window: nw}, ggwd.DropEvent{Window: nw},
	} {
		if got := eventWindow(ev); got != nw {
			t.Errorf("eventWindow(%T) = %p, want the event's window", ev, got)
		}
	}
	for _, ev := range []ggwd.Event{ggwd.StartEvent{}, ggwd.WakeEvent{}, ggwd.SystemColorModeEvent{}, ggwd.MonitorEvent{}} {
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
		from ggwd.WindowState
		want WindowState
	}{
		{ggwd.WindowStateMaximized, WindowMaximized},
		{ggwd.WindowStateMinimized, WindowMinimized},
		{ggwd.WindowStateFullscreen, WindowFullscreen},
		{ggwd.WindowStateNormal, WindowNormal},
	} {
		if err := a.handle(ggwd.WindowStateEvent{State: c.from}); err != nil {
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

func TestAppUsesNativeModifierSnapshot(t *testing.T) {
	t.Parallel()
	a := &Window{}
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true, Modifiers: ggwd.Mods{Meta: true}})
	if !a.takeInput().mods.Meta {
		t.Fatal("native modifier snapshot was lost without a separate modifier key event")
	}
}

func TestAppKeepsModifierOnQueuedKeyPress(t *testing.T) {
	t.Parallel()
	a := &Window{}
	for _, e := range []ggwd.KeyEvent{
		{Key: ggwd.KeyMetaLeft, Pressed: true},
		{Key: ggwd.KeyO, Pressed: true},
		{Key: ggwd.KeyO},
		{Key: ggwd.KeyMetaLeft},
	} {
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	f := a.takeInput()
	if !f.mods.Meta {
		t.Fatal("Cmd+O lost its modifier when Cmd was released before the frame")
	}
	if a.keyboard.Mods().Meta {
		t.Fatal("released modifier remains held")
	}
}

func TestPointerEventsCarryTheirModifiers(t *testing.T) {
	t.Parallel()
	for _, e := range []ggwd.Event{
		ggwd.MouseButtonEvent{Button: ggwd.MouseButtonLeft, Pressed: true, Modifiers: ggwd.Mods{Shift: true}},
		ggwd.ScrollEvent{Y: -1, Modifiers: ggwd.Mods{Shift: true}},
	} {
		a := &Window{}
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
		if !a.takeInput().mods.Shift {
			t.Errorf("%T: a modifier held as it arrived, with no KeyEvent for it, was lost", e)
		}
	}
}

// A modifier whose release the window system never reported, taken by a
// system shortcut or an input source switch, is let go by the next event
// that says it is not held.
func TestLostModifierReleaseIsForgotten(t *testing.T) {
	t.Parallel()
	for _, later := range []ggwd.Event{
		ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true},
		ggwd.MouseButtonEvent{Button: ggwd.MouseButtonLeft, Pressed: true},
		ggwd.ScrollEvent{Y: -1},
	} {
		a := &Window{}
		if err := a.handle(ggwd.KeyEvent{Key: ggwd.KeyShiftLeft, Pressed: true, Modifiers: ggwd.Mods{Shift: true}}); err != nil {
			t.Fatal(err)
		}
		a.takeInput()
		if err := a.handle(later); err != nil {
			t.Fatal(err)
		}
		if a.takeInput().mods.Shift || a.keyboard.Mods().Shift {
			t.Errorf("after %T Shift is still held", later)
		}
	}
	// One the event says is held stays held.
	a := &Window{}
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyShiftLeft, Pressed: true, Modifiers: ggwd.Mods{Shift: true}})
	a.handle(ggwd.KeyEvent{Key: ggwd.KeyA, Pressed: true, Modifiers: ggwd.Mods{Shift: true}})
	a.takeInput()
	if !a.keyboard.Mods().Shift {
		t.Error("a held Shift was let go")
	}
}

// The fingers lifting reaches the widgets though it moves nothing, and is
// not lost to the momentum that follows within the frame.
func TestScrollPhaseReachesTheFrame(t *testing.T) {
	t.Parallel()
	a := &Window{}
	for _, e := range []ggwd.ScrollEvent{
		{X: -1, Phase: ggwd.ScrollPhaseChanged},
		{Phase: ggwd.ScrollPhaseEnded},
		{X: -0.5, Phase: ggwd.ScrollPhaseMomentum},
	} {
		if err := a.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel.X != -1.5 {
		t.Fatalf("phase %v wheel %v", f.phase, f.wheel)
	}
	a.handle(ggwd.ScrollEvent{Phase: ggwd.ScrollPhaseEnded})
	if f := a.takeInput(); f.phase != ScrollPhaseEnded || f.wheel != (Point{}) {
		t.Fatalf("a lift alone: phase %v wheel %v", f.phase, f.wheel)
	}
}

func TestTheInspectorChordReachesNoWidget(t *testing.T) {
	t.Parallel()
	w := &Window{cfg: Config{Inspector: "f1"}, inspectChord: MustChord("f1")}
	for _, e := range []ggwd.KeyEvent{
		{Key: ggwd.KeyF1, Pressed: true},
		{Key: ggwd.KeyF1, Pressed: true, Repeat: true},
		{Key: ggwd.KeyA, Pressed: true},
	} {
		if err := w.handle(e); err != nil {
			t.Fatal(err)
		}
	}
	if keys := w.takeInput().keys; len(keys) != 1 || keys[0] != ggwd.KeyA {
		t.Fatalf("keys = %v; want only A, the inspector's F1 kept from the widgets", keys)
	}
}
