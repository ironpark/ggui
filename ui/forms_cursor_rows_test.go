package ui_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// The rows of a list box and the steppers of a number input keep the arrow
// too: shadcn's ComboboxItem and CommandItem are cursor-default, and a
// stepper is a Button, which sets no cursor.
func TestListRowsAndSteppersKeepTheDefaultCursor(t *testing.T) {
	t.Parallel()
	files := []string{"main.go", "go.mod"}
	list := ui.ListBox(ggui.Const(files), func(f string) string { return f },
		func(f ggui.Readable[string]) ggui.Widget { return ggui.TextOf(f) }).Name("Files")
	number := ui.NumberInput(ggui.State(3.0)).Name("Count")
	p := ggui.NewProbe(ggui.Column(list, number).Gap(8).Align(ggui.AlignStart), ggui.Sz(320, 200))
	defer p.Close()
	row, ok := p.FindRole(ggui.RoleOption, "main.go")
	if !ok {
		t.Fatal("the list has no main.go row")
	}
	p.Move(row.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("cursor = %v over a list box row, want default", got)
	}
	field, ok := p.Find("Count")
	if !ok {
		t.Fatal("no number input named Count")
	}
	seen := map[ggui.CursorShape]bool{}
	for x := field.Rect.Origin.X + 1; x < field.Rect.Origin.X+field.Rect.Size.W; x += 2 {
		p.Move(ggui.Pt(x, field.Rect.Center().Y))
		seen[p.Cursor()] = true
	}
	if !seen[ggui.CursorShapeText] || !seen[ggui.CursorShapeDefault] || len(seen) != 2 {
		t.Errorf("cursors across a number input = %v, want the text beam over the editor and the arrow over the steppers", seen)
	}
}

// A button among the addons of an input group shows the arrow, not the
// beam of the field it sits in: a button's cursor is its own.
func TestAddonButtonKeepsTheArrowInsideAField(t *testing.T) {
	t.Parallel()
	group := ui.InputGroup(ggui.TextInput(ggui.State("")).Name("Site")).Trailing(ui.Button("Go", nil))
	p := ggui.NewProbe(ggui.Column(group).Align(ggui.AlignStart), ggui.Sz(320, 120))
	defer p.Close()
	button, ok := p.FindRole(ggui.RoleButton, "Go")
	if !ok {
		t.Fatal("no Go button")
	}
	p.Move(button.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("cursor = %v over a button in an input group, want default", got)
	}
	p.Move(ggui.Pt(20, button.Rect.Center().Y))
	if got := p.Cursor(); got != ggui.CursorShapeText {
		t.Errorf("cursor = %v over the editor, want the text beam", got)
	}
}
