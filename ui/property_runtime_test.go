package ui

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
)

type selectionProperties struct {
	widget ggui.Widget
	set    func([]string)
	bind   func(ggui.Readable[[]string])
	values func() []string
	labels func() []string
}

func selectionPropertyFixture(kind string, value ggui.Binding[string], changed func(string)) selectionProperties {
	switch kind {
	case "select":
		w := Select(value).Format(strings.ToUpper).OnChange(changed)
		return selectionProperties{w, func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.BindOptions(r) }, func() []string { return w.options }, func() []string {
			out := []string{}
			for _, it := range w.items {
				out = append(out, it.SemanticName())
			}
			return out
		}}
	case "combobox":
		w := Combobox(value).Format(strings.ToUpper).OnChange(changed)
		return selectionProperties{w, func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.BindOptions(r) }, func() []string { return w.options }, func() []string {
			out := []string{}
			for _, it := range w.search.entries {
				out = append(out, it.label)
			}
			return out
		}}
	case "radios":
		w := Radios(value).Format(strings.ToUpper).OnChange(changed)
		return selectionProperties{w, func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.BindOptions(r) }, func() []string { return w.options }, func() []string {
			out := []string{}
			for _, it := range w.radios {
				out = append(out, it.SemanticName())
			}
			return out
		}}
	default:
		w := ToggleGroup(value).Format(strings.ToUpper).OnChange(changed)
		return selectionProperties{w, func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.BindOptions(r) }, func() []string { return w.options }, func() []string { return w.names }}
	}
}
func TestSelectionPropertySnapshotsAndBindingReplacement(t *testing.T) {
	for _, kind := range []string{"select", "combobox", "radios", "toggle"} {
		t.Run(kind, func(t *testing.T) {
			value := ggui.State("missing")
			changes := 0
			w := selectionPropertyFixture(kind, value, func(string) { changes++ })
			p := ggui.NewProbe(ggui.Component(func() ggui.Widget { return ggui.Column(ggui.Cached(w.widget)) }), ggui.Sz(400, 300))
			defer p.Close()
			p.Frame()
			if len(w.values()) != 0 {
				t.Fatal("constructor options not empty")
			}
			input := []string{"a", "b"}
			w.set(input)
			input[0] = "mutated"
			p.Frame()
			if strings.Join(w.values(), ",") != "a,b" || strings.Join(w.labels(), ",") != "A,B" {
				t.Fatal("snapshot or formatter", w.values(), w.labels())
			}
			a, b := ggui.State([]string{"a", "b"}), ggui.State([]string{"a", "b"})
			w.bind(a)
			p.Frame()
			w.bind(b)
			p.Frame()
			a.Set([]string{"old"})
			p.Frame()
			if w.values()[0] != "a" {
				t.Fatal("old source still connected")
			}
			b.Set([]string{"b", "c", "c"})
			p.Frame()
			if strings.Join(w.labels(), ",") != "B,C,C" {
				t.Fatal("replacement source not tracked")
			}
			w.set([]string{"b", "c", "c"})
			b.Set([]string{"ignored"})
			p.Frame()
			if strings.Join(w.values(), ",") != "b,c,c" {
				t.Fatal("equal literal did not detach")
			}
			w.set(nil)
			p.Frame()
			if len(w.values()) != 0 || value.Get() != "missing" || changes != 0 {
				t.Fatal("list replacement wrote selection")
			}
		})
	}
}

func TestRadiosKeepDuplicateOccurrenceIdentityAndFocus(t *testing.T) {
	value := ggui.State("a")
	w := Radios(value).Options([]string{"a", "a", "b"})
	p := ggui.NewProbe(ggui.Cached(ggui.Column(w)), ggui.Sz(400, 200))
	defer p.Close()
	p.Tap("a")
	first, second := w.radios[0], w.radios[1]
	w.Options([]string{"b", "a", "a", "c"})
	p.Frame()
	if w.radios[1] != first || w.radios[2] != second || !first.Focused {
		t.Fatal("surviving duplicate identities or focus lost")
	}
	b, ok := p.Find("b")
	if !ok {
		t.Fatal("missing b")
	}
	p.Press(b.Center())
	w.Options([]string{"c"})
	p.Release(b.Center())
	if value.Get() != "a" {
		t.Fatal("removed radio completed an old press")
	}
}

