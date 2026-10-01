package ui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestBubbleVariantsPickTheirThemeColors(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	for _, tc := range []struct {
		name             string
		set              func(*BubbleWidget) *BubbleWidget
		fill, fg, border color.Color
	}{
		{"default", func(b *BubbleWidget) *BubbleWidget { return b }, th.Primary, th.PrimaryFg, nil},
		{"secondary", (*BubbleWidget).Secondary, th.Secondary, th.SecondaryFg, nil},
		{"muted", (*BubbleWidget).Muted, th.Muted, th.Fg, nil},
		{"tinted", (*BubbleWidget).Tinted, mix(th.Card, th.Primary, .12), th.Fg, nil},
		{"outline", (*BubbleWidget).Outline, th.Bg, th.Fg, th.Border},
		{"ghost", (*BubbleWidget).Ghost, nil, th.Fg, nil},
		{"destructive", (*BubbleWidget).Destructive, mix(th.Card, th.Destructive, .12), th.Destructive, nil},
	} {
		b := Bubble(ggui.Text("hi"))
		if tc.set(b) != b {
			t.Fatalf("%s: the variant setter returned another bubble", tc.name)
		}
		b.Layout(ggui.Loose(ggui.Sz(300, 100)), th.Apply(ggui.Env{}))
		fill, fg, border := b.colors()
		if !sameColor(fill, tc.fill) || !sameColor(fg, tc.fg) || !sameColor(border, tc.border) {
			t.Errorf("%s: colors %v %v %v, want %v %v %v", tc.name, fill, fg, border, tc.fill, tc.fg, tc.border)
		}
	}
}

// sameColor compares colors by value, treating two nils as equal.
func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

func TestBubbleLinkPressesThroughSemanticsAndKeyboard(t *testing.T) {
	t.Parallel()
	opened := 0
	p := ggui.NewProbe(Bubble(ggui.Text("docs")).Link("Open docs", func() { opened++ }), ggui.Sz(300, 80))
	defer p.Close()
	node, ok := p.Semantics().Find(ggui.RoleLink, "Open docs")
	if !ok {
		t.Fatalf("Link did not describe a link:\n%s", p.Semantics())
	}
	p.Perform(node.ID, ggui.Action{Kind: ggui.ActionPress})
	p.Tap("Open docs")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if opened != 3 {
		t.Fatalf("press, tap and Enter opened the link %d times, want 3", opened)
	}
}

func TestBubbleDisabledFollowsItsBindingAndIgnoresInput(t *testing.T) {
	t.Parallel()
	pressed := 0
	disabled := ggui.State(true)
	b := Bubble(ggui.Text("hi")).Action("Reply", func() { pressed++ }).BindDisabled(disabled)
	p := ggui.NewProbe(b, ggui.Sz(300, 80))
	defer p.Close()
	node, _ := p.Semantics().Find(ggui.RoleButton, "Reply")
	if !node.Disabled {
		t.Fatal("bound-disabled bubble is not described as disabled")
	}
	p.Perform(node.ID, ggui.Action{Kind: ggui.ActionPress})
	p.Click(node.Rect.Center())
	if pressed != 0 || b.Act(ggui.Action{Kind: ggui.ActionPress}) {
		t.Fatalf("disabled bubble ran its action %d times", pressed)
	}
	disabled.Set(false)
	p.Tap("Reply")
	if pressed != 1 {
		t.Fatalf("re-enabled bubble ran its action %d times, want 1", pressed)
	}
	b.Disabled(true)
	p.Click(node.Rect.Center())
	if pressed != 1 {
		t.Fatal("Disabled(true) after a binding did not disable the bubble")
	}
	if b.Act(ggui.Action{Kind: ggui.ActionFocus}) {
		t.Fatal("a bubble handled an action other than press")
	}
}

func TestBubbleReactionsStartSitsAtTheLeadingEdge(t *testing.T) {
	t.Parallel()
	body := func(start bool) ggui.Rect {
		b := Bubble(ggui.Text("a long enough message")).Reactions(Button("Like", nil).Ghost())
		if start {
			b.ReactionsStart()
		}
		p := ggui.NewProbe(b, ggui.Sz(400, 120))
		defer p.Close()
		f, ok := p.FindRole(ggui.RoleButton, "Like")
		if !ok {
			t.Fatal("the reaction button is missing")
		}
		return f.Rect
	}
	end, start := body(false), body(true)
	if start.Origin.X >= end.Origin.X {
		t.Fatalf("ReactionsStart put the reaction at x=%v, not before the default x=%v", start.Origin.X, end.Origin.X)
	}
}

func TestBubbleGroupStacksBubblesEightApart(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(BubbleGroup(
		Bubble(ggui.Text("one")).Action("First", func() {}),
		Bubble(ggui.Text("two")).Action("Second", func() {}),
	), ggui.Sz(300, 200))
	defer p.Close()
	first, _ := p.FindRole(ggui.RoleButton, "First")
	second, _ := p.FindRole(ggui.RoleButton, "Second")
	if gap := second.Rect.Origin.Y - (first.Rect.Origin.Y + first.Rect.Size.H); gap != 8 {
		t.Fatalf("bubbles are %v apart, want 8: %v %v", gap, first.Rect, second.Rect)
	}
}
