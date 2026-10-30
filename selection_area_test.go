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
