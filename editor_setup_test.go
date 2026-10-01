package ggui

import (
	"testing"
)

func TestTextInputIsFoundByPlaceholderThenName(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("")).Placeholder("Search")
	p := NewProbe(w, Sz(200, 30))
	defer p.Close()
	if _, ok := p.FindRole(RoleTextField, "Search"); !ok {
		t.Fatal("an unnamed field is not found by its placeholder")
	}
	w.Name("Query")
	if _, ok := p.FindRole(RoleTextField, "Query"); !ok {
		t.Fatal("a named field is not found by its name")
	}
	name := State("First")
	w.BindName(name)
	if _, ok := p.FindRole(RoleTextField, "First"); !ok {
		t.Fatal("a field is not found by its bound name")
	}
	name.Set("Second")
	if _, ok := p.FindRole(RoleTextField, "Second"); !ok || !w.HasName() {
		t.Fatal("the field's name did not follow its binding")
	}
}

func TestTextInputBindDisabledStopsInputAndActions(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value, disabled := State("abc"), State(true)
	w := TextInput(value).BindDisabled(disabled)
	p := NewProbe(w, Sz(200, 30))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if w.Focused() || Untrack(value.Get) != "abc" {
		t.Fatalf("a disabled field focused %v, value %q; want no focus and no edit", w.Focused(), Untrack(value.Get))
	}
	node, ok := p.Semantics().Find(RoleTextField, "")
	if !ok || !node.Disabled {
		t.Fatalf("a disabled field is described as %+v, want Disabled", node.Node)
	}
	p.Perform(node.ID, Action{Kind: ActionSetValue, Text: "set"})
	if got := Untrack(value.Get); got != "abc" {
		t.Fatalf("an assistive action set a disabled field to %q", got)
	}

	disabled.Set(false)
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if got := Untrack(value.Get); !w.Focused() || got != "ab" {
		t.Fatalf("re-enabled field: focused %v, value %q; want focus and \"ab\"", w.Focused(), got)
	}
	node, _ = p.Semantics().Find(RoleTextField, "")
	p.Perform(node.ID, Action{Kind: ActionSetValue, Text: "set"})
	if got := Untrack(value.Get); got != "set" {
		t.Fatalf("ActionSetValue gave %q, want \"set\"", got)
	}
}

func TestTextInputInheritsDisabledFromItsSubtree(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("abc")
	w := TextInput(value)
	p := NewProbe(Provide(InputDisabledKey, true, w), Sz(200, 30))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Type(Mods{}, KeyBackspace)
	if !w.IsDisabled() || w.Focused() || Untrack(value.Get) != "abc" {
		t.Fatalf("under InputDisabledKey: disabled %v, focused %v, value %q", w.IsDisabled(), w.Focused(), Untrack(value.Get))
	}
}

func TestTextInputSizesFromStyleLinesAndMinWidth(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	free := Loose(Sz(Unbounded, Unbounded))
	plain := TextInput(State(""))
	one := plain.Layout(free, Env{})
	if got := TextInput(State("")).MinWidth(50).Layout(free, Env{}); got.W != 50 {
		t.Fatalf("MinWidth(50) asks for %v wide with the width left to it", got.W)
	}
	big := TextInput(State("")).Style(TextStyle{Size: 2 * DefaultTextSize})
	if got := big.Layout(free, Env{}); got.H <= one.H*1.5 {
		t.Fatalf("a field at twice the text size is %v tall, against %v", got.H, one.H)
	}
	two := TextInput(State("")).Lines(2)
	if got := two.Layout(Loose(Sz(200, Unbounded)), Env{}); !two.multiline || got.H != two.linesHeight(2) {
		t.Fatalf("Lines(2) is %v tall (multiline %v), want two lines, %v", got.H, two.multiline, two.linesHeight(2))
	}
	least := TextInput(State("")).Lines(0)
	if got := least.Layout(Loose(Sz(200, Unbounded)), Env{}); got.H != least.linesHeight(1) {
		t.Fatalf("Lines(0) is %v tall, want at least one line", got.H)
	}
}

func TestTextInputBaselineIsTheFirstLineAscent(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	w := TextInput(State("x"))
	if _, ok := w.Baseline(); ok {
		t.Fatal("a field reports a baseline before it has been laid out")
	}
	w.Layout(Loose(Sz(200, 30)), Env{})
	got, ok := w.Baseline()
	if want := w.face(1).Metrics().HAscent; !ok || got != want {
		t.Fatalf("baseline = %v, %v; want the ascent %v", got, ok, want)
	}
	// Beside a box, which sits on its bottom edge, the field's baseline
	// lines up with that edge.
	row := Row(Box().Size(10, 40), w).Align(AlignBaseline)
	row.Layout(Loose(Sz(300, 100)), Env{})
	if b, ok := row.Baseline(); !ok || !closeTo(b, 40) {
		t.Fatalf("row baseline = %v, %v; want the field's, on the box's bottom at 40", b, ok)
	}
}

func TestTextInputKeyKeepsCaretAndFocusWhenMoved(t *testing.T) {
	t.Parallel()
	useFakeIME(t)
	value := State("hello")
	moved := State(false)
	var input *TextInputWidget
	p := ProbeBuilder(func() Widget {
		return Reactive(func() Widget {
			input = TextInput(value).Key("query")
			if moved.Get() {
				return Column(Box().Size(10, 40), input)
			}
			return Column(input)
		})
	}, Sz(200, 100))
	defer p.Close()
	p.Click(Pt(195, 5))
	p.Key("left", "left")
	moved.Set(true)
	p.Frame()
	p.Key("backspace")
	if got := Untrack(value.Get); !input.Focused() || got != "helo" {
		t.Fatalf("the moved field: focused %v, value %q; want focus and the caret carried over (\"helo\")", input.Focused(), got)
	}
}
