package ggui

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
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

type storeModel struct {
	title string
	tags  []string
}

// A Select follows what Update and Changed publish, passes on a change
// only when its part differs, and an Update inside another publishes once.
func TestStoreSelectFollowsPublishedChanges(t *testing.T) {
	store := NewStore(&storeModel{title: "a"})
	title := Select(store, func(m *storeModel) string { return m.title })
	tags := Select(store, func(m *storeModel) []string { return slices.Clone(m.tags) }).WithEqual(slices.Equal)
	runs := 0
	p := ProbeBuilder(func() Widget { return TextOf(title) }, Sz(100, 20))
	defer p.Close()
	p.Setup(func() { Watch(title, func(string) { runs++ }) })
	p.Frame()
	if got := Untrack(title.Get); got != "a" {
		t.Fatalf("title %q", got)
	}
	store.Update(func(m *storeModel) {
		m.title = "b"
		store.Update(func(m *storeModel) { m.tags = append(m.tags, "x") })
	})
	if v := Untrack(store.version.Get); v != 1 {
		t.Errorf("nested Updates published %d times, want 1", v)
	}
	p.Frame()
	if got := Untrack(title.Get); got != "b" || !slices.Equal(Untrack(tags.Get), []string{"x"}) {
		t.Fatalf("after Update: %q %v", got, Untrack(tags.Get))
	}
	store.Model().tags = append(store.Model().tags, "y")
	store.Changed()
	p.Frame()
	if got := Untrack(tags.Get); !slices.Equal(got, []string{"x", "y"}) {
		t.Fatalf("after Changed: %v", got)
	}
	if runs != 2 {
		t.Errorf("the title watcher ran %d times, want 2: once at first and once for its change", runs)
	}
}

// SelectBind hands a control's edit to the model as an Update.
func TestSelectBindWritesThroughUpdate(t *testing.T) {
	store := NewStore(&storeModel{title: "a"})
	b := SelectBind(store, func(m *storeModel) string { return m.title }, func(m *storeModel, v string) { m.title = v })
	b.Set("z")
	if store.Model().title != "z" || Untrack(b.Get) != "z" {
		t.Fatalf("model %q, binding %q", store.Model().title, Untrack(b.Get))
	}
	store.Action(func(m *storeModel) { m.title = "w" })()
	if Untrack(b.Get) != "w" {
		t.Fatalf("after Action: %q", Untrack(b.Get))
	}
}

func TestSignalGetSet(t *testing.T) {
	t.Parallel()
	s := State(1)
	if got := s.Get(); got != 1 {
		t.Fatalf("Get() = %d, want 1", got)
	}
	s.Update(func(n int) int { return n + 41 })
	if got := s.Get(); got != 42 {
		t.Fatalf("Get() = %d, want 42", got)
	}
}

func TestEffectRerunsOnChange(t *testing.T) {
	t.Parallel()
	s := State("a")
	var seen []string
	dispose := reactive.Observe(func() { seen = append(seen, s.Get()) })
	defer dispose()

	if len(seen) != 1 || seen[0] != "a" {
		t.Fatalf("effect did not run once on registration: %v", seen)
	}

	s.Set("b")
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 2 || seen[1] != "b" {
		t.Fatalf("effect did not re-run after Set: %v", seen)
	}

	// A flush with nothing dirty must not re-run the effect.
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 2 {
		t.Fatalf("effect re-ran while clean: %v", seen)
	}
}

func TestIdleEffectFlushDoesNotAllocate(t *testing.T) {
	s := State(0)
	dispose := Root(func() {
		for range 100 {
			reactive.Observe(func() { s.Get() })
		}
	})
	defer dispose()
	if !reactive.Flush() {
		t.Fatal("initial flush did not settle")
	}
	if got := testing.AllocsPerRun(100, func() { reactive.Flush() }); got != 0 {
		t.Fatalf("idle flush allocated %g times", got)
	}
}

func TestEffectFlushIncludesCleanupWrites(t *testing.T) {
	t.Parallel()
	trigger, result := State(0), State(0)
	var seen []int
	dispose := Root(func() {
		// The observer has already run when a later effect's cleanup
		// writes, so settling requires another pass in creation order.
		reactive.Observe(func() { seen = append(seen, result.Get()) })
		reactive.Observe(func() {
			trigger.Get()
			OnCleanup(func() { result.Set(Untrack(trigger.Get)) })
		})
	})
	defer dispose()
	reactive.Flush()
	reactive.FlushUsers(nil)
	for _, v := range []int{1, 2} {
		trigger.Set(v)
		if !reactive.Flush() {
			t.Fatal("cleanup write did not settle")
		}
		reactive.Flush()
		reactive.FlushUsers(nil)
	}
	if !slices.Equal(seen, []int{0, 1, 2}) {
		t.Fatalf("observer saw %v, want [0 1 2]", seen)
	}
}

