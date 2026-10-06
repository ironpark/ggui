package main

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

// TestTodo drives the app headlessly. Tap finds controls by their
// accessible names, which the rows derive from their titles.
func TestTodo(t *testing.T) {
	t.Parallel()
	// The model's derived values need an owner, as in main: a Root that
	// the probe disposes with itself.
	var m *model
	dispose := ggui.Root(func() { m = newModel() })
	p := ggui.ProbeBuilder(func() ggui.Widget { return build(m) }, ggui.Sz(520, 600))
	p.Setup(func() { ggui.OnCleanup(dispose) })
	defer p.Close()
	shortcuts(p, m)

	if _, ok := p.Semantics().Find(ggui.RoleText, "Nothing here"); !ok {
		t.Fatal("empty list should show the Empty state")
	}
	m.Draft.Set("Buy milk")
	p.Tap("Add")
	m.Draft.Set("Walk the dog")
	p.Tap("Add")
	if got := len(ggui.Peek(m.Todos)); got != 2 {
		t.Fatalf("todos after two adds = %d, want 2", got)
	}
	if ggui.Peek(m.Draft) != "" {
		t.Fatal("draft was not cleared after adding")
	}

	p.Advance(time.Second) // the new rows animate in
	p.Tap("Done: Buy milk")
	if !ggui.Peek(m.Todos)[0].Done.Get() {
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
	ggui.Peek(m.Todos)[0].Title.Set("Buy oat milk")
	if _, ok := p.Find("Edit: Buy oat milk"); !ok {
		t.Fatal("the editor's name did not follow the title")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if _, ok := p.Find("Edit: Buy oat milk"); ok {
		t.Fatal("Enter did not close the editor")
	}
	if _, ok := p.Find("Done: Buy oat milk"); !ok {
		t.Fatal("the checkbox's name did not follow the edited title")
	}

	// Cmd+K asks before clearing; the dialog's Remove button confirms.
	p.Key("cmd+k")
	if !ggui.Peek(m.Confirm) {
		t.Fatal("shortcut did not open the confirmation")
	}
	p.Tap("Remove")
	if got := len(ggui.Peek(m.Todos)); got != 1 || ggui.Peek(m.Confirm) {
		t.Fatalf("after clearing: %d todos, confirm=%v", got, ggui.Peek(m.Confirm))
	}
	p.Tap("Remove Walk the dog")
	if got := len(ggui.Peek(m.Todos)); got != 0 {
		t.Fatalf("todos after removing the last row = %d, want 0", got)
	}
}
