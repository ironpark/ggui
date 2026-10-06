package ggui

import "testing"

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
