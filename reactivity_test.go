package ggui

import (
	"errors"
	"testing"
)

// A remembered value outlives the rebuilds of the View that asked for it,
// and of a View its parent rebuilt, and goes once no build asks for it.
func TestRememberKeepsValuesAcrossRebuilds(t *testing.T) {
	page, keys := State(0), State([]string{"a", "b"})
	made := map[string]int{}
	disposed := map[string]int{}
	values := map[string]*StateValue[string]{}
	p := ProbeBuilder(func() Widget {
		return View(page, func(int) Widget {
			// The form is made again with every page, and asks again.
			return View(keys, func(ks []string) Widget {
				for _, k := range ks {
					values[k] = Remember(k, func() *StateValue[string] {
						made[k]++
						OnCleanup(func() { disposed[k]++ })
						return State(k)
					})
				}
				return Box()
			})
		})
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	values["a"].Set("edited")

	keys.Set([]string{"a", "b"}) // equal: no rebuild
	keys.Set([]string{"b", "a"})
	p.Frame()
	page.Set(1)
	p.Frame()
	if made["a"] != 1 || made["b"] != 1 {
		t.Fatalf("made %v, want each once", made)
	}
	if got := Untrack(values["a"].Get); got != "edited" {
		t.Fatalf("a holds %q after rebuilds, want the edit", got)
	}
	keys.Set([]string{"b"})
	p.Frame()
	if disposed["a"] != 1 || disposed["b"] != 0 {
		t.Fatalf("disposed %v, want a once and b not", disposed)
	}
	keys.Set([]string{"a", "b"})
	p.Frame()
	if made["a"] != 2 || Untrack(values["a"].Get) != "a" {
		t.Fatalf("a came back as %q after %d makes, want a fresh one", Untrack(values["a"].Get), made["a"])
	}
}

// Keys belong to their Component, together with the type of the value, and
// a Component's values go when it does.
func TestRememberScopesByComponentAndType(t *testing.T) {
	show := State(true)
	var ints [2]*StateValue[int]
	var str *StateValue[string]
	gone := 0
	p := ProbeBuilder(func() Widget {
		cell := func(i int) Widget {
			return Component(func() Widget {
				ints[i] = Remember("k", func() *StateValue[int] {
					OnCleanup(func() { gone++ })
					return State(i)
				})
				if i == 0 {
					str = Remember("k", func() *StateValue[string] { return State("s") })
				}
				return Box()
			})
		}
		return Column(cell(0), If(show, func() Widget { return cell(1) }))
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	if ints[0] == ints[1] || Untrack(ints[1].Get) != 1 || Untrack(str.Get) != "s" {
		t.Fatal("components shared a key, or one key gave one value for two types")
	}
	show.Set(false)
	p.Frame()
	if gone != 1 {
		t.Fatalf("%d values went with their component, want 1", gone)
	}
}

// Outside every builder Remember makes a value each time.
func TestRememberOutsideABuilder(t *testing.T) {
	a := Remember(1, func() *int { return new(int) })
	b := Remember(1, func() *int { return new(int) })
	if a == b {
		t.Fatal("remembered with no builder around")
	}
}

// A text input bound to a Draft shows the model until edited, commits on
// Enter, keeps a refused edit with its error, and drops an edit on Escape.
func TestTextInputCommitsAndRevertsADraft(t *testing.T) {
	saved := State("ada")
	commits := 0
	d := Draft[string](saved, func(v string) error {
		commits++
		if v == "" {
			return errors.New("a name is required")
		}
		saved.Set(v)
		return nil
	})
	input := TextInput(d)
	p := NewProbe(input, Sz(200, 30))
	defer p.Close()
	escape := KeyEvent{Kind: KeyPress, Key: KeyEscape}
	p.Click(Pt(10, 10))
	p.Key("end")
	p.Text("!")
	if Untrack(saved.Get) != "ada" || Untrack(d.Get) != "ada!" || !Untrack(d.Dirty().Get) {
		t.Fatalf("while typing: saved %q, draft %q", Untrack(saved.Get), Untrack(d.Get))
	}
	p.Key("enter")
	if Untrack(saved.Get) != "ada!" || Untrack(d.Dirty().Get) || commits != 1 {
		t.Fatalf("after Enter: saved %q, dirty %v, %d commits", Untrack(saved.Get), Untrack(d.Dirty().Get), commits)
	}

	p.Key("cmd+a", "backspace", "enter")
	if Untrack(d.Error().Get) != "a name is required" || Untrack(d.Get) != "" || Untrack(saved.Get) != "ada!" {
		t.Fatalf("refused: error %q, draft %q, saved %q", Untrack(d.Error().Get), Untrack(d.Get), Untrack(saved.Get))
	}
	p.Key("escape")
	if Untrack(d.Get) != "ada!" || Untrack(d.Error().Get) != "" || !input.ConsumesKey(escape) {
		t.Fatalf("after Escape: draft %q, error %q, Escape kept %v", Untrack(d.Get), Untrack(d.Error().Get), input.ConsumesKey(escape))
	}
	p.Key("escape")
	if input.ConsumesKey(escape) {
		t.Fatal("Escape on a clean field stayed with it, so a dialog around it would not close")
	}

	// The model changing under a clean draft shows through.
	saved.Set("grace")
	p.Frame()
	if Untrack(d.Get) != "grace" {
		t.Fatalf("draft %q after the model changed, want grace", Untrack(d.Get))
	}
}
