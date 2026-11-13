package ui_test

import (
	"math"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/ui/theme"
)

type selectTriggerFace struct {
	natural ggui.Size
	painted ggui.Rect
	paints  int
}

func (f *selectTriggerFace) Layout(c ggui.Constraints, _ ggui.Env) ggui.Size {
	return c.Constrain(f.natural)
}
func (f *selectTriggerFace) Paint(_ *ggui.Canvas, r ggui.Rect) { f.painted = r; f.paints++ }

func TestSelectTriggerContentMeasuresFaceWithoutChevronSpace(t *testing.T) {
	t.Parallel()
	th := theme.Default()
	face := &selectTriggerFace{natural: ggui.Sz(84, 52)}
	control := ui.Select(ggui.State("a")).Options([]string{"a", "An option much wider than the face"}).TriggerContent(face).Name("Custom")
	env := th.Apply(ggui.Env{})
	loose := ggui.Loose(ggui.Sz(500, 300))
	if got, want := control.Layout(loose, env), th.FieldPad.Inflate(face.natural); got != want {
		t.Fatalf("custom field=%v, want only face + padding %v", got, want)
	}
	if got := control.MinWidth(180).Layout(loose, env); got.W != 180 || got.H != 52+th.FieldPad.Vertical() {
		t.Fatalf("MinWidth changed natural height or reserved chevron room: %v", got)
	}
	if got := control.Layout(ggui.Tight(ggui.Sz(120, 30)), env); got != ggui.Sz(120, 30) {
		t.Fatalf("tight constraints ignored: %v", got)
	}
	p := ggui.NewProbe(theme.With(th, ggui.Column(control)), ggui.Sz(500, 300))
	defer p.Close()
	field := find(t, p, ggui.RoleSelect, "Custom").Rect
	if face.painted.Origin.X != field.Origin.X+th.FieldPad.Left || face.painted.Origin.Y != field.Origin.Y+th.FieldPad.Top ||
		face.painted.Size.W != field.Size.W-th.FieldPad.Horizontal() || face.painted.Size.H != 52 {
		t.Fatalf("face not painted inside full field padding: face=%v field=%v", face.painted, field)
	}
}

func TestSelectTriggerContentPreservesSemanticsAndInput(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		value := ggui.State("alpha")
		changes, childActions := 0, 0
		face := ui.Button("Decorative button", func() { childActions++ })
		control := ui.Select(value).Native(native).Options([]string{"alpha", "beta", "gamma"}).Format(strings.ToUpper).
			TriggerContent(face).Name("Choice").AlignItemWithTrigger(false).OnChange(func(string) { changes++ })
		p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(control, ui.Button("After", nil))), ggui.Sz(350, 300))
		n := node(t, p.Semantics(), ggui.RoleSelect, "Choice")
		if n.Value != "ALPHA" {
			t.Fatalf("custom text replaced formatted semantic value: %q", n.Value)
		}
		if _, ok := p.Semantics().Find(ggui.RoleButton, "Decorative button"); ok {
			t.Fatal("decorative face exposed a second interactive semantic node")
		}
		p.Key("tab", "down")
		if value.Get() != "beta" {
			t.Fatal("closed keyboard selection failed")
		}
		p.Key("space", "down", "enter")
		if value.Get() != "gamma" || control.Popup().IsOpen() {
			t.Fatal("popup keyboard selection failed")
		}
		p.Tap("Choice")
		p.Tap("ALPHA")
		if value.Get() != "alpha" || changes != 3 || childActions != 0 {
			t.Fatalf("native=%v selection=%q changes=%d child presses=%d", native, value.Get(), changes, childActions)
		}
		field := find(t, p, ggui.RoleSelect, "Choice").Rect
		control.Disabled(true)
		p.Click(field.Center())
		if control.Popup().IsOpen() || childActions != 0 || !node(t, p.Semantics(), ggui.RoleSelect, "Choice").Disabled {
			t.Fatal("disabled custom trigger accepted input")
		}
		p.Close()
	}
}

func TestSelectTriggerContentUpdatesCachedLayoutAndRestoresDefault(t *testing.T) {
	t.Parallel()
	value, height := ggui.State("Alpha"), ggui.State(26.0)
	face := ggui.Box().Width(60).BindHeight(height)
	control := ui.Select(value).Options([]string{"Alpha", "Beta"}).TriggerContent(face).Name("Dynamic")
	p := ggui.NewProbe(ggui.Column(ggui.Cached(control)), ggui.Sz(500, 300))
	defer p.Close()
	before := find(t, p, ggui.RoleSelect, "Dynamic").Rect
	height.Set(64)
	after := find(t, p, ggui.RoleSelect, "Dynamic").Rect
	if after.Size.H-before.Size.H != 38 {
		t.Fatalf("bound face height did not invalidate cached Select: before=%v after=%v", before, after)
	}
	replacement := &selectTriggerFace{natural: ggui.Sz(140, 44)}
	control.TriggerContent(replacement)
	replaced := find(t, p, ggui.RoleSelect, "Dynamic").Rect
	if replaced.Size.W <= after.Size.W || replaced.Size.H >= after.Size.H {
		t.Fatalf("replacing face did not invalidate cache: before=%v after=%v", after, replaced)
	}
	paints := replacement.paints
	control.TriggerContent(nil)
	restored := find(t, p, ggui.RoleSelect, "Dynamic").Rect
	plain := ui.Select(value).Options([]string{"Alpha", "Beta"}).Layout(ggui.Loose(ggui.Sz(500, 300)), theme.Default().Apply(ggui.Env{}))
	if restored.Size != plain {
		t.Fatalf("nil did not restore original field geometry: got=%v want=%v", restored.Size, plain)
	}
	if replacement.paints != paints {
		t.Fatal("nil still painted the old custom face")
	}
	if got := node(t, p.Semantics(), ggui.RoleSelect, "Dynamic").Value; got != "Alpha" {
		t.Fatalf("restored value=%q", got)
	}
}

func TestSelectTriggerContentAlignsUsingTallAnchor(t *testing.T) {
	t.Parallel()
	control := ui.Select(ggui.State("Beta")).Options([]string{"Alpha", "Beta", "Gamma"}).
		TriggerContent(ggui.Box().Size(140, 64)).Name("Tall choice")
	p := ggui.NewProbe(ggui.Provide(ggui.ReducedMotionKey, true, ggui.Column(ggui.Box().Height(100), control)), ggui.Sz(400, 350))
	defer p.Close()
	p.Tap("Tall choice")
	field := find(t, p, ggui.RoleSelect, "Tall choice").Rect
	selected := find(t, p, ggui.RoleOption, "Beta").Rect
	if math.Abs(field.Center().Y-selected.Center().Y) > 1e-6 {
		t.Fatalf("selected item did not align with custom anchor height: field=%v item=%v", field, selected)
	}
}
