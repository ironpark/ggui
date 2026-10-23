package ggui

import "testing"

// textAt returns the window point over offset b of a laid-out paragraph
// painted at r, halfway down its line.
func textAt(w *RichTextWidget, r Rect, b int) Point {
	var at Point
	n := len(w.para.text)
	lo, hi := b, min(b+1, n)
	if lo == hi {
		lo = max(b-1, 0)
	}
	w.para.rects(lo, hi, r.Size.W, w.alignment(), func(box Rect) {
		x := box.Origin.X
		if b == n {
			x += box.Size.W
		}
		at = Pt(r.Origin.X+x+0.5, r.Origin.Y+box.Origin.Y+box.Size.H/2)
	})
	return at
}

func TestSelectableTextDoubleClickSelectsAWordAndCopies(t *testing.T) {
	t.Parallel()
	w := Text("hello world").Selectable()
	p := NewProbe(w, Sz(300, 30))
	defer p.Close()
	p.Frame()
	r := Rct(Pt(0, 0), Sz(300, 30))
	at := textAt(w.rich, r, 7) // inside "world"
	p.Click(at)
	p.Click(at)
	if got := w.rich.SelectedText(); got != "world" {
		t.Fatalf("double click selected %q, want world", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "world" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "hello world" {
		t.Fatalf("select all copied %q", got)
	}
	p.Key("escape")
	if lo, hi := w.rich.Selection(); lo != hi {
		t.Fatalf("escape left %d..%d selected", lo, hi)
	}
}

func TestSelectableRichTextDragAndShiftClick(t *testing.T) {
	t.Parallel()
	w := RichText(Span("one "), Span("two").Weight(WeightBold), Span(" three")).Selectable()
	p := NewProbe(w, Sz(400, 30))
	defer p.Close()
	p.Frame()
	r := Rct(Pt(0, 0), Sz(400, 30))
	p.Press(textAt(w, r, 0))
	p.Move(textAt(w, r, 7))
	p.Release(textAt(w, r, 7))
	if got := w.SelectedText(); got != "one two" {
		t.Fatalf("drag selected %q, want across the spans to \"one two\"", got)
	}
	p.ClickWith(textAt(w, r, len("one two three")), Mods{Shift: true})
	if got := w.SelectedText(); got != "one two three" {
		t.Fatalf("shift-click extended to %q", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "one two three" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	if p.Cursor() != CursorShapeText {
		t.Fatalf("cursor over selectable text is %v, want the text cursor", p.Cursor())
	}
}

func TestSelectableTextDescribesItsSelection(t *testing.T) {
	t.Parallel()
	w := RichText(Span("abc def")).Selectable()
	p := NewProbe(w, Sz(300, 30))
	defer p.Close()
	tree := p.Semantics()
	node, ok := tree.Find(RoleText, "abc def")
	if !ok || node.Actions&ActionSetSelection == 0 {
		t.Fatalf("node %+v, want text that takes a selection", node)
	}
	p.Perform(node.ID, Action{Kind: ActionSetSelection, SelStart: 4, SelEnd: 7})
	if got := w.SelectedText(); got != "def" {
		t.Fatalf("ActionSetSelection selected %q", got)
	}
	node, _ = p.Semantics().Find(RoleText, "abc def")
	if node.SelStart != 4 || node.SelEnd != 7 {
		t.Fatalf("node reports %d..%d, want 4..7", node.SelStart, node.SelEnd)
	}
}

func TestSelectableTextKeepsTextLayout(t *testing.T) {
	t.Parallel()
	plain := Text("aaa bbb ccc").MaxLines(1)
	sel := Text("aaa bbb ccc").MaxLines(1).Selectable()
	a := plain.Layout(Loose(Sz(60, 100)), Env{})
	b := sel.Layout(Loose(Sz(60, 100)), Env{})
	if a != b {
		t.Fatalf("selectable text measured %v, plain %v", b, a)
	}
	pb, _ := plain.Baseline()
	sb, _ := sel.Baseline()
	if pb != sb {
		t.Fatalf("baselines %v and %v", pb, sb)
	}
}
