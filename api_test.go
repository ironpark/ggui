package ggui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// This reader tracks through Get, but deliberately has no GetAny method.
type formattedReader struct{ source ggui.Readable[int] }

func (r formattedReader) Get() int     { return r.source.Get() }
func (formattedReader) String() string { return "custom reader" }

func TestSprintfCustomReadablePassesThroughWithoutGetAny(t *testing.T) {
	t.Parallel()
	state := ggui.State(1)
	r := formattedReader{state}
	text := ggui.Sprintf("%v", r)
	defer text.Dispose()
	if got := text.Get(); got != "custom reader" {
		t.Fatalf("custom argument formatted as %q", got)
	}
	state.Set(2)
	if got := text.Get(); got != "custom reader" {
		t.Fatalf("bare reader was implicitly unwrapped: %q", got)
	}
}

func TestTextfAdaptsCustomReadableAndTracksLens(t *testing.T) {
	t.Parallel()
	state := ggui.State(1)
	form := ggui.State(struct{ Name string }{"Ada"})
	reader := formattedReader{state}
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Textf("%d %s", ggui.Derived(reader.Get), form.Field(func(f *struct{ Name string }) *string { return &f.Name }))
	}, ggui.Sz(300, 50))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleText, "1 Ada"); !ok {
		t.Fatal("adapted reader or lens was not formatted")
	}
	state.Set(2)
	form.Set(struct{ Name string }{"Grace"})
	if _, ok := p.Semantics().Find(ggui.RoleText, "2 Grace"); !ok {
		t.Fatal("formatted value did not follow both sources")
	}
}

func TestKeyedIdentityAcrossParentsAndRemount(t *testing.T) {
	t.Parallel()
	left, right := ggui.State(false), ggui.State(false)
	rebuild, parent := ggui.State(0), ggui.State(0)
	show := ggui.State(true)
	var controls [2]*ui.CheckboxWidget
	panel := func(index int, value *ggui.StateValue[bool], label string) ggui.Widget {
		return ggui.Component(func() ggui.Widget {
			visible := ggui.Derived(func() bool { parent.Get(); return index != 0 || show.Get() })
			return ggui.If(visible, func() ggui.Widget {
				return ggui.Reactive(func() ggui.Widget {
					rebuild.Get()
					controls[index] = ui.Checkbox(value, label)
					return controls[index]
				})
			})
		})
	}

	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Column(panel(0, left, "Left"), panel(1, right, "Right"))
	}, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	first := controls[0].HitID()
	if first == nil || first == controls[1].HitID() {
		t.Fatal("separate mounts must have distinct automatic IDs")
	}
	p.Tap("Left")
	p.Frame()
	if !controls[0].Focused || controls[1].Focused {
		t.Fatal("static paint transferred focus between mounts")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if ggui.Untrack(left.Get) || ggui.Untrack(right.Get) {
		t.Fatal("Space must toggle only the focused left checkbox")
	}
	parent.Set(1)
	rebuild.Set(1)
	p.Frame()
	p.Frame()
	if controls[0].HitID() != first {
		t.Fatal("rebuilding a mounted component changed its ID")
	}
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if !ggui.Untrack(left.Get) || ggui.Untrack(right.Get) {
		t.Fatal("Space after rebuild must toggle only the left checkbox")
	}
	show.Set(false)
	p.Frame()
	show.Set(true)
	p.Frame()
	if controls[0].HitID() == first {
		t.Fatal("a new mount reused the disposed mount's identity")
	}
}

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
	t.Parallel()
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
	t.Parallel()
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
