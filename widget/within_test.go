package widget

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
)

// Within hears the pointer over its child and what is inside it, focus
// coming into it and leaving, and a press anywhere in it before the
// widget it lands on, and takes none of it.
func TestWithinCallsItsCallbacks(t *testing.T) {
	t.Parallel()
	var hover, focus []bool
	var downs, rowHover []int
	tapped := 0
	inner := Pointer(Tap(Box().Size(40, 40), func() { tapped++ })).OnHover(func(on bool) {
		if on {
			rowHover = append(rowHover, 1)
		} else {
			rowHover = append(rowHover, 0)
		}
	})
	w := Column(
		Within(Row(inner, Focus(Box().Size(40, 40)), Box().Size(40, 40))).
			OnHover(func(on bool) { hover = append(hover, on) }).
			OnFocus(func(on bool) { focus = append(focus, on) }).
			OnDown(func(ggui.PointerEvent) { downs = append(downs, tapped) }),
		Focus(Box().Size(40, 40)),
	)
	p := ggui.NewProbe(w, ggui.Sz(200, 100))
	defer p.Close()
	p.Move(ggui.Pt(10, 10)) // the tap target inside
	p.Move(ggui.Pt(100, 10))
	p.Move(ggui.Pt(10, 10))
	if !slices.Equal(hover, []bool{true}) || !slices.Equal(rowHover, []int{1, 0, 1}) {
		t.Fatalf("hover %v, the inner's %v; want [true] and [1 0 1]", hover, rowHover)
	}
	p.Click(ggui.Pt(10, 10))
	if !slices.Equal(downs, []int{0}) || tapped != 1 {
		t.Fatalf("downs heard with %v taps before, %d taps; want the press heard before the tap", downs, tapped)
	}
	p.Click(ggui.Pt(100, 10)) // the bare box inside
	p.Click(ggui.Pt(60, 10))  // the focusable inside
	if len(downs) != 3 || !slices.Equal(focus, []bool{true}) {
		t.Fatalf("%d downs, focus %v; want 3 and [true]", len(downs), focus)
	}
	p.Click(ggui.Pt(10, 60)) // the focusable beside
	if len(downs) != 3 || !slices.Equal(focus, []bool{true, false}) {
		t.Fatalf("%d downs, focus %v after a click beside; want 3 and [true false]", len(downs), focus)
	}
	p.Move(ggui.Pt(10, 60))
	if !slices.Equal(hover, []bool{true, false}) {
		t.Fatalf("hover %v after leaving, want [true false]", hover)
	}
}

// Typing in a field inside a Within reaches the field alone: the letters
// are consumed by it. Escape, which it leaves, and a chord with ⌘ it does
// not claim, are offered.
func TestTypingInAFieldDoesNotReachKeysWithin(t *testing.T) {
	t.Parallel()
	text := ggui.State("")
	var heard []string
	w := Within(TextInput(text)).OnKey(func(ev ggui.KeyEvent) bool {
		heard = append(heard, ev.Key.String())
		return ev.Key == ggui.KeyK
	})
	p := ggui.NewProbe(Column(w), ggui.Sz(200, 60))
	defer p.Close()
	p.Click(ggui.Pt(10, 10))
	p.TypeText(ggui.Mods{}, ggui.KeyH, "h")
	p.TypeText(ggui.Mods{}, ggui.KeyI, "i")
	if text.Get() != "hi" || len(heard) != 0 {
		t.Fatalf("typed %q, offered %v; want the letters in the field alone", text.Get(), heard)
	}
	p.Key("escape")
	p.Key("cmd+k")
	if !slices.Equal(heard, []string{"Escape", "K"}) {
		t.Fatalf("offered %v, want Escape and ⌘K", heard)
	}
	p.Key("cmd+a") // the field's own select-all, which it claims
	if len(heard) != 2 {
		t.Fatalf("offered %v: a chord the field claims was offered", heard)
	}
}
