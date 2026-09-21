package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type optionControl struct {
	ggui.Widget
	popup   *ggui.PopupWidget
	options func([]string)
	bind    func(ggui.Readable[[]string])
	format  func(func(string) string)
}

func makeOptionControl(kind string, value ggui.Binding[string], changed func(string)) optionControl {
	if kind == "select" {
		w := ui.Select(value, []string{"Alpha", "Beta"}).Named("Choice").OnChange(changed)
		return optionControl{w, w.Popup(), func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.OptionsWhen(r) }, func(fn func(string) string) { w.Format(fn) }}
	}
	w := ui.Combobox(value, []string{"Alpha", "Beta"}).Named("Choice").OnChange(changed)
	return optionControl{w, w.Popup(), func(v []string) { w.Options(v) }, func(r ggui.Readable[[]string]) { w.OptionsWhen(r) }, func(fn func(string) string) { w.Format(fn) }}
}

func TestOptionsUpdateOpenControlWithoutWritingSelection(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		t.Run(kind, func(t *testing.T) {
			value := ggui.State("Alpha")
			changes := 0
			w := makeOptionControl(kind, value, func(string) { changes++ })
			p := ggui.NewProbe(ggui.Component(func() ggui.Widget { return ggui.Column(w) }), ggui.Sz(500, 400))
			defer p.Close()
			p.Tap("Choice")
			p.Advance(time.Second)
			options := []string{"Gamma", "Delta", "Epsilon"}
			w.options(options)
			options[0] = "Mutated"
			p.Frame()
			if !w.popup.IsOpen() || value.Get() != "Alpha" || changes != 0 {
				t.Fatal("option update closed popup or changed selection")
			}
			if _, ok := p.Find("Alpha"); ok {
				t.Fatal("removed option remains interactive")
			}
			p.Tap("Gamma")
			if value.Get() != "Gamma" || changes != 1 || w.popup.IsOpen() {
				t.Fatal("new option did not select its snapshot value")
			}
			p.Tap("Choice")
			w.options(nil)
			p.Frame()
			if !w.popup.IsOpen() || value.Get() != "Gamma" || changes != 1 {
				t.Fatal("empty options changed selection or closed popup")
			}
			p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
			if value.Get() != "Gamma" || changes != 1 {
				t.Fatal("empty options accepted a selection")
			}
		})
	}
}

func TestOptionsWhenLastSettingWinsInsideCaches(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		t.Run(kind, func(t *testing.T) {
			w := makeOptionControl(kind, ggui.State("Alpha"), nil)
			source := ggui.State([]string{"Alpha", "First"})
			w.bind(source)
			p := ggui.NewProbe(ggui.Cached(ggui.Component(func() ggui.Widget { return ggui.Column(w) })), ggui.Sz(500, 400))
			defer p.Close()
			p.Tap("Choice")
			p.Advance(time.Second)
			assertOption := func(label string) {
				t.Helper()
				if _, ok := p.Find(label); !ok {
					t.Fatalf("option %q missing", label)
				}
			}
			assertOption("First")
			source.Set([]string{"Alpha", "Second"})
			assertOption("Second")
			w.options([]string{"Alpha", "Static"})
			assertOption("Static")
			source.Set([]string{"Ignored"})
			assertOption("Static")
			replacement := ggui.State([]string{"Replacement"})
			w.bind(replacement)
			assertOption("Replacement")
			w.bind(nil)
			replacement.Set([]string{"Detached"})
			assertOption("Replacement")
			if _, ok := p.Find("Detached"); ok {
				t.Fatal("nil binding did not detach")
			}
		})
	}
}

func TestOptionsAndFormatComposeInEitherOrder(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		for _, formatFirst := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "/format-first", false: "/options-first"}[formatFirst], func(t *testing.T) {
				w := makeOptionControl(kind, ggui.State("Alpha"), nil)
				if formatFirst {
					w.format(strings.ToUpper)
				}
				w.options([]string{"Gamma"})
				if !formatFirst {
					w.format(strings.ToUpper)
				}
				p := ggui.NewProbe(ggui.Column(w), ggui.Sz(300, 300))
				defer p.Close()
				p.Tap("Choice")
				if _, ok := p.Find("GAMMA"); !ok {
					t.Fatal("current formatter was not applied")
				}
			})
		}
	}
}

func TestSelectOptionsResizeCachedTrigger(t *testing.T) {
	w := ui.Select(ggui.State("a"), []string{"a"}).Named("Choice")
	p := ggui.NewProbe(ggui.Component(func() ggui.Widget { return ggui.Column(w) }), ggui.Sz(600, 300))
	defer p.Close()
	before := find(t, p, ggui.RoleSelect, "Choice").Rect.Size.W
	w.Options([]string{"a", "A much longer option that needs a wider trigger"})
	after := find(t, p, ggui.RoleSelect, "Choice").Rect.Size.W
	if after <= before {
		t.Fatalf("cached width stayed %g after option replacement (before %g)", after, before)
	}
	source := ggui.State([]string{"a"})
	w.OptionsWhen(source)
	before = find(t, p, ggui.RoleSelect, "Choice").Rect.Size.W
	source.Set([]string{"a", "A longer option published through the bound source"})
	after = find(t, p, ggui.RoleSelect, "Choice").Rect.Size.W
	if after <= before {
		t.Fatal("bound options did not invalidate the cached trigger's width")
	}
}

