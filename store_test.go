package ggui

import (
	"slices"
	"testing"
)

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
