package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

// A tab picks what it stands for, and its close button closes it: the
// button is there, to be found and tapped, whether or not it shows.
func TestEditorTabSelectsAndCloses(t *testing.T) {
	var picked, closed string
	tab := func(name string) ggui.Widget {
		return ui.EditorTab(name, func() { picked = name }).OnClose(func() { closed = name }).CloseName("Shut " + name)
	}
	p := ggui.NewProbe(widget.Row(
		tab("a.go"),
		ui.TabGroup(tab("b.go").(*ui.EditorTabWidget).Dirty(true), ui.EditorTab("c.go", func() { picked = "c.go" }).Preview(true)),
	), ggui.Sz(600, 40))
	defer p.Close()
	p.Frame()
	p.Tap("b.go")
	if picked != "b.go" {
		t.Fatalf("tapping b.go picked %q", picked)
	}
	p.Tap("Shut a.go")
	if closed != "a.go" || picked != "b.go" {
		t.Fatalf("closing a.go closed %q and picked %q", closed, picked)
	}
	if _, ok := p.Find("Close c.go"); ok {
		t.Fatal("a tab with no OnClose has a close button")
	}
	p.Tap("c.go")
	if picked != "c.go" {
		t.Fatalf("tapping the tab in the group picked %q", picked)
	}
}

// A tab is no narrower than its MinWidth, and its label no wider than
// MaxLabelWidth; a Status has its room only while there is one, and while
// StatusWhile says it shows; a close button takes none.
func TestEditorTabWidths(t *testing.T) {
	long := "a label far longer than any tab would draw it whole"
	shown := false
	p := ggui.NewProbe(widget.Column(
		ui.EditorTab("a", func() {}),
		ui.EditorTab(long, func() {}).Name("long"),
		ui.EditorTab(long, func() {}).Name("closing").OnClose(func() {}),
		ui.EditorTab(long, func() {}).Name("unmarked").Status(nil),
		ui.EditorTab(long, func() {}).Name("marked").Status(widget.Box().Size(7, 7)),
		ui.EditorTab(long, func() {}).Name("resting").Status(widget.Box().Size(7, 7)).StatusWhile(func() bool { return shown }),
	), ggui.Sz(600, 300))
	defer p.Close()
	p.Frame()
	width := func(name string) float64 {
		f, ok := p.Find(name)
		if !ok {
			t.Fatalf("no tab %s", name)
		}
		return f.Rect.Size.W
	}
	if w := width("a"); w != ui.EditorTabMinWidth {
		t.Fatalf("a short tab is %v wide, not %v", w, ui.EditorTabMinWidth)
	}
	if w := width("long"); w > 10+170+10 {
		t.Fatalf("a long tab is %v wide, past its label's cap", w)
	}
	if d := width("closing") - width("long"); d != 0 {
		t.Fatalf("a close button takes %v", d)
	}
	if d := width("unmarked") - width("long"); d != 0 {
		t.Fatalf("no status takes %v", d)
	}
	if d := width("marked") - width("long"); d <= 0 {
		t.Fatalf("a status's room adds %v", d)
	}
	if d := width("resting") - width("long"); d != 0 {
		t.Fatalf("a status at rest takes %v", d)
	}
	shown = true
	p.Frame()                        // the paint that sees it come
	p.Advance(10 * time.Millisecond) // the layout it asks for, opening the room
	p.Advance(time.Second)
	if d := width("resting") - width("marked"); d != 0 {
		t.Fatalf("a status come is %v off a status's room", d)
	}
}

// Scrolled past its start, a group's label stays at the start of what
// shows, over the tabs, and takes the press there.
func TestTabGroupLabelSticks(t *testing.T) {
	var picked string
	labels := 0
	var tabs []ggui.Widget
	for _, name := range []string{"one", "two", "three", "four", "five", "six"} {
		tabs = append(tabs, ui.EditorTab(name, func() { picked = name }))
	}
	group := ui.TabGroup(tabs...).Label(widget.Text("group")).OnLabel(func() { labels++ }).LabelName("Group")
	bar := widget.Scroll(widget.Row(widget.Box().Size(60, 10), group)).Horizontal().Offset(260).FadeEnds(0, 28)
	p := ggui.NewProbe(bar, ggui.Sz(300, 40))
	defer p.Close()
	p.Frame()
	label, ok := p.Find("Group")
	if !ok {
		t.Fatal("no label")
	}
	if x := label.Rect.Origin.X; x < 0 || x > 8 {
		t.Fatalf("the label is at %v, not stuck at the start", x)
	}
	p.Tap("Group")
	if labels != 1 || picked != "" {
		t.Fatalf("a tap on the stuck label: %d label taps, picked %q", labels, picked)
	}
}