func TestToggleOptionsRejectStaleSegmentsAndKeepFocus(t *testing.T) {
	value := ggui.State("a")
	w := ToggleGroup(value).Options([]string{"a", "b"})
	p := ggui.NewProbe(ggui.Column(w), ggui.Sz(400, 200))
	defer p.Close()
	p.Tap("a")
	p.Type(ggui.Mods{}, ggui.KeyTab)
	old := toggleSegment[string]{g: w, i: 0, version: w.optionsVersion}
	w.Options([]string{"c", "d"})
	p.Frame()
	if old.Act(ggui.Action{Kind: ggui.ActionSelect}) || value.Get() != "a" {
		t.Fatal("old segment selected replacement")
	}
	if !old.Describe().Disabled {
		t.Fatal("old segment exposes active semantics")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	if value.Get() != "c" {
		t.Fatal("group keyboard focus lost or absent selection mishandled", value.Get())
	}
	w.Options(nil)
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyEnter)
	if value.Get() != "c" {
		t.Fatal("empty group changed selection")
	}
}

func TestCompositeNamesUseSameTargetsAndDetach(t *testing.T) {
	for _, kind := range []string{"text", "otp", "combo", "group"} {
		t.Run(kind, func(t *testing.T) {
			source := ggui.State("first")
			var widget ggui.Widget
			var literal func(string)
			var name func() string
			switch kind {
			case "text":
				w := TextField(ggui.State("")).BindName(source)
				widget = w
				literal = func(s string) { w.Name(s) }
				name = w.input.SemanticName
			case "otp":
				w := InputOTP(ggui.State(""), 4).BindName(source)
				widget = w
				literal = func(s string) { w.Name(s) }
				name = w.input.SemanticName
			case "combo":
				w := Combobox(ggui.State("a")).BindName(source)
				widget = w
				literal = func(s string) { w.Name(s) }
				name = w.button.SemanticName
			case "group":
				field := TextField(ggui.State(""))
				w := InputGroup(field).BindName(source)
				widget = w
				literal = func(s string) { w.Name(s) }
				name = field.input.SemanticName
			}
			p := ggui.NewProbe(ggui.Cached(widget), ggui.Sz(400, 200))
			defer p.Close()
			p.Frame()
			source.Set("second")
			p.Frame()
			if name() != "second" {
				t.Fatal("name did not reach semantic target", name())
			}
			literal("second")
			source.Set("ignored")
			p.Frame()
			if name() != "second" {
				t.Fatal("literal name did not detach", name())
			}
		})
	}
}

func TestUIBindingsRejectNilAndTypedNil(t *testing.T) {
	var options *ggui.StateValue[[]string]
	var text *ggui.StateValue[string]
	var invalid *ggui.StateValue[bool]
	var state *ggui.StateValue[AttachmentState]
	runs := []func(){
		func() { Select(ggui.State("")).BindOptions(nil) }, func() { Combobox(ggui.State("")).BindOptions(options) },
		func() { Radios(ggui.State("")).BindOptions(options) }, func() { ToggleGroup(ggui.State("")).BindOptions(options) },
		func() { Field("label", ggui.Text("value")).BindError(text) }, func() { InputOTP(ggui.State(""), 4).BindInvalid(invalid) },
		func() { Attachment("file", "").BindState(state) }, func() { Button("x", nil).BindName(text) },
	}
	for i, run := range runs {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("binding %d accepted nil", i)
				}
			}()
			run()
		}()
	}
}

func TestRepeatedRuntimeConfigurationPreservesChildren(t *testing.T) {
	table := Table(ggui.State([]int{1, 2}), func(v int) int { return v },
		Col("Value", func(v ggui.Readable[int]) ggui.Widget { return ggui.Textf("%d", v) }))
	p := ggui.NewProbe(ggui.Cached(table), ggui.Sz(300, 400))
	defer p.Close()
	p.Advance(0)
	natural := p.Frame().H
	table.Height(100)
	if got := p.Frame().H; got != 100 {
		t.Fatalf("runtime height = %v", got)
	}
	column := table.column
	revision := table.props.Version()
	table.Height(100)
	if table.column != column || table.props.Version() != revision {
		t.Fatal("equal height replaced children or invalidated layout")
	}
	table.Height(0)
	if got := p.Frame().H; got != natural {
		t.Fatalf("cleared height = %v, want natural %v", got, natural)
	}

	item := MenuItem("Save", nil).Shortcut("Ctrl+S")
	shortcut := item.shortcut
	item.Shortcut("Ctrl+S")
	if item.shortcut != shortcut {
		t.Fatal("equal shortcut replaced its text widget")
	}
	item.Shortcut("Cmd+S")
	if item.shortcut != shortcut {
		t.Fatal("changed shortcut replaced its text widget")
	}
}