func TestEffectFlushIncludesWritesAfterNestedFlush(t *testing.T) {
	t.Parallel()
	trigger, before, after := State(0), State(0), State(0)
	var seenBefore, seenAfter []int
	dispose := Root(func() {
		reactive.Observe(func() { seenBefore = append(seenBefore, before.Get()) })
		reactive.Observe(func() { seenAfter = append(seenAfter, after.Get()) })
		reactive.Observe(func() {
			if v := trigger.Get(); v != 0 {
				before.Set(v)
				if !reactive.Flush() {
					t.Fatal("nested flush did not settle")
				}
				after.Set(v)
			}
		})
	})
	defer dispose()
	reactive.Flush()
	reactive.FlushUsers(nil)
	trigger.Set(1)
	if !reactive.Flush() {
		t.Fatal("outer flush did not settle")
	}
	if !slices.Equal(seenBefore, []int{0, 1}) || !slices.Equal(seenAfter, []int{0, 1}) {
		t.Fatalf("observers saw %v and %v, want [0 1] each", seenBefore, seenAfter)
	}
}

func TestEffectCycleIsNotRecordedAsSettled(t *testing.T) {
	t.Parallel()
	reactive.Flush()
	reactive.FlushUsers(nil)
	n := State(0)
	dispose := reactive.Observe(func() { n.Set(n.Get() + 1) })
	defer dispose()
	for range 2 {
		before := Untrack(n.Get)
		if reactive.Flush() {
			t.Fatal("cyclic effect was considered settled")
		}
		if Untrack(n.Get) <= before {
			t.Fatal("flush skipped a cycle left pending by the previous flush")
		}
	}
}

func TestEffectDisposeStopsUpdates(t *testing.T) {
	t.Parallel()
	s := State(0)
	runs := 0
	dispose := reactive.Observe(func() { s.Get(); runs++ })
	dispose()

	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after dispose", runs)
	}
}

func TestUntrackedReadDoesNotSubscribe(t *testing.T) {
	t.Parallel()
	s := State(0)
	s.Get() // outside any effect
	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil) // must not panic on a nil subscriber
}

func TestSetSkipsEqualValue(t *testing.T) {
	t.Parallel()
	s := State(1)
	runs := 0
	dispose := reactive.Observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 after writing an equal value", runs)
	}
	s.Set(2)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 after writing a new value", runs)
	}
}

func TestSetNotifiesForUncomparableValue(t *testing.T) {
	t.Parallel()
	s := State([]int{1})
	runs := 0
	dispose := reactive.Observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2: slices have no default equality", runs)
	}
}

func TestWithEqualOverridesComparison(t *testing.T) {
	t.Parallel()
	s := State([]int{1}).WithEqual(slices.Equal)
	runs := 0
	dispose := reactive.Observe(func() { s.Get(); runs++ })
	defer dispose()

	s.Set([]int{1})
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 1 {
		t.Fatalf("runs = %d, want 1 with a custom equality", runs)
	}
}

func TestSignalMapDerivesValue(t *testing.T) {
	t.Parallel()
	n := State(2)
	label := n.Map(func(v int) string { return strconv.Itoa(v * 10) })
	defer label.Dispose()

	if got := label.Get(); got != "20" {
		t.Fatalf("Get() = %q, want %q", got, "20")
	}
	n.Set(3)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := label.Get(); got != "30" {
		t.Fatalf("Get() = %q, want %q after Set", got, "30")
	}
}

func TestMemoChainSettlesInOneFlush(t *testing.T) {
	t.Parallel()
	n := State(1)
	doubled := n.Map(func(v int) int { return v * 2 })
	defer doubled.Dispose()
	label := doubled.Map(func(v int) string { return strconv.Itoa(v) })
	defer label.Dispose()

	var seen []string
	dispose := reactive.Observe(func() { seen = append(seen, label.Get()) })
	defer dispose()

	n.Set(21)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 2 || seen[1] != "42" {
		t.Fatalf("seen = %v, want the chain to settle in one flush", seen)
	}
}

func TestMemoRecomputesOnlyWhenSourceChanges(t *testing.T) {
	t.Parallel()
	n := State(1)
	runs := 0
	m := n.Map(func(v int) int { runs++; return v % 2 })
	defer m.Dispose()

	var seen []int
	dispose := reactive.Observe(func() { seen = append(seen, m.Get()) })
	defer dispose()

	n.Set(3) // different source, same result: the reader must not re-run
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 2 {
		t.Fatalf("runs = %d, want 2 recomputes", runs)
	}
	if len(seen) != 1 {
		t.Fatalf("seen = %v, want the reader left alone for an unchanged result", seen)
	}
}

func TestMemoDisposeStopsRecomputation(t *testing.T) {
	t.Parallel()
	n := State(1)
	m := n.Map(func(v int) int { return v + 1 })
	m.Get()
	m.Dispose()

	n.Set(10)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := m.Get(); got != 2 {
		t.Fatalf("Get() = %d, want the last value 2 after Dispose", got)
	}
}

func TestToggleAndAdd(t *testing.T) {
	t.Parallel()
	on := State(false)
	n := State(1.5)
	Toggle(on)
	Add(n, 2)
	if !on.Get() || n.Get() != 3.5 {
		t.Fatalf("on = %v, n = %v; want true, 3.5", on.Get(), n.Get())
	}
}

