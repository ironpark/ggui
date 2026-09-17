package ggui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestParseChord(t *testing.T) {
	cases := map[string]Chord{
		"cmd+s":        {Key: ebiten.KeyS, Cmd: true},
		"Ctrl+Shift+Z": {Key: ebiten.KeyZ, Mods: Mods{Ctrl: true, Shift: true}},
		"escape":       {Key: ebiten.KeyEscape},
		"alt+enter":    {Key: ebiten.KeyEnter, Mods: Mods{Alt: true}},
		"up":           {Key: ebiten.KeyArrowUp},
		"f1":           {Key: ebiten.KeyF1},
	}
	for in, want := range cases {
		got, err := ParseChord(in)
		if err != nil || got != want {
			t.Errorf("ParseChord(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ctrl+", "bogus", "s+ctrl"} {
		if _, err := ParseChord(bad); err == nil {
			t.Errorf("ParseChord(%q) accepted", bad)
		}
	}
	if s := MustChord("ctrl+shift+z").String(); s != "ctrl+shift+z" {
		t.Fatalf("String = %q", s)
	}
}

func TestKeyEventIs(t *testing.T) {
	ev := KeyEvent{Kind: KeyPress, Key: ebiten.KeyS, Mods: Mods{Ctrl: true}}
	if !ev.Is(MustChord("ctrl+s")) || ev.Is(MustChord("ctrl+shift+s")) || ev.Is(MustChord("s")) {
		t.Fatal("ctrl+s matched wrongly")
	}
	if (KeyEvent{Kind: KeyFocus, Key: ebiten.KeyS, Mods: Mods{Ctrl: true}}).Is(MustChord("ctrl+s")) {
		t.Fatal("a focus event is not a press")
	}
}

// spaceEater is a focusable region that consumes Space.
type spaceEater struct {
	FocusWidget
	seen []ebiten.Key
}

func (e *spaceEater) HandleKey(ev KeyEvent) {
	if ev.Kind == KeyPress {
		e.seen = append(e.seen, ev.Key)
	}
}
func (e *spaceEater) ConsumesKey(ev KeyEvent) bool { return ev.Key == ebiten.KeySpace }
func (e *spaceEater) Paint(dst *Canvas, r Rect)    { dst.HitKey(r, e) }

func TestShortcutsRunAroundTheFocusedWidget(t *testing.T) {
	e := &spaceEater{FocusWidget: *Focus(Box().Size(20, 20))}
	p := NewProbe(e, Sz(50, 50))
	defer p.Close()
	runs := map[string]int{}
	p.Shortcut("space", func() { runs["space"]++ })
	p.Shortcut("s", func() { runs["s"]++ })
	p.Shortcut("ctrl+s", func() { runs["ctrl+s"]++ })
	p.Type(Mods{}, ebiten.KeySpace) // nothing focused: the bare key runs
	if runs["space"] != 1 {
		t.Fatalf("space ran %d times with nothing focused, want 1", runs["space"])
	}
	p.Click(Pt(5, 5))
	p.Type(Mods{}, ebiten.KeySpace, ebiten.KeyS)
	if runs["space"] != 1 || runs["s"] != 1 || len(e.seen) != 2 {
		t.Fatalf("space %d s %d, widget saw %v; want the widget to keep Space and s to run after it", runs["space"], runs["s"], e.seen)
	}
	p.Type(Mods{Ctrl: true}, ebiten.KeyS)
	if runs["ctrl+s"] != 1 || len(e.seen) != 2 {
		t.Fatalf("ctrl+s ran %d, widget saw %v; a modified chord goes before the widget", runs["ctrl+s"], e.seen)
	}
	p.Shortcut("space", func() { runs["excl"]++ }).Exclusive()
	p.Type(Mods{}, ebiten.KeySpace)
	if runs["excl"] != 1 || len(e.seen) != 2 {
		t.Fatalf("exclusive ran %d, widget saw %v; want it to take the key", runs["excl"], e.seen)
	}
}

func TestPopupTrapsTabAndRestoresFocus(t *testing.T) {
	focused := ""
	track := func(name string, size float64) *FocusWidget {
		return Focus(Box().Size(size, size)).OnFocus(func(on bool) {
			if on {
				focused = name
			} else if focused == name {
				focused = ""
			}
		})
	}
	a, b := track("a", 20), track("b", 20)
	c1, c2 := track("c1", 20), track("c2", 20)
	pop := Popup(a, Column(c1, c2))
	p := NewProbe(Column(pop, b), Sz(100, 200))
	defer p.Close()
	p.Click(Pt(10, 10))
	if focused != "a" {
		t.Fatalf("focused %q after clicking a", focused)
	}
	pop.Show()
	p.Frame()
	p.Type(Mods{}, ebiten.KeyTab)
	if focused != "c2" {
		t.Fatalf("focused %q; the scope should take focus to c1 and Tab move it to c2", focused)
	}
	p.Type(Mods{}, ebiten.KeyTab)
	if focused != "c1" {
		t.Fatalf("focused %q; Tab must wrap inside the popup, not reach b", focused)
	}
	p.Type(Mods{}, ebiten.KeyEscape)
	if pop.IsOpen() {
		t.Fatal("Escape did not close the popup")
	}
	p.Move(Pt(90, 190)) // the next frame's dispatch restores focus
	if focused != "a" {
		t.Fatalf("focused %q after closing, want a", focused)
	}
}

func TestTabScrollsTheTargetIntoView(t *testing.T) {
	items := Column(Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)), Focus(Box().Size(50, 50)))
	sc := Scroll(items)
	p := NewProbe(sc, Sz(50, 70))
	defer p.Close()
	p.Type(Mods{}, ebiten.KeyTab, ebiten.KeyTab)
	if sc.offset != 30 {
		t.Fatalf("offset = %v after tabbing to the second item, want 30 so it is fully shown", sc.offset)
	}
}
