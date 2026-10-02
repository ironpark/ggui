package ggui

import (
	"fmt"
	"slices"
	"testing"
)

// FocusRef focuses the first focusable widget in what it is attached to,
// from posted work, and leaves focus alone when a trap it is outside is open
// or the widget has nothing to focus.
func TestFocusRefFocusesTheAttachedWidget(t *testing.T) {
	t.Parallel()
	var first, second, empty FocusRef
	dialog := State(false)
	p := ProbeBuilder(func() Widget {
		return Column(
			first.Attach(Focus(Box().Size(20, 20))),
			second.Attach(Row(Box().Size(5, 5), Focus(Box().Size(20, 20)), Focus(Box().Size(20, 20)))),
			empty.Attach(Box().Size(20, 20)),
			If(dialog, func() Widget { return &shortcutKeeper{trap: true} }),
		)
	}, Sz(200, 200))
	defer p.Close()
	focusedOnly := func(what string, want *FocusRef) {
		t.Helper()
		p.Frame() // runs the posted Focus and paints the request
		p.Frame() // paints with the focus in place
		for name, ref := range map[string]*FocusRef{"first": &first, "second": &second, "empty": &empty} {
			if ref.Focused() != (ref == want) {
				t.Fatalf("%s: %s.Focused() = %v", what, name, ref.Focused())
			}
		}
	}
	p.Frame()
	focusedOnly("at start", nil)
	p.Post(second.Focus)
	focusedOnly("second", &second)
	p.Post(first.Focus)
	focusedOnly("first", &first)
	p.Post(empty.Focus)
	focusedOnly("nothing to focus", &first)
	dialog.Set(true)
	p.Frame()
	p.Post(second.Focus)
	p.Frame()
	p.Frame()
	if second.Focused() {
		t.Fatal("Focus reached a widget behind an open dialog")
	}
}

// A FocusRef whose widget was scrolled into view by Focus reports the focus
// once the scroll has painted, without waiting for input to refresh it.
func TestFocusRefFocusedAfterScrollingIntoView(t *testing.T) {
	t.Parallel()
	refs := make([]FocusRef, 30)
	rows := make([]Widget, len(refs))
	for i := range rows {
		rows[i] = refs[i].Attach(Focus(Box().Size(100, 20)))
	}
	p := NewProbe(Scroll(Column(rows...)), Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Post(refs[25].Focus)
	p.Frame() // focuses the row, out of view, and scrolls to it
	p.Frame() // paints it in view
	p.Frame() // paints with the focus where it now is
	if !refs[25].Focused() {
		t.Fatal("Focused() = false after the row scrolled into view")
	}
}

// A widget rebuilt where the focused one was is still told it has focus.
func TestRebuiltFocusedWidgetIsToldItHasFocus(t *testing.T) {
	t.Parallel()
	var got []string
	gen := State(0)
	var ref FocusRef
	p := NewProbe(ref.Attach(View(gen, func(n int) Widget { return &keyLog{n: n, got: &got} })), Sz(100, 100))
	defer p.Close()
	p.Frame()
	p.Post(ref.Focus)
	p.Frame()
	gen.Set(1)
	p.Type(Mods{}, KeyA)
	want := []string{"0 focus", "1 focus", "1 press"}
	if !slices.Equal(got, want) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

type keyLog struct {
	n   int
	got *[]string
}

func (k *keyLog) Layout(Constraints, Env) Size { return Sz(20, 20) }
func (k *keyLog) Paint(dst *Canvas, r Rect)    { dst.HitKey(r, k) }
func (k *keyLog) HandleKey(e KeyEvent) {
	kind := map[KeyKind]string{KeyFocus: "focus", KeyPress: "press", KeyBlur: "blur"}[e.Kind]
	*k.got = append(*k.got, fmt.Sprint(k.n, " ", kind))
}