func TestCombineDerivesFromTwoSources(t *testing.T) {
	t.Parallel()
	a := State(2)
	b := State("x")
	joined := Combine(a, b, func(n int, s string) string { return strings.Repeat(s, n) })
	defer joined.Dispose()

	if got := joined.Get(); got != "xx" {
		t.Fatalf("Get() = %q, want %q", got, "xx")
	}
	b.Set("ab")
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := joined.Get(); got != "abab" {
		t.Fatalf("Get() = %q, want %q", got, "abab")
	}
}

func TestWatchRunsOnChange(t *testing.T) {
	t.Parallel()
	s := State(0)
	var seen []int
	dispose := Root(func() { Watch(s, func(v int) { seen = append(seen, v) }) })
	reactive.FlushUsers(nil)
	defer dispose()

	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 2 || seen[0] != 0 || seen[1] != 1 {
		t.Fatalf("seen = %v, want [0 1]", seen)
	}
}

func TestSignalWritesAdvanceTheLayoutGeneration(t *testing.T) {
	t.Parallel()
	s := State(1)
	before := reactive.LayoutGen()
	s.Set(1)
	if reactive.LayoutGen() != before {
		t.Fatal("an equal write must not request layout")
	}
	s.Set(2)
	written := reactive.LayoutGen()
	if written == before {
		t.Fatal("a write must request layout")
	}
	Invalidate(Env{})
	if reactive.LayoutGen() == written {
		t.Fatal("Invalidate must request layout")
	}
}

func TestAppendAndRemoveOnSliceSignals(t *testing.T) {
	t.Parallel()
	s := State([]int{1, 2, 3})
	runs := 0
	dispose := reactive.Observe(func() { s.Get(); runs++ })
	defer dispose()
	Append(s, 4, 5)
	reactive.Flush()
	reactive.FlushUsers(nil)
	Remove(s, func(n int) bool { return n%2 == 0 })
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := Untrack(s.Get); len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("slice = %v, want [1 3 5]", got)
	}
	if runs != 3 {
		t.Fatalf("effect ran %d times, want 3 (initial, append, remove)", runs)
	}
	Remove(s, func(int) bool { return false })
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 3 {
		t.Fatalf("effect ran %d times after a Remove that matched nothing, want still 3", runs)
	}
}

// A reader of two derived values must never see one of them updated and the
// other not, whatever order the effects were created in. Here the Watch is
// registered before the memo it reads, which is what an effect that outlives
// a rebuilt memo ends up looking like.
func TestReaderSeesNoPartialUpdate(t *testing.T) {
	t.Parallel()
	s := State(0)
	var b *DerivedValue[int]
	var seen [][2]int

	a := Derived(func() int { return s.Get() * 10 })
	defer a.Dispose()
	defer Root(func() {
		Watch(a, func(av int) {
			if b != nil {
				seen = append(seen, [2]int{av, b.Get()})
			}
		})
	})()
	b = Derived(func() int { return s.Get() * 100 })
	defer b.Dispose()

	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 1 || seen[0] != [2]int{10, 100} {
		t.Fatalf("watch saw %v, want one call with [10 100]", seen)
	}
}

// The same, with the second value two derivations away from the signal: the
// far memo is not itself marked by the write, only checked through the one
// above it.
func TestReaderSeesNoPartialUpdateThroughChain(t *testing.T) {
	t.Parallel()
	s := State(0)
	var far *DerivedValue[int]
	var seen [][2]int

	a := Derived(func() int { return s.Get() * 10 })
	defer a.Dispose()
	defer Root(func() {
		Watch(a, func(av int) {
			if far != nil {
				seen = append(seen, [2]int{av, far.Get()})
			}
		})
	})()
	near := Derived(func() int { return s.Get() * 100 })
	defer near.Dispose()
	far = near.Map(func(v int) int { return v + 1 })
	defer far.Dispose()

	s.Set(1)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if len(seen) != 1 || seen[0] != [2]int{10, 101} {
		t.Fatalf("watch saw %v, want one call with [10 101]", seen)
	}
}

// A memo read from outside any effect, between writes, reports the value its
// inputs imply now rather than the one the last flush left behind.
func TestMemoReadOutsideEffectIsCurrent(t *testing.T) {
	t.Parallel()
	s := State(2)
	m := Derived(func() int { return s.Get() * 3 })
	defer m.Dispose()
	if got := m.Get(); got != 6 {
		t.Fatalf("got %d, want 6", got)
	}
	s.Set(5)
	if got := m.Get(); got != 15 {
		t.Fatalf("got %d before flush, want 15", got)
	}
}

type form struct {
	Name string
	Age  int
}

