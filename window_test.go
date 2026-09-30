package ggui

import (
	"testing"
	"time"
)

func TestConfigOptionsMapWindowFields(t *testing.T) {
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
