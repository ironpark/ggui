package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func numberProbe(n *ui.NumberInputWidget) *ggui.Probe {
	return ggui.NewProbe(ggui.Column(n, ui.Button("Elsewhere", nil)), ggui.Sz(300, 120))
}

func TestNumberInputCommitsParsedClampedRoundedText(t *testing.T) {
	t.Parallel()
	value := ggui.State(5.0)
	var changes []float64
	n := ui.NumberInput(value).Name("Qty").Range(0, 100).Step(0.5).OnChange(func(v float64) { changes = append(changes, v) })
	p := numberProbe(n)
	defer p.Close()
	p.Frame()
	if got := n.Input().EditingState().Text; got != "5.0" {
		t.Fatalf("shown %q, want 5.0 (one decimal, from the step)", got)
	}
	p.Tap("Qty")
	p.Key("cmd+a")
	paste(p, "12.36")
	if got := ggui.Untrack(value.Get); got != 5 {
		t.Fatalf("typing wrote %v before a commit", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if got := ggui.Untrack(value.Get); got != 12.4 {
		t.Fatalf("Enter committed %v, want 12.4", got)
	}
	p.Frame()
	if got := n.Input().EditingState().Text; got != "12.4" {
		t.Fatalf("shown %q after commit, want 12.4", got)
	}
	p.Key("cmd+a")
	paste(p, "500")
	p.Tap("Elsewhere") // blur commits
	if got := ggui.Untrack(value.Get); got != 100 {
		t.Fatalf("blur committed %v, want it clamped to 100", got)
	}
	if len(changes) != 2 || changes[1] != 100 {
		t.Fatalf("OnChange saw %v", changes)
	}
}

func TestNumberInputInvalidTextReverts(t *testing.T) {
	t.Parallel()
	value := ggui.State(3.0)
	n := ui.NumberInput(value).Name("Qty")
	p := numberProbe(n)
	defer p.Close()
	p.Tap("Qty")
	p.Key("cmd+a")
	paste(p, "abc-") // the filter leaves "-", which is not a number
	if got := n.Input().EditingState().Text; got != "-" {
		t.Fatalf("filter left %q", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	p.Frame()
	if got := n.Input().EditingState().Text; got != "3" || ggui.Untrack(value.Get) != 3 {
		t.Fatalf("invalid text left %q / %v, want it reverted to 3", got, ggui.Untrack(value.Get))
	}
}

func TestNumberInputArrowsAndSteppers(t *testing.T) {
	t.Parallel()
	value := ggui.State(0.0)
	n := ui.NumberInput(value).Name("Qty").Range(-5, 12)
	p := numberProbe(n)
	defer p.Close()
	p.Tap("Qty")
	p.Type(ggui.Mods{}, ggui.KeyArrowUp)
	if got := ggui.Untrack(value.Get); got != 1 {
		t.Fatalf("ArrowUp: %v, want 1", got)
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyArrowUp)
	if got := ggui.Untrack(value.Get); got != 11 {
		t.Fatalf("Shift+ArrowUp: %v, want 11", got)
	}
	p.Tap("Increment")
	if got := ggui.Untrack(value.Get); got != 12 {
		t.Fatalf("+ button: %v, want 12", got)
	}
	if _, ok := p.Find("Increment"); ok {
		t.Fatal("the + button still takes input at the top of the range")
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyArrowDown)
	p.Type(ggui.Mods{Shift: true}, ggui.KeyArrowDown)
	if got := ggui.Untrack(value.Get); got != -5 {
		t.Fatalf("Shift+ArrowDown twice: %v, want -5", got)
	}
	if _, ok := p.Find("Decrement"); ok {
		t.Fatal("the − button still takes input at the bottom of the range")
	}
	p.Tap("Increment")
	if got := ggui.Untrack(value.Get); got != -4 {
		t.Fatalf("+ button: %v, want -4", got)
	}
}

func TestNumberInputSemanticsAndActions(t *testing.T) {
	t.Parallel()
	value := ggui.State(2.0)
	p := numberProbe(ui.NumberInput(value).Name("Qty").Range(0, 10).Step(0.25))
	defer p.Close()
	n := node(t, p.Semantics(), ggui.RoleSpinButton, "Qty")
	if n.Min != 0 || n.Max != 10 || n.Now != 2 || n.Value != "2.00" {
		t.Fatalf("node = min %v max %v now %v value %q", n.Min, n.Max, n.Now, n.Value)
	}
	if !n.Actions.Has(ggui.ActionIncrement | ggui.ActionDecrement | ggui.ActionSetValue | ggui.ActionFocus) {
		t.Fatalf("actions = %b", n.Actions)
	}
	if _, ok := p.Semantics().Find(ggui.RoleTextField, ""); ok {
		t.Fatalf("the editor shows as a text field of its own:\n%s", p.Semantics())
	}
	act(t, p, ggui.RoleSpinButton, "Qty", ggui.Action{Kind: ggui.ActionIncrement})
	if got := ggui.Untrack(value.Get); got != 2.25 {
		t.Fatalf("increment: %v", got)
	}
	act(t, p, ggui.RoleSpinButton, "Qty", ggui.Action{Kind: ggui.ActionSetValue, Num: 42})
	if got := ggui.Untrack(value.Get); got != 10 {
		t.Fatalf("set value 42: %v, want clamped 10", got)
	}
	act(t, p, ggui.RoleSpinButton, "Qty", ggui.Action{Kind: ggui.ActionSetValue, Text: "3.1"})
	if got := ggui.Untrack(value.Get); got != 3.1 {
		t.Fatalf("set value text: %v", got)
	}
	if n := node(t, p.Semantics(), ggui.RoleSpinButton, "Qty"); n.Value != "3.10" {
		t.Fatalf("value text %q", n.Value)
	}
	// The spin button is the editor, so a screen reader moves its caret
	// and reads where it is as in any field.
	if !n.Actions.Has(ggui.ActionSetSelection) {
		t.Fatalf("the spin button cannot move its caret: actions %b", n.Actions)
	}
	act(t, p, ggui.RoleSpinButton, "Qty", ggui.Action{Kind: ggui.ActionSetSelection, SelStart: 1, SelEnd: 3})
	if n := node(t, p.Semantics(), ggui.RoleSpinButton, "Qty"); n.SelStart != 1 || n.SelEnd != 3 {
		t.Fatalf("selection %d-%d after setting it to 1-3", n.SelStart, n.SelEnd)
	}
}

func TestNumberInputDisabled(t *testing.T) {
	t.Parallel()
	value := ggui.State(1.0)
	busy := ggui.State(true)
	p := numberProbe(ui.NumberInput(value).Name("Qty").BindDisabled(busy))
	defer p.Close()
	if !node(t, p.Semantics(), ggui.RoleSpinButton, "Qty").Disabled {
		t.Fatal("not marked disabled")
	}
	if _, ok := p.Find("Increment"); ok {
		t.Fatal("a disabled field's stepper takes input")
	}
	act(t, p, ggui.RoleSpinButton, "Qty", ggui.Action{Kind: ggui.ActionIncrement})
	if ggui.Untrack(value.Get) != 1 {
		t.Fatal("a disabled field stepped")
	}
	busy.Set(false)
	p.Tap("Increment")
	if ggui.Untrack(value.Get) != 2 {
		t.Fatal("re-enabled field did not step")
	}
}

func TestNumberInputFollowsExternalWrites(t *testing.T) {
	t.Parallel()
	value := ggui.State(1.0)
	n := ui.NumberInput(value).Precision(2)
	p := numberProbe(n)
	defer p.Close()
	p.Frame()
	value.Set(7.5)
	p.Frame()
	if got := n.Input().EditingState().Text; got != "7.50" {
		t.Fatalf("shown %q, want 7.50", got)
	}
}