func TestLensReadsAndWritesThrough(t *testing.T) {
	t.Parallel()
	f := State(form{Name: "a", Age: 1})
	name := f.Lens(func(v form) string { return v.Name }, func(v form, s string) form { v.Name = s; return v })
	var b Binding[string] = name
	runs := 0
	dispose := reactive.Observe(func() { runs++; b.Get() })
	defer dispose()
	b.Set("bb")
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := Untrack(f.Get); got.Name != "bb" || got.Age != 1 {
		t.Fatalf("whole = %+v", got)
	}
	if runs != 2 || Untrack(b.Get) != "bb" {
		t.Fatalf("runs = %d, peek = %q", runs, Untrack(b.Get))
	}
}

func TestTweenIsABinding(t *testing.T) {
	t.Parallel()
	var b Binding[float64] = Tween(0.0, time.Millisecond)
	b.Set(1)
	if Untrack(b.Get) != 0 {
		t.Fatalf("tween jumped: %v", Untrack(b.Get))
	}
}

func TestForRetainEvictsOffscreenRows(t *testing.T) {
	t.Parallel()
	var items []int
	for i := range 100 {
		items = append(items, i)
	}
	src := State(items)
	var f *EachWidget[int, int]
	off := State(0.0)
	dispose := reactive.Observe(func() {
		f = EachKeyed(src, func(i int) int { return i }, func(EachItem[int]) Widget { return Box().Size(10, 10) }).ItemExtent(10).Retain(2)
	})
	defer dispose()
	p := NewProbe(Scroll(f).BindOffset(off), Sz(10, 50))
	p.Frame()
	off.Set(500)
	p.Frame()
	if n := len(f.entries); n != 5+2 {
		t.Fatalf("mounted rows = %d, want 7", n)
	}
}

// independentWritable deliberately does not embed StateValue: the helpers must
// depend only on the public contract, including generic argument inference.
type independentWritable[T any] struct {
	value   T
	writes  int
	updates int
}

func (w *independentWritable[T]) Get() T              { return w.value }
func (w *independentWritable[T]) Set(v T)             { w.value = v; w.writes++ }
func (w *independentWritable[T]) Update(fn func(T) T) { w.updates++; w.Set(fn(w.value)) }

func TestWritableHelpersOnSignalsLensesAndCustomValues(t *testing.T) {
	t.Parallel()
	state := State(form{Name: "kept", Age: 1})
	age := state.Lens(func(f form) int { return f.Age }, func(f form, v int) form { f.Age = v; return f })
	custom := &independentWritable[int]{value: 1}
	for _, w := range []Writable[int]{State(1), age, custom} {
		Add(w, 2)
		Add(w, 3)
		if Untrack(w.Get) != 6 {
			t.Fatalf("accumulated value=%d", Untrack(w.Get))
		}
	}
	if Untrack(state.Get).Name != "kept" || custom.updates != 2 {
		t.Fatal("update did not use writable contract")
	}
	boolState := State(struct{ On bool }{})
	flag := boolState.Lens(func(v struct{ On bool }) bool { return v.On }, func(v struct{ On bool }, b bool) struct{ On bool } { v.On = b; return v })
	Toggle(flag)
	if !Untrack(boolState.Get).On {
		t.Fatal("toggle did not write through lens")
	}
	// Inference also works without an interface-typed intermediate variable.
	Add(custom, 1)
	Toggle(&independentWritable[bool]{})
}

func TestWritableSlicesCopyAndSkipNoopRemoval(t *testing.T) {
	t.Parallel()
	type model struct {
		Items []int
		Other int
	}
	items := make([]int, 2, 8)
	items[0], items[1] = 1, 2
	state := State(model{Items: items, Other: 7})
	lens := state.Lens(func(m model) []int { return m.Items }, func(m model, v []int) model { m.Items = v; return m })
	runs := 0
	dispose := reactive.Observe(func() { state.Get(); runs++ })
	defer dispose()
	Remove(lens, func(v int) bool { return v == 9 })
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 1 {
		t.Fatal("no-op remove notified readers")
	}
	Append(lens, 3)
	reactive.Flush()
	reactive.FlushUsers(nil)
	if runs != 2 || items[:3][2] != 0 {
		t.Fatal("append reused backing storage or missed notification")
	}
	Remove(lens, func(v int) bool { return v == 2 })
	reactive.Flush()
	reactive.FlushUsers(nil)
	got := Untrack(state.Get)
	if runs != 3 || len(got.Items) != 2 || got.Items[0] != 1 || got.Items[1] != 3 || got.Other != 7 {
		t.Fatalf("result=%+v, runs=%d", got, runs)
	}
	custom := &independentWritable[[]int]{value: []int{1}}
	Remove(custom, func(int) bool { return false })
	if custom.writes != 0 {
		t.Fatal("no-op remove wrote custom value")
	}
	Append(custom, 2)
	if custom.updates != 1 || len(custom.value) != 2 {
		t.Fatal("append bypassed custom Update")
	}
}

func TestAnimatedBindingsAreNotWritable(t *testing.T) {
	t.Parallel()
	for _, b := range []Binding[float64]{Tween(0.0, time.Second), Spring(0.0)} {
		if _, ok := b.(Writable[float64]); ok {
			t.Fatal("animated binding must not support immediate updates")
		}
	}
}

