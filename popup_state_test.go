package ggui

import (
	"testing"
	"time"
)

func TestPopupOpenDetachesItsBindingAndOnCloseFiresOnce(t *testing.T) {
	t.Parallel()
	closes := 0
	open := State(false)
	p := Popup(Box().Size(60, 20), Box().Size(80, 40)).BindOpen(open).OnClose(func() { closes++ })
	pr := NewProbe(p, Sz(300, 300))
	defer pr.Close()
	pr.Frame()
	open.Set(true)
	if !p.IsOpen() {
		t.Fatal("writing the bound state did not open the popup")
	}
	p.Hide()
	if Untrack(open.Get) || closes != 1 {
		t.Fatalf("Hide left the binding %v and fired OnClose %d times; want false and 1", Untrack(open.Get), closes)
	}
	p.Toggle()
	if !Untrack(open.Get) {
		t.Fatal("Toggle on a closed popup did not write the binding open")
	}
	// Open detaches: the popup closes, and the state it was bound to stays.
	p.Open(false)
	if p.IsOpen() || !Untrack(open.Get) || closes != 2 {
		t.Fatalf("Open(false): open %v, binding %v, closes %d; want closed, binding untouched, 2", p.IsOpen(), Untrack(open.Get), closes)
	}
	p.Open(false)
	open.Set(false)
	if closes != 2 {
		t.Fatalf("closing a closed popup, or writing a detached binding, fired OnClose: %d", closes)
	}
	p.Toggle()
	if !p.IsOpen() || Untrack(open.Get) {
		t.Fatalf("Toggle after Open: open %v, old binding %v; want open and the binding left alone", p.IsOpen(), Untrack(open.Get))
	}
}

func TestPopupGapSetsTheDistanceFromTheAnchor(t *testing.T) {
	t.Parallel()
	var content Rect
	p := Popup(Box().Size(60, 20), probe(80, 40, &content)).Gap(10)
	pr := NewProbe(Column(Box().Size(10, 30), p), Sz(300, 300))
	defer pr.Close()
	p.Show()
	pr.Frame()
	if content.Origin.Y != 30+20+10 || p.Rect() != content {
		t.Fatalf("content at %+v, Rect %+v; want it 10 below the anchor's bottom at 50", content, p.Rect())
	}
}

func TestPopupShowAtPlacesContentAtAPointUntilAnchored(t *testing.T) {
	t.Parallel()
	var content Rect
	p := Popup(Box().Size(60, 20), probe(80, 40, &content))
	pr := NewProbe(Column(p), Sz(300, 200))
	defer pr.Close()
	p.ShowAt(Pt(100, 50))
	pr.Frame()
	if content != Rct(Pt(100, 54), Sz(80, 40)) {
		t.Fatalf("ShowAt(100, 50) painted the content at %+v, want the gap below the point at (100,54)", content)
	}
	// A point near the edge keeps the content on screen.
	p.ShowAt(Pt(290, 50))
	pr.Frame()
	if content.Origin.X != 300-80 {
		t.Fatalf("ShowAt near the right edge put the content at x=%v, want 220", content.Origin.X)
	}
	p.AnchorPosition()
	pr.Frame()
	if content.Origin != Pt(0, 24) {
		t.Fatalf("after AnchorPosition the content is at %v, want below the anchor at (0,24)", content.Origin)
	}
}

func TestPopupEscapeClosesAndReturnsFocus(t *testing.T) {
	t.Parallel()
	var openerFocused, insideFocused bool
	opener := Focus(Box().Size(60, 20)).OnFocus(func(v bool) { openerFocused = v })
	inside := Focus(Box().Size(80, 20)).OnFocus(func(v bool) { insideFocused = v })
	p := Popup(opener, Column(inside, Focus(Box().Size(80, 20))))
	pr := NewProbe(Column(p), Sz(300, 300))
	defer pr.Close()
	pr.Click(Pt(30, 10))
	if !openerFocused {
		t.Fatal("a click did not focus the opener")
	}
	p.Show()
	pr.Move(Pt(290, 290)) // the trap is noticed with the next input
	if !insideFocused || openerFocused {
		t.Fatalf("opening the popup: inside focused %v, opener %v; want focus moved into the popup", insideFocused, openerFocused)
	}
	pr.Key("tab", "tab")
	if !insideFocused {
		t.Fatal("Tab left the popup's two controls")
	}
	pr.Key("escape")
	pr.Move(Pt(290, 290))
	if p.IsOpen() || !openerFocused {
		t.Fatalf("after Escape: open %v, opener focused %v; want closed with focus back on the opener", p.IsOpen(), openerFocused)
	}
}

