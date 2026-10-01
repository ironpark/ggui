package ggui

import "testing"

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
