package ggui

import "testing"

// shortcutKeeper registers a chord from Paint, under a focus trap if trap
// is set and on an inert canvas while inert is, with a key region so the
// trap has something to hold.
type shortcutKeeper struct {
	chord Chord
	fn    func()
	trap  bool
	inert *StateValue[bool]
}

func (s *shortcutKeeper) Layout(c Constraints, _ Env) Size { return c.Constrain(Sz(20, 20)) }
func (s *shortcutKeeper) HandleKey(KeyEvent)               {}
func (s *shortcutKeeper) Paint(dst *Canvas, r Rect) {
	if s.inert != nil && Untrack(s.inert.Get) {
		dst = dst.Inert()
	}
	paint := func(dst *Canvas) {
		dst.HitKey(r, s)
		dst.Shortcut(s.chord, s.fn)
	}
	if s.trap {
		dst.FocusTrap(s, nil, paint)
		return
	}
	paint(dst)
}

// A painted shortcut runs while its widget is painted and stops when the
// widget goes, when it is inert, or while a trap it is outside is open.
func TestCanvasShortcutFollowsWhatIsPainted(t *testing.T) {
	t.Parallel()
	shown, inert, dialog := State(true), State(false), State(false)
	var saves, dialogSaves int
	p := ProbeBuilder(func() Widget {
		return Column(
			If(shown, func() Widget {
				return &shortcutKeeper{chord: MustChord("cmd+s"), fn: func() { saves++ }, inert: inert}
			}),
			If(dialog, func() Widget {
				return &shortcutKeeper{chord: MustChord("cmd+s"), fn: func() { dialogSaves++ }, trap: true}
			}),
		)
	}, Sz(100, 100))
	defer p.Close()
	press := func(what string, wantSaves, wantDialog int) {
		t.Helper()
		p.Key("cmd+s")
		if saves != wantSaves || dialogSaves != wantDialog {
			t.Fatalf("%s: saves %d, dialog saves %d; want %d, %d", what, saves, dialogSaves, wantSaves, wantDialog)
		}
	}
	p.Frame()
	press("painted", 1, 0)
	dialog.Set(true)
	p.Frame()
	press("behind a dialog", 1, 1)
	dialog.Set(false)
	inert.Set(true)
	p.Frame()
	press("inert", 1, 1)
	inert.Set(false)
	shown.Set(false)
	p.Frame()
	press("gone", 1, 1)
	shown.Set(true)
	p.Frame()
	press("back", 2, 1)
}
