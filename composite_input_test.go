package ggui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

type focusRequester struct {
	child    ggui.Widget
	target   ggui.KeyHandler
	request  bool
	observed *int
}

func (w *focusRequester) Layout(c ggui.Constraints, e ggui.Env) ggui.Size {
	return w.child.Layout(c, e)
}
func (w *focusRequester) Paint(dst *ggui.Canvas, r ggui.Rect) {
	if w.observed != nil {
		dst.ObserveInput(r, func() { *w.observed++ })
	}
	dst.Paint(w.child, r)
	if w.request {
		dst.RequestFocus(w.target)
		w.request = false
	}
}
func TestCompositeFocusRequestAndInputObserver(t *testing.T) {
	actions, observed := 0, 0
	b := ui.Button("Child", func() { actions++ })
	w := &focusRequester{child: b, target: b, request: true, observed: &observed}
	p := ggui.NewProbe(w, ggui.Sz(200, 100))
	defer p.Close()
	p.Frame()
	if n, ok := p.Semantics().Focused(); !ok || n.Name != "Child" {
		t.Fatal("focus request not applied", n)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if observed != 1 || actions != 1 {
		t.Fatal("observer swallowed child keyboard input", observed, actions)
	}
	p.Tap("Child")
	if observed != 2 || actions != 2 {
		t.Fatal("observer swallowed child pointer input", observed, actions)
	}
	// A request cannot focus a disabled or unpainted control.
	b.Disabled(true)
	w.request = true
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if actions != 2 {
		t.Fatal("disabled requested target activated")
	}
}
func TestInputObserversRespectModalScope(t *testing.T) {
	observed := 0
	open := ggui.State(true)
	background := &focusRequester{child: ui.Button("Background", func() {}), observed: &observed}
	p := ggui.NewProbe(ggui.Column(background, ui.Dialog(open, ui.Button("Modal", func() {}))), ggui.Sz(400, 300))
	defer p.Close()
	p.Tap("Modal")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if observed != 0 {
		t.Fatal("modal input reached background observer")
	}
}
