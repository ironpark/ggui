package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

// TestTodo drives the app headlessly. Tap finds controls by their
// accessible names, which the rows derive from their titles.
func TestTodo(t *testing.T) {
	m := newModel()
	p := ggui.ProbeBuilder(m.build, ggui.Sz(520, 600))
	defer p.Close()
	m.shortcuts(p)

	if _, ok := p.Semantics().Find(ggui.RoleText, "Nothing here"); !ok {
		t.Fatal("empty list should show the Empty state")
	}
	m.Draft.Set("Buy milk")
	p.Tap("Add")
	m.Draft.Set("Walk the dog")
	p.Tap("Add")
	if got := len(ggui.Untrack(m.Todos.Get)); got != 2 {
		t.Fatalf("todos after two adds = %d, want 2", got)
	}
	if ggui.Untrack(m.Draft.Get) != "" {
		t.Fatal("draft was not cleared after adding")
	}

	p.Tap("Done: Buy milk")
	if !ggui.Untrack(m.Todos.Get)[0].Done.Get() {
		t.Fatal("checkbox did not mark the first item done")
	}
	p.Tap("Active")
	if _, ok := p.Find("Done: Buy milk"); ok {
		t.Fatal("finished item still listed under Active")
	}
	p.Tap("All")

	// Inline editing: a tap on the title swaps in a field bound to it.
	milk, ok := p.Find("Done: Buy milk")
	if !ok {
		t.Fatal("first row missing")
	}
	p.Click(ggui.Pt(milk.Rect.Origin.X+milk.Rect.Size.W+60, milk.Center().Y))
	if _, ok := p.Find("Edit: Buy milk"); !ok {
		t.Fatal("tapping the title did not open the editor")
	}
	p.Tap("Edit: Buy milk") // focus the field
	ggui.Untrack(m.Todos.Get)[0].Title.Set("Buy oat milk")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if _, ok := p.Find("Edit: Buy milk"); ok {
		t.Fatal("Enter did not close the editor")
	}
	if _, ok := p.Find("Done: Buy oat milk"); !ok {
		t.Fatal("the checkbox's name did not follow the edited title")
	}

	// Cmd+K asks before clearing; the dialog's Remove button confirms.
	p.Type(ggui.Mods{Meta: true}, ggui.KeyK)
	if !ggui.Untrack(m.Confirm.Get) {
		t.Fatal("shortcut did not open the confirmation")
	}
	p.Tap("Remove")
	if got := len(ggui.Untrack(m.Todos.Get)); got != 1 || ggui.Untrack(m.Confirm.Get) {
		t.Fatalf("after clearing: %d todos, confirm=%v", got, ggui.Untrack(m.Confirm.Get))
	}
	p.Tap("Remove Walk the dog")
	if got := len(ggui.Untrack(m.Todos.Get)); got != 0 {
		t.Fatalf("todos after removing the last row = %d, want 0", got)
	}
}