// Field is Lens with one closure instead of two: it reads and writes through
// a pointer into a copy of the whole.
func TestFieldLensReadsAndWritesThrough(t *testing.T) {
	t.Parallel()
	type form struct {
		Name string
		Age  int
	}
	f := State(form{Name: "a", Age: 1})
	name := f.Field(func(v *form) *string { return &v.Name })

	seen := ""
	defer Root(func() { Watch(name, func(s string) { seen = s }) })()
	if name.Get() != "a" || Untrack(name.Get) != "a" {
		t.Fatalf("read %q/%q, want a", name.Get(), Untrack(name.Get))
	}

	name.Set("b")
	reactive.Flush()
	reactive.FlushUsers(nil)
	if got := Untrack(f.Get); got.Name != "b" || got.Age != 1 {
		t.Fatalf("writing the part left the whole as %+v", got)
	}
	if seen != "b" {
		t.Fatalf("the watcher saw %q", seen)
	}

	name.Update(func(s string) string { return s + "!" })
	if got := Untrack(f.Get).Name; got != "b!" {
		t.Fatalf("Update through the field gave %q", got)
	}

	// Writing the whole is seen through the field.
	f.Set(form{Name: "c", Age: 2})
	reactive.Flush()
	reactive.FlushUsers(nil)
	if Untrack(name.Get) != "c" || seen != "c" {
		t.Fatalf("peek %q, watcher %q after the whole was replaced", Untrack(name.Get), seen)
	}
}

// Controlled shows the value it was built with and hands a change to set,
// which may refuse it.
func TestControlledAsksSetForAChange(t *testing.T) {
	t.Parallel()
	var asked []int
	b := Controlled(1, func(v int) { asked = append(asked, v) })
	b.Set(2)
	if b.Get() != 1 || !slices.Equal(asked, []int{2}) {
		t.Fatalf("Get = %d, asked %v; want 1 and [2]", b.Get(), asked)
	}
}

func TestDerivedIsLazyPureAndUntrackIsFresh(t *testing.T) {
	t.Parallel()
	n := State(1)
	calls := 0
	m := Derived(func() int { calls++; return n.Get() * 2 })
	defer m.Dispose()
	if calls != 0 {
		t.Fatal("derived evaluated at construction")
	}
	if Untrack(m.Get) != 2 || calls != 1 {
		t.Fatal("first read")
	}
	n.Set(2)
	reactive.Flush()
	if calls != 1 {
		t.Fatal("unobserved derived evaluated during flush")
	}
	if Untrack(m.Get) != 4 || calls != 2 {
		t.Fatal("untracked read was stale")
	}
	bad := Derived(func() int { n.Set(9); return 0 })
	defer bad.Dispose()
	assertPanics(t, func() { bad.Get() })
	if n.Get() != 2 {
		t.Fatal("derived changed user state")
	}
	var cycle *DerivedValue[int]
	cycle = Derived(func() int { return cycle.Get() })
	defer cycle.Dispose()
	assertPanics(t, func() { cycle.Get() })
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected panic")
		}
	}()
	fn()
}

func TestPublicEffectMountOrderCleanupAndDispose(t *testing.T) {
	t.Parallel()
	n, shown := State(0), State(true)
	var log []string
	p := ProbeBuilder(func() Widget {
		return If(shown, func() Widget {
			Effect(func() Cleanup { n.Get(); log = append(log, "effect"); return func() { log = append(log, "cleanup") } })
			return FromFuncs(func(c Constraints, env Env) Size { log = append(log, "layout"); return c.Constrain(Size{}) }, func(*Canvas, Rect) { log = append(log, "paint") })
		})
	}, Sz(20, 20))
	p.Frame()
	if !reflect.DeepEqual(log, []string{"layout", "effect", "paint"}) {
		t.Fatalf("first frame: %v", log)
	}
	log = nil
	n.Set(1)
	n.Set(2)
	p.Frame()
	if !slices.Equal(log, []string{"layout", "cleanup", "effect", "paint"}) && !slices.Equal(log, []string{"cleanup", "effect", "paint"}) {
		t.Fatalf("update: %v", log)
	}
	log = nil
	shown.Set(false)
	p.Frame()
	if !slices.Equal(log, []string{"cleanup"}) {
		t.Fatalf("unmount: %v", log)
	}
	p.Close()
	if !slices.Equal(log, []string{"cleanup"}) {
		t.Fatal("cleanup repeated")
	}
	assertPanics(t, func() { Effect(func() Cleanup { return nil }) })
}

