package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// textArea is a field that fills the window and asks for the I-beam all
// over it, for a panel to float over.
func textArea() ggui.Widget {
	return ggui.Pointer(ggui.Box().Size(400, 300)).Cursor(ggui.CursorShapeText)
}

// A menu's rows and the panel around them show the arrow over a text field
// they cover, and so does the window around a modal menu.
func TestAMenuShowsTheArrowOverATextField(t *testing.T) {
	t.Parallel()
	m := ui.Menu("File", ui.MenuItem("Open", nil), ui.MenuItem("Save", nil))
	p := ggui.NewProbe(ggui.Stack(textArea(), ggui.Padding(m, 40, 0, 0, 40)), ggui.Sz(400, 300))
	defer p.Close()
	p.Move(ggui.Pt(200, 200))
	if got := p.Cursor(); got != ggui.CursorShapeText {
		t.Fatalf("over the field the cursor is %v, want text", got)
	}
	p.Tap("File")
	p.Advance(time.Second)
	row := find(t, p, ggui.RoleMenuItem, "Save")
	p.Move(row.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("over a menu row the cursor is %v, want the arrow", got)
	}
	panel := m.Popup().Rect()
	p.Move(panel.Origin.Add(ggui.Pt(1, 1)))
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("over the menu's padding the cursor is %v, want the arrow", got)
	}
	p.Move(ggui.Pt(350, 280))
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("beside an open modal menu the cursor is %v, want the arrow", got)
	}
}

// A dialog's panel and scrim show the arrow over a text field beneath, and
// a field inside the dialog still shows its I-beam.
func TestADialogShowsTheArrowOverATextField(t *testing.T) {
	t.Parallel()
	open := ggui.State(true)
	d := ui.Dialog(open, ggui.Column(ggui.Text("Rename"), ui.TextField(ggui.State("")).Placeholder("name")))
	p := ggui.NewProbe(ggui.Stack(textArea(), d), ggui.Sz(400, 300))
	defer p.Close()
	p.Advance(time.Second)
	title, ok := p.Semantics().Find(ggui.RoleText, "Rename")
	if !ok {
		t.Fatal("the dialog did not open")
	}
	p.Move(title.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("over the dialog's panel the cursor is %v, want the arrow", got)
	}
	p.Move(ggui.Pt(3, 3))
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Errorf("over the dialog's scrim the cursor is %v, want the arrow", got)
	}
	field := find(t, p, ggui.RoleTextField, "name")
	p.Move(field.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeText {
		t.Errorf("over a field in the dialog the cursor is %v, want text", got)
	}
}

// A context menu registers its region over its content, to take the
// secondary click first, and asks for no cursor there: the field it wraps
// keeps its I-beam.
func TestAContextMenuLeavesItsContentTheCursor(t *testing.T) {
	t.Parallel()
	c := ui.ContextMenu(ui.TextField(ggui.State("")).Placeholder("name"), ui.MenuItem("Paste", nil))
	p := ggui.NewProbe(c, ggui.Sz(300, 100))
	defer p.Close()
	field := find(t, p, ggui.RoleTextField, "name")
	p.Move(field.Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeText {
		t.Fatalf("over a field in a context menu the cursor is %v, want text", got)
	}
}

// A button inside a text field's box asks for the arrow, as a browser's
// button does over an input's cursor: text.
func TestAButtonOverATextFieldShowsTheArrow(t *testing.T) {
	t.Parallel()
	b := ui.Button("Clear", nil)
	p := ggui.NewProbe(ggui.Stack(textArea(), ggui.Padding(b, 20, 0, 0, 20)), ggui.Sz(400, 300))
	defer p.Close()
	p.Move(find(t, p, ggui.RoleButton, "Clear").Rect.Center())
	if got := p.Cursor(); got != ggui.CursorShapeDefault {
		t.Fatalf("over a button on a text field the cursor is %v, want the arrow", got)
	}
}
