package ui_test

import (
	"runtime"
	"testing"
	"time"
	"weak"

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

// A tab is built at its first layout, but what it builds belongs to the
// builder that made it, and goes when that builds again: a bar built anew
// at each change, as one whose tabs' titles change is, lets its old tabs
// go. Under the layout's owner, which lasts as long as the window, every
// build kept the one before it alive.
func TestEditorTabsABuilderMadeGoWhenItBuildsAgain(t *testing.T) {
	builds := ggui.State(0)
	var first weak.Pointer[ui.EditorTabWidget]
	p := ggui.NewProbe(ggui.Reactive(func() ggui.Widget {
		tab := ui.EditorTab("a.go", func() {}).Dirty(true).Selected(true).Status(widget.Box().Size(7, 7))
		if builds.Get() == 0 {
			first = weak.Make(tab)
		}
		return tab
	}), ggui.Sz(300, 40))
	defer p.Close()
	p.Frame()
	for n := 1; n <= 3; n++ {
		builds.Set(n)
		p.Frame()
	}
	runtime.GC()
	runtime.GC()
	if first.Value() != nil {
		t.Fatal("the tab of the first build is still alive three builds later")
	}
}

// A tab with a close button goes too, once the Envs its build derived age
// out of the memo that keeps them, values and all, for a few hundred
// builds: a bar rebuilt for hours keeps that many at most.
func TestEditorTabsWithACloseButtonGoInTime(t *testing.T) {
	builds := ggui.State(0)
	var first weak.Pointer[ui.EditorTabWidget]
	p := ggui.NewProbe(ggui.Reactive(func() ggui.Widget {
		tab := ui.EditorTab("a.go", func() {}).OnClose(func() {})
		if builds.Get() == 0 {
			first = weak.Make(tab)
		}
		return tab
	}), ggui.Sz(300, 40))
	defer p.Close()
	p.Frame()
	for n := 1; n <= 1500; n++ {
		builds.Set(n)
		p.Frame()
	}
	runtime.GC()
	runtime.GC()
	if first.Value() != nil {
		t.Fatal("the tab of the first build is still alive 1500 builds later")
	}
}

// A tab kept from an earlier build, as a bar paints a closed tab while it
// shrinks away, still lays out and paints after its builder built again:
// what it built was let go, not taken from it.
func TestEditorTabKeptFromAnEarlierBuildStillPaints(t *testing.T) {
	builds := ggui.State(0)
	var kept ggui.Widget
	p := ggui.NewProbe(ggui.Reactive(func() ggui.Widget {
		if builds.Get() == 0 {
			kept = ui.EditorTab("leaving.go", func() {}).OnClose(func() {})
		}
		return widget.Row(kept, ui.EditorTab("staying.go", func() {}))
	}), ggui.Sz(400, 40))
	defer p.Close()
	p.Frame()
	before, ok := p.Find("leaving.go")
	if !ok {
		t.Fatal("no leaving.go")
	}
	builds.Set(1)
	p.Frame()
	after, ok := p.Find("leaving.go")
	if !ok {
		t.Fatal("the tab kept from the first build is gone after the second")
	}
	if after.Rect != before.Rect {
		t.Fatalf("the kept tab moved from %v to %v", before.Rect, after.Rect)
	}
}
