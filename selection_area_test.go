package ggui

import "testing"

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