func TestEffectWriteSettlesBeforePaint(t *testing.T) {
	t.Parallel()
	n := State(0)
	painted := -1
	p := ProbeBuilder(func() Widget {
		Effect(func() Cleanup {
			if n.Get() == 0 {
				n.Set(1)
			}
			return nil
		})
		return View(n, func(v int) Widget {
			return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Size{}) }, func(*Canvas, Rect) { painted = v })
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	if painted != 1 {
		t.Fatalf("painted stale value %d", painted)
	}
}

func TestEffectDisposedBeforeFirstRunAndUntrackedCleanup(t *testing.T) {
	t.Parallel()
	trigger, other := State(0), State(0)
	runs := 0
	p := ProbeBuilder(func() Widget {
		stop := Effect(func() Cleanup { t.Error("disposed effect ran"); return nil })
		stop()
		stop()
		Effect(func() Cleanup { trigger.Get(); runs++; return func() { other.Get() } })
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	trigger.Set(1)
	p.Frame()
	other.Set(1)
	p.Frame()
	if runs != 2 {
		t.Fatalf("cleanup subscribed: %d", runs)
	}
}

func TestIfFactoryLifetimeAndConfiguration(t *testing.T) {
	t.Parallel()
	shown := State(false)
	created, cleaned := 0, 0
	var local *StateValue[int]
	branch := If(shown, func() Widget {
		created++
		local = State(0)
		OnCleanup(func() { cleaned++ })
		return Textf("%d", local)
	})
	p := NewProbe(branch, Sz(100, 30))
	defer p.Close()
	p.Frame()
	if created != 0 {
		t.Fatal("inactive branch constructed")
	}
	shown.Set(true)
	p.Frame()
	local.Set(9)
	p.Frame()
	if created != 1 {
		t.Fatal("branch rebuilt on local write")
	}
	shown.Set(false)
	p.Frame()
	shown.Set(true)
	p.Frame()
	if created != 2 || cleaned != 1 || local.Get() != 0 {
		t.Fatal("branch lifetime incorrect")
	}
	assertPanics(t, func() { branch.Else(func() Widget { return Box() }) })
}

func TestEachPositionKeyIndexAndEmptyLifetime(t *testing.T) {
	t.Parallel()
	type item struct {
		ID    int
		Label string
	}
	source := State([]item{{1, "a"}, {2, "b"}})
	var rows []EachItem[item]
	created, cleaned, empty := 0, 0, 0
	keyed := EachKeyed(source, func(v item) int { return v.ID }, func(row EachItem[item]) Widget {
		rows = append(rows, row)
		created++
		OnCleanup(func() { cleaned++ })
		return Textf("%v", row.Value)
	}).Else(func() Widget { empty++; return Text("empty") })
	p := NewProbe(keyed, Sz(200, 100))
	defer p.Close()
	p.Frame()
	source.Set([]item{{2, "B"}, {1, "A"}})
	p.Frame()
	if created != 2 || rows[0].Index.Get() != 1 || rows[0].Value.Get().Label != "A" {
		t.Fatal("key/index reuse incorrect")
	}
	source.Set(nil)
	p.Frame()
	if cleaned != 2 || empty != 1 {
		t.Fatal("rows not disposed or empty not mounted")
	}
	source.Set(nil)
	p.Frame()
	if empty != 1 {
		t.Fatal("empty branch remounted")
	}
	values := State([]string{"x", "x"})
	var positional []EachItem[string]
	q := NewProbe(Each(values, func(row EachItem[string]) Widget { positional = append(positional, row); return TextOf(row.Value) }), Sz(100, 50))
	defer q.Close()
	q.Frame()
	values.Set([]string{"y"})
	q.Frame()
	if len(positional) != 2 || positional[0].Value.Get() != "y" {
		t.Fatal("positional list not reused")
	}
}

func TestDerivedEqualityAndMeasurementTracking(t *testing.T) {
	t.Parallel()
	n := State(1)
	runs := 0
	p := ProbeBuilder(func() Widget {
		v := Derived(func() []int { return []int{n.Get() % 2} }).WithEqual(slices.Equal[[]int])
		return View(v, func([]int) Widget { runs++; return Box() })
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	n.Set(3)
	p.Frame()
	if runs != 1 {
		t.Fatal("equal derived value propagated")
	}
}

func TestPublicEffectCycleReportsItsOrigin(t *testing.T) {
	t.Parallel()
	n := State(0)
	p := ProbeBuilder(func() Widget {
		Effect(func() Cleanup { n.Set(n.Get() + 1); return nil })
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	defer func() {
		err, ok := recover().(error)
		if !ok || !errors.Is(err, ErrCycle) {
			t.Fatalf("expected ErrCycle, got %v", err)
		}
		if reactive.Debug && !strings.Contains(err.Error(), "reactivity_test.go:") {
			t.Fatalf("missing origin: %v", err)
		}
	}()
	p.Frame()
}

func TestEffectCanCloseAppBeforePaint(t *testing.T) {
	t.Parallel()
	var p *Probe
	painted := false
	cleaned := 0
	p = ProbeBuilder(func() Widget {
		Effect(func() Cleanup { p.Close(); return func() { cleaned++ } })
		return FromFuncs(func(c Constraints, _ Env) Size { return c.Constrain(Size{}) }, func(*Canvas, Rect) { painted = true })
	}, Sz(10, 10))
	p.Frame()
	if painted {
		t.Fatal("closed app painted")
	}
	if cleaned != 1 {
		t.Fatalf("cleanup after close: %d", cleaned)
	}
}

func TestEachEmptyBranchLaidOutAfterExitTransition(t *testing.T) {
	t.Parallel()
	items := State([]int{1})
	cleaned := 0
	p := ProbeBuilder(func() Widget {
		return EachKeyed(items, func(n int) int { return n }, func(EachItem[int]) Widget {
			OnCleanup(func() { cleaned++ })
			return Box().Size(10, 10)
		}).Transition(func(w Widget) *TransitionWidget { return Transition(w).Fade().Duration(time.Millisecond) }).
			Else(func() Widget { return Column(Text("empty"), Text("list")) })
	}, Sz(100, 100))
	defer p.Close()
	p.Frame()
	items.Set(nil)
	p.Frame()
	p.Advance(2 * time.Millisecond)
	if cleaned != 1 {
		t.Fatalf("exit cleanup count=%d", cleaned)
	}
	if _, ok := p.Semantics().Find(RoleText, "empty"); !ok {
		t.Fatal("empty branch not painted")
	}
}

// View rebuilds when its value changes, and not when a signal its builder
// happened to read does.
func TestViewFollowsOnlyItsValue(t *testing.T) {
	value, other := State(0), State(0)
	builds := 0
	p := ProbeBuilder(func() Widget {
		return View(value, func(int) Widget { builds++; other.Get(); return Box() })
	}, Size{W: 100, H: 100})
	defer p.Close()
	p.Frame()
	other.Set(1)
	p.Frame()
	if builds != 1 {
		t.Fatalf("built %d times after another signal changed, want 1", builds)
	}
	value.Set(1)
	p.Frame()
	if builds != 2 {
		t.Fatalf("built %d times after the value changed, want 2", builds)
	}
}

// Peek reads a value without subscribing the computation that reads it.
func TestPeekDoesNotSubscribe(t *testing.T) {
	s := State(1)
	d := Map(s, func(v int) int { return v * 2 })
	runs := 0
	dispose := Root(func() {
		Effect(func() Cleanup {
			runs++
			Peek(s)
			Peek(d)
			return nil
		})
	})
	defer dispose()
	p := NewProbe(Box(), Sz(10, 10))
	defer p.Close()
	p.Frame()
	s.Set(2)
	p.Frame()
	if runs != 1 {
		t.Fatalf("the effect ran %d times after a peeked value changed, want 1", runs)
	}
	if Peek(d) != 4 {
		t.Fatalf("Peek gave %d, want the latest value 4", Peek(d))
	}
}

// Not and Or follow the flags they combine.
func TestBooleanCombinators(t *testing.T) {
	a, b := State(false), State(true)
	not, or := Not(a), Or(a, b)
	check := func(wantNot, wantOr bool) {
		t.Helper()
		if Peek(not) != wantNot || Peek(or) != wantOr {
			t.Fatalf("Not %v Or %v; want %v %v", Peek(not), Peek(or), wantNot, wantOr)
		}
	}
	check(true, true)
	a.Set(true)
	check(false, true)
	a.Set(false)
	b.Set(false)
	check(true, false)
	if Peek(Or()) {
		t.Fatal("Or of nothing should be false")
	}
}

// Responsive lays out the arrangement its width calls for, and the two may
// share a widget.
func TestResponsivePicksAnArrangementByWidth(t *testing.T) {
	shared := Box().Size(40, 20)
	wide := Row(shared, Box().Size(40, 20))
	narrow := Column(shared, Box().Size(40, 20))
	r := Responsive(100, wide, narrow)
	if got := r.Layout(Loose(Sz(200, 200)), Env{}); got != Sz(80, 20) {
		t.Fatalf("wide: %v, want the row's 80x20", got)
	}
	if got := r.Layout(Loose(Sz(90, 200)), Env{}); got != Sz(40, 40) {
		t.Fatalf("narrow: %v, want the column's 40x40", got)
	}
}

// Stretch is Align(AlignStretch).
func TestStretchIsAlignStretch(t *testing.T) {
	if Column().Stretch().align != AlignStretch || Row().Stretch().align != AlignStretch {
		t.Fatal("Stretch did not set AlignStretch")
	}
}

type resourceReply struct {
	value int
	err   error
}
type resourceRequest struct {
	input int
	ctx   context.Context
	reply chan resourceReply
}

func receiveRequest(t *testing.T, requests <-chan resourceRequest) resourceRequest {
	t.Helper()
	select {
	case r := <-requests:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("resource did not start")
		return resourceRequest{}
	}
}
func waitResourcePost(t *testing.T, p *Probe) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p.postMu.Lock()
		n := len(p.posted)
		p.postMu.Unlock()
		if n > 0 {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("resource did not post completion")
}

func TestResourceCancellationLateResultsReloadAndErrors(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 10)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			result := <-req.reply // Deliberately ignore cancellation.
			return result.value, result.err
		})
		return Await(r).Pending(func() Widget { return Text("pending") }).Then(func(v Readable[int]) Widget { return Textf("value %d", v) }).Catch(func(e Readable[error]) Widget { return Textf("error %v", e) })
	}, Sz(200, 30))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	input.Set(2)
	p.Frame()
	b := receiveRequest(t, requests)
	if a.ctx.Err() != context.Canceled || b.input != 2 {
		t.Fatal("input change did not cancel/restart")
	}
	b.reply <- resourceReply{value: 0}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Status != Ready || r.Get().Value != 0 {
		t.Fatal("zero value was not a success")
	}
	id := r.Get().RequestID
	a.reply <- resourceReply{value: 99}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().RequestID != id || r.Get().Value != 0 {
		t.Fatal("old completion overwrote latest")
	}
	r.Reload()
	p.Frame()
	c := receiveRequest(t, requests)
	if c.input != 2 || r.Get().RequestID == id {
		t.Fatal("reload did not start a new request")
	}
	c.reply <- resourceReply{err: errors.New("failed")}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Status != Failed || r.Get().Err.Error() != "failed" {
		t.Fatal("error was not published")
	}
	r.Reload()
	p.Frame()
	d := receiveRequest(t, requests)
	p.Close()
	if d.ctx.Err() != context.Canceled {
		t.Fatal("dispose did not cancel")
	}
	// Completion after Close is dropped by the dispatcher; join the worker
	// through an independent channel in the disposal-specific test below.
	d.reply <- resourceReply{value: 7}
}

func TestResourceInputChangedBeforePostedCompletion(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 2)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			v := <-req.reply
			return v.value, v.err
		})
		return Box()
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	a.reply <- resourceReply{value: 1}
	waitResourcePost(t, p)
	input.Set(2)
	p.Frame()
	if r.Get().Status != Pending || r.Get().RequestID != 2 {
		t.Fatal("stale posted value committed")
	}
	b := receiveRequest(t, requests)
	b.reply <- resourceReply{value: 2}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Value != 2 {
		t.Fatal("new result lost")
	}
}