func TestPopupKeysKeepsFocusOnTheOpener(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	for _, keys := range []bool{true, false} {
		field := TextInput(State("x"))
		p := Popup(field, Box().Size(80, 40))
		if keys {
			p.Keys(field)
		}
		pr := NewProbe(Column(p), Sz(200, 200))
		pr.Click(Pt(100, 5))
		p.Show()
		pr.Click(Pt(20, 40)) // inside the content
		if field.Focused() != keys {
			t.Errorf("Keys %v: the field is focused %v after a click in the content, want %v", keys, field.Focused(), keys)
		}
		pr.Close()
	}
}

func TestPopupKeyKeepsItOpenWhenMoved(t *testing.T) {
	t.Parallel()
	moved := State(false)
	var p *PopupWidget
	pr := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			p = Popup(Box().Size(60, 20), Box().Size(80, 40)).Key("menu")
			if moved.Get() {
				return Column(Box().Size(10, 50), p)
			}
			return Column(p)
		})
	}, Sz(300, 300))
	defer pr.Close()
	pr.Frame()
	p.Show()
	pr.Frame()
	moved.Set(true)
	pr.Frame()
	if !p.IsOpen() {
		t.Fatal("a rebuilt popup with the same key at another place closed")
	}
}

func TestPopupFadesOutInertThenStopsPainting(t *testing.T) {
	t.Parallel()
	paints, taps := 0, 0
	content := FromFuncs(
		func(c Constraints, _ Env) Size { return c.Constrain(Sz(80, 40)) },
		func(*Canvas, Rect) { paints++ },
	)
	p := Popup(Box().Size(60, 20), Tap(content, func() { taps++ }))
	pr := NewProbe(p, Sz(300, 300))
	defer pr.Close()
	pr.Advance(0)
	p.Show()
	pr.Advance(time.Second)
	p.Hide()
	paints = 0
	pr.Advance(50 * time.Millisecond) // of the default 150ms
	if paints == 0 {
		t.Fatal("a closing popup stopped painting before its animation ended")
	}
	pr.Click(Pt(20, 40))
	if taps != 0 {
		t.Fatalf("a click on a closing popup's content tapped it %d times; it is inert", taps)
	}
	pr.Advance(time.Second)
	paints = 0
	pr.Frame()
	if paints != 0 {
		t.Fatal("a closed popup still paints after its animation")
	}
}

func TestPopupOwnerNestsTheContentUnderTheOpener(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	for _, owned := range []bool{true, false} {
		field := TextInput(State("")).Name("combo")
		option := FromFuncs(
			func(c Constraints, _ Env) Size { return c.Constrain(Sz(80, 20)) },
			func(dst *Canvas, r Rect) { dst.Leaf(r, Node{Role: RoleImage, Name: "option"}) },
		)
		p := Popup(field, option)
		if owned {
			p.Owner(field)
		}
		pr := NewProbe(Column(p), Sz(200, 200))
		p.Show()
		tree := pr.Semantics()
		under := false
		for i, n := range tree.Nodes(RoleImage) {
			for _, a := range tree.Ancestors(i) {
				under = under || (a.Role == RoleTextField && a.Name == "combo")
			}
			if n.Name != "option" {
				t.Fatalf("unexpected image %q", n.Name)
			}
		}
		if under != owned {
			t.Errorf("Owner set %v: the option is under the field %v", owned, under)
		}
		pr.Close()
	}
}