func TestSelectOptionsResetKeyboardHighlightToValue(t *testing.T) {
	value := ggui.State("Beta")
	w := ui.Select(value, []string{"Alpha", "Beta"}).Named("Choice")
	p := ggui.NewProbe(ggui.Column(w), ggui.Sz(400, 350))
	defer p.Close()
	p.Tap("Choice")
	p.Type(ggui.Mods{}, ggui.KeyArrowUp)
	w.Options([]string{"Gamma", "Beta", "Delta"})
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if value.Get() != "Beta" {
		t.Fatal("replacement reused stale keyboard index")
	}
	p.Tap("Choice")
	w.Options([]string{"Gamma", "Delta"})
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if value.Get() != "Gamma" {
		t.Fatal("absent selection did not start navigation at the first option")
	}
}

func TestComboboxOptionsKeepSearchQueryAndFocus(t *testing.T) {
	value := ggui.State("Alpha")
	w := ui.Combobox(value, []string{"Alpha", "Beta"}).Named("Choice")
	p := ggui.NewProbe(ggui.Component(func() ggui.Widget { return ggui.Column(w) }), ggui.Sz(450, 400))
	defer p.Close()
	p.Tap("Choice")
	p.Tap("Search options")
	act(t, p, ggui.RoleTextField, "Search options", ggui.Action{Kind: ggui.ActionSetValue, Text: "be"})
	w.Options([]string{"Alpine", "Berry", "Beet"})
	p.Frame()
	if !w.Popup().IsOpen() {
		t.Fatal("option replacement closed the popup")
	}
	if got := node(t, p.Semantics(), ggui.RoleTextField, "Search options").Value; got != "be" {
		t.Fatalf("search query reset to %q", got)
	}
	if _, ok := p.Find("Alpine"); ok {
		t.Fatal("query was not applied to new options")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if value.Get() != "Beet" {
		t.Fatal("search lost keyboard focus or navigation did not use new matches")
	}
}

func TestRepeatedOptionConfigurationSettles(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		for _, bound := range []bool{false, true} {
			t.Run(kind+map[bool]string{true: "/bound", false: "/static"}[bound], func(t *testing.T) {
				w := makeOptionControl(kind, ggui.State("Alpha"), nil)
				source := ggui.State([]string{"Alpha", "Beta"})
				configure := func() { w.options([]string{"Alpha", "Beta"}) }
				if bound {
					configure = func() { w.bind(source) }
				}
				configure()
				layouts := 0
				parent := ggui.FromFuncs(func(c ggui.Constraints, env ggui.Env) ggui.Size {
					layouts++
					configure()
					return w.Layout(c, env)
				}, w.Paint)
				p := ggui.NewProbe(parent, ggui.Sz(300, 100))
				defer p.Close()
				p.Frame()
				p.Frame() // settle initial label measurement
				before := layouts
				p.Frame()
				p.Frame()
				if layouts != before {
					t.Fatalf("repeating unchanged options kept laying out: %d -> %d", before, layouts)
				}
			})
		}
	}
}

// A function-valued field makes the reader non-comparable, while its Get
// still participates in tracking through the signal it reads.
type optionReader struct{ read func() []string }

func (r optionReader) Get() []string { return r.read() }

func TestOptionsWhenSupportsCustomNonComparableReaders(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		t.Run(kind, func(t *testing.T) {
			source := ggui.State([]string{"Alpha"})
			reader := optionReader{source.Get}
			w := makeOptionControl(kind, ggui.State("Alpha"), nil)
			w.bind(reader)
			w.bind(reader) // must not compare non-comparable interfaces with ==
			p := ggui.NewProbe(ggui.Cached(ggui.Column(w)), ggui.Sz(400, 350))
			defer p.Close()
			p.Tap("Choice")
			source.Set([]string{"Beta"})
			if _, ok := p.Find("Beta"); !ok {
				t.Fatal("custom reader's signal dependency was not tracked under cache")
			}
		})
	}
}

func TestOptionsReplacementCancelsPressOnRemovedRow(t *testing.T) {
	for _, kind := range []string{"select", "combobox"} {
		t.Run(kind, func(t *testing.T) {
			value := ggui.State("Alpha")
			w := makeOptionControl(kind, value, nil)
			p := ggui.NewProbe(ggui.Column(w), ggui.Sz(400, 350))
			defer p.Close()
			p.Tap("Choice")
			p.Advance(time.Second)
			row, ok := p.Find("Beta")
			if !ok {
				t.Fatal("missing initial row")
			}
			p.Press(row.Center())
			w.options([]string{"Gamma", "Delta"})
			p.Release(row.Center())
			if value.Get() != "Alpha" || !w.popup.IsOpen() {
				t.Fatal("release selected a replacement row the user never pressed")
			}
		})
	}
}