func TestAwaitRequestIdentityAndReactiveValue(t *testing.T) {
	t.Parallel()
	source := State(AsyncState[int]{RequestID: 1, Status: Ready, Value: 1})
	builds, cleanups := 0, 0
	var value Readable[int]
	view := Await(source).Then(func(v Readable[int]) Widget {
		builds++
		value = v
		OnCleanup(func() { cleanups++ })
		return Textf("%d", v)
	})
	p := NewProbe(view, Sz(100, 30))
	defer p.Close()
	p.Frame()
	source.Set(AsyncState[int]{RequestID: 1, Status: Ready, Value: 2})
	p.Frame()
	if builds != 1 || value.Get() != 2 {
		t.Fatal("same request rebuilt or value froze")
	}
	source.Set(AsyncState[int]{RequestID: 2, Status: Ready, Value: 3})
	p.Frame()
	if builds != 2 || cleanups != 1 {
		t.Fatal("fast request failed to remount")
	}
	assertPanics(t, func() { view.Pending(func() Widget { return Box() }) })
}

func TestResourceEqualityAndSharedAwait(t *testing.T) {
	t.Parallel()
	input := State(1)
	requests := make(chan resourceRequest, 3)
	show := State(true)
	var r *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		r = Resource(input.Get, func(ctx context.Context, n int) (int, error) {
			req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
			requests <- req
			v := <-req.reply
			return v.value, v.err
		}, ResourceEqual(func(a, b int) bool { return a%2 == b%2 }))
		return Column(If(show, func() Widget { return Await(r) }), Await(r))
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	a := receiveRequest(t, requests)
	input.Set(3)
	show.Set(false)
	p.Frame()
	if a.ctx.Err() != nil || r.Get().RequestID != 1 {
		t.Fatal("equal input or consumer exit canceled shared resource")
	}
	a.reply <- resourceReply{value: 8}
	waitResourcePost(t, p)
	p.Frame()
	if r.Get().Value != 8 || len(requests) != 0 {
		t.Fatal("shared resource duplicated work")
	}
}

func TestResourceDisposedComponentIgnoresCompletionWhileAppLives(t *testing.T) {
	t.Parallel()
	shown := State(true)
	requests := make(chan resourceRequest, 1)
	var resource *ResourceValue[int]
	p := ProbeBuilder(func() Widget {
		return If(shown, func() Widget {
			resource = Resource(func() int { return 1 }, func(ctx context.Context, n int) (int, error) {
				req := resourceRequest{n, ctx, make(chan resourceReply, 1)}
				requests <- req
				result := <-req.reply
				return result.value, result.err
			})
			return Await(resource)
		})
	}, Sz(10, 10))
	defer p.Close()
	p.Frame()
	request := receiveRequest(t, requests)
	shown.Set(false)
	p.Frame()
	if request.ctx.Err() != context.Canceled {
		t.Fatal("component exit did not cancel")
	}
	before := resource.Get()
	request.reply <- resourceReply{value: 99}
	waitResourcePost(t, p)
	p.Frame()
	if resource.Get() != before {
		t.Fatal("disposed resource accepted a result")
	}
}
