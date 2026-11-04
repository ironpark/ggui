package ggui

import (
	"testing"
)

func TestASelectionRunsThroughTheTextsOfAnArea(t *testing.T) {
	t.Parallel()
	a, b, c := Text("first line").Selectable(), Text("second line").Selectable(), Text("third line").Selectable()
	area := SelectionArea(Column(a, b, c).Gap(10).Stretch())
	p := NewProbe(area, Sz(300, 200))
	defer p.Close()
	p.Frame()
	// From inside the first paragraph to inside the third.
	p.Press(textAt(a.rich, a.rich.rect, 6))
	p.Move(textAt(c.rich, c.rich.rect, 5))
	p.Release(textAt(c.rich, c.rich.rect, 5))
	p.Frame()
	if got, want := area.SelectedText(), "line\nsecond line\nthird"; got != want {
		t.Fatalf("drag across three paragraphs selected %q, want %q", got, want)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "line\nsecond line\nthird" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
	// Back up into the first: the selection runs the other way.
	p.Press(textAt(c.rich, c.rich.rect, 5))
	p.Move(textAt(a.rich, a.rich.rect, 6))
	p.Release(textAt(a.rich, a.rich.rect, 6))
	p.Frame()
	if got, want := area.SelectedText(), "line\nsecond line\nthird"; got != want {
		t.Fatalf("drag up across three paragraphs selected %q, want %q", got, want)
	}
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != "first line\nsecond line\nthird line" {
		t.Fatalf("select all copied %q", got)
	}
	p.Key("escape")
	if got := area.SelectedText(); got != "" {
		t.Fatalf("escape left %q selected", got)
	}
}

func TestADragWithinOneTextOfAnAreaSelectsAsItsOwn(t *testing.T) {
	t.Parallel()
	a, b := Text("hello world").Selectable(), Text("other").Selectable()
	area := SelectionArea(Column(a, b).Gap(10))
	p := NewProbe(area, Sz(300, 100))
	defer p.Close()
	p.Frame()
	p.Press(textAt(a.rich, a.rich.rect, 0))
	p.Move(textAt(a.rich, a.rich.rect, 5))
	p.Release(textAt(a.rich, a.rich.rect, 5))
	if got := area.SelectedText(); got != "hello" {
		t.Fatalf("drag within one paragraph selected %q, want hello", got)
	}
	p.Key("cmd+c")
	if got := p.Clipboard().Read(); got != "hello" {
		t.Fatalf("copy put %q on the clipboard", got)
	}
}

func TestFindSelectsTheMatchesOfAnAreaInTurn(t *testing.T) {
	t.Parallel()
	a, b := Text("Error: the path").Selectable(), Text("another error, and an ERROR").Selectable()
	area := SelectionArea(Column(a, b).Gap(10))
	p := NewProbe(area, Sz(400, 100))
	defer p.Close()
	p.Frame()
	for n, want := range []string{"Error", "error", "ERROR"} {
		at, ok := area.Find("error", 0, n)
		if !ok || area.SelectedText() != want {
			t.Fatalf("match %d: found %v, selected %q; want %q", n, ok, area.SelectedText(), want)
		}
		if at.Size.W <= 0 || (n == 0) != (at.Origin.Y < b.rich.rect.Origin.Y) {
			t.Fatalf("match %d painted at %v", n, at)
		}
	}
	if _, ok := area.Find("error", 0, 3); ok || area.SelectedText() != "ERROR" {
		t.Fatalf("a fourth match was found, or the third let go: %q", area.SelectedText())
	}
	// From below the first text, its match does not count.
	if _, ok := area.Find("error", b.rich.rect.Origin.Y, 0); !ok || area.SelectedText() != "error" {
		t.Fatalf("from the second text, the first match is %q", area.SelectedText())
	}
}

func TestIndexFoldIgnoresCase(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		s, needle string
		from      int
		lo, hi    int
	}{
		{"Hello World", "world", 0, 6, 11},
		{"ÄBC äbc", "äbc", 1, 5, 9},
		{"abc", "x", 0, -1, -1},
		{"abc", "", 0, -1, -1},
	} {
		if lo, hi := IndexFold(c.s, c.needle, c.from); lo != c.lo || hi != c.hi {
			t.Errorf("IndexFold(%q, %q, %d) = %d, %d; want %d, %d", c.s, c.needle, c.from, lo, hi, c.lo, c.hi)
		}
	}
}

// textAt returns the window point over offset b of a laid-out paragraph
// painted at r, halfway down its line.
func textAt(w *RichTextWidget, r Rect, b int) Point {
	var at Point
	n := len(w.para.Text)
	lo, hi := b, min(b+1, n)
	if lo == hi {
		lo = max(b-1, 0)
	}
	w.para.Rects(lo, hi, r.Size.W, w.alignment(), func(box Rect) {
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
