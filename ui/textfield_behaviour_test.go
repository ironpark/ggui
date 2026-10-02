package ui_test

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestTextFieldCallbacksFollowEditsSubmitAndBlur(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	var changes, submits, commits []string
	field := ui.TextField(value).Name("Code").
		Filter(func(s string) string { return strings.Map(dropDigits, s) }).
		OnChange(func(s string) { changes = append(changes, s) }).
		OnSubmit(func(s string) { submits = append(submits, s) }).
		OnCommit(func(s string) { commits = append(commits, s) })
	p := ggui.NewProbe(ggui.Column(field, ui.Button("Elsewhere", nil)), ggui.Sz(300, 120))
	defer p.Close()
	p.Tap("Code")
	paste(p, "a1b2")
	if got := ggui.Untrack(value.Get); got != "ab" {
		t.Fatalf("Filter left %q, want %q", got, "ab")
	}
	if len(changes) == 0 || changes[len(changes)-1] != "ab" {
		t.Fatalf("OnChange saw %q, want it to end with the filtered value", changes)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if len(submits) != 1 || submits[0] != "ab" {
		t.Fatalf("Enter submitted %q, want [ab]", submits)
	}
	paste(p, "c")
	p.Tap("Elsewhere")
	if len(commits) < 2 || commits[len(commits)-1] != "abc" {
		t.Fatalf("OnCommit saw %q, want a commit on submit and on blur ending with abc", commits)
	}
}

// paste enters s into the focused editor by pasting it: headless probes have
// no IME, which is how editors receive typed text.
func paste(p *ggui.Probe, s string) {
	p.Clipboard().Write(s)
	p.Key("cmd+v")
}

// dropDigits is a strings.Map function that removes ASCII digits.
func dropDigits(r rune) rune {
	if r >= '0' && r <= '9' {
		return -1
	}
	return r
}

func TestTextFieldMultilineBreaksOnEnterAndSubmitsOnCmdEnter(t *testing.T) {
	t.Parallel()
	value := ggui.State("")
	submitted := ""
	single := ui.TextField(ggui.State("")).Name("Single")
	multi := ui.TextField(value).Name("Notes").Multiline().Lines(3).OnSubmit(func(s string) { submitted = s })
	p := ggui.NewProbe(ggui.Column(single, multi), ggui.Sz(300, 300))
	defer p.Close()
	one, _ := p.FindRole(ggui.RoleTextField, "Single")
	three, _ := p.FindRole(ggui.RoleTextField, "Notes")
	if three.Rect.Size.H < 1.5*one.Rect.Size.H {
		t.Fatalf("a three-line field is %v tall next to a single line's %v", three.Rect.Size.H, one.Rect.Size.H)
	}
	p.Tap("Notes")
	paste(p, "a")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	paste(p, "b")
	if got := ggui.Untrack(value.Get); got != "a\nb" || submitted != "" {
		t.Fatalf("Enter in a multiline field gave %q and submitted %q; want a line break and no submit", got, submitted)
	}
	p.Key("cmd+enter")
	if submitted != "a\nb" {
		t.Fatalf("cmd+enter submitted %q, want %q", submitted, "a\nb")
	}
}

func TestTextFieldMinWidthStyleAndBaselineShapeTheBox(t *testing.T) {
	t.Parallel()
	plain := ui.TextField(ggui.State("x")).Name("Plain")
	wide := ui.TextField(ggui.State("x")).Name("Wide").MinWidth(260)
	big := ui.TextField(ggui.State("x")).Name("Big").Style(ggui.TextStyle{Size: 40})
	p := ggui.NewProbe(ggui.Column(ggui.Row(plain), ggui.Row(wide), ggui.Row(big)), ggui.Sz(600, 400))
	defer p.Close()
	a, _ := p.FindRole(ggui.RoleTextField, "Plain")
	b, _ := p.FindRole(ggui.RoleTextField, "Wide")
	c, _ := p.FindRole(ggui.RoleTextField, "Big")
	// Left to choose its width, a field asks for its MinWidth.
	env := uitheme.Default().Apply(ggui.Env{})
	free := ggui.Loose(ggui.Sz(ggui.Unbounded, 100))
	if got, plainW := wide.Layout(free, env).W, plain.Layout(free, env).W; got < 260 || got <= plainW {
		t.Fatalf("MinWidth(260) field asks for %v (plain %v)", got, plainW)
	}
	if b.Rect.Size.W != 600 {
		t.Fatalf("MinWidth field is %v wide in a bounded 600 row, want it to fill", b.Rect.Size.W)
	}
	if c.Rect.Size.H <= a.Rect.Size.H {
		t.Fatalf("a 40px text style left the field %v tall (plain %v)", c.Rect.Size.H, a.Rect.Size.H)
	}
	base, ok := big.Baseline()
	if !ok || base <= 0 || base >= c.Rect.Size.H {
		t.Fatalf("baseline %v (%v) is not inside the %v tall field", base, ok, c.Rect.Size.H)
	}
	if small, _ := plain.Baseline(); small >= base {
		t.Fatalf("the larger style's baseline %v is not below the plain one's %v", base, small)
	}
}

func TestTextFieldPasswordHidesValueFromSemantics(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(ui.TextField(ggui.State("hunter2")).Name("Password").Password(), ggui.Sz(300, 60))
	defer p.Close()
	node, _ := p.Semantics().Find(ggui.RoleTextField, "Password")
	if strings.Contains(node.Value, "hunter2") {
		t.Fatalf("the password field exposes its secret as %q", node.Value)
	}
}

// Under a Probe, which has no platform IME, typed text reaches a focused
// text field at its caret, as it would on a platform without one.
func TestProbeTextTypesIntoAFocusedTextField(t *testing.T) {
	t.Parallel()
	v := ggui.State("")
	p := ggui.NewProbe(ui.TextField(v), ggui.Sz(200, 40))
	defer p.Close()
	size := p.Frame()
	p.Text("ab")
	if got := ggui.Untrack(v.Get); got != "" {
		t.Fatalf("an unfocused field took %q", got)
	}
	p.Click(ggui.Pt(size.W/2, size.H/2))
	p.Text("ab")
	p.Type(ggui.Mods{}, ggui.KeyArrowLeft)
	p.Text("c")
	if got := ggui.Untrack(v.Get); got != "acb" {
		t.Fatalf("value %q, want acb: the c at the caret", got)
	}
}
