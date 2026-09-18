package ui_test

import (
	"fmt"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestAccordionSelectionAndKeyboard(t *testing.T) {
	open := ggui.State([]string{})
	calls := 0
	a := ui.Accordion(open,
		ui.AccordionItem("a", "Alpha", ui.Button("Inside Alpha", func() { calls++ })),
		ui.AccordionItem("b", "Disabled", ggui.Text("Hidden")).Disabled(true),
		ui.AccordionItem("c", "Charlie", ggui.Text("Content")),
	)
	p := ggui.NewProbe(a, ggui.Sz(300, 300))
	defer p.Close()
	p.Tap("Alpha")
	if !slices.Equal(ggui.Untrack(open.Get), []string{"a"}) {
		t.Fatal(ggui.Untrack(open.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if !slices.Equal(ggui.Untrack(open.Get), []string{"c"}) {
		t.Fatalf("single mode/disabled skip: %v", ggui.Untrack(open.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeySpace)
	if !slices.Equal(ggui.Untrack(open.Get), []string{"a"}) {
		t.Fatal("Home failed", ggui.Untrack(open.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyEnter)
	if calls != 1 {
		t.Fatal("open content is not keyboard reachable")
	}
	p.Tap("Alpha")
	if len(ggui.Untrack(open.Get)) != 0 {
		t.Fatal("second click did not collapse")
	}
	a.Multiple()
	p.Tap("Alpha")
	p.Tap("Charlie")
	if !slices.Equal(ggui.Untrack(open.Get), []string{"a", "c"}) {
		t.Fatal("multiple mode", ggui.Untrack(open.Get))
	}
	p.Tap("Alpha")
	if !slices.Equal(ggui.Untrack(open.Get), []string{"c"}) {
		t.Fatal("independent collapse", ggui.Untrack(open.Get))
	}
	open.Set([]string{"a"})
	p.Frame()
	find(t, p, ggui.RoleButton, "Inside Alpha")
}

func TestCommandFiltersAndSkipsDisabled(t *testing.T) {
	query := ggui.State("")
	picked := ""
	cmd := ui.Command(query,
		ui.CommandItem("Unavailable", func() { t.Fatal("disabled action ran") }).Disabled(true),
		ui.CommandItem("Open", func() { picked = "open" }).Keywords("file", "문서"),
		ui.CommandItem("Save", func() { picked = "save" }),
	)
	p := ggui.NewProbe(ggui.Column(cmd), ggui.Sz(300, 260))
	defer p.Close()
	p.Tap("Search commands")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "open" {
		t.Fatal("initial enabled match", picked)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if picked != "save" {
		t.Fatal("arrow navigation", picked)
	}
	query.Set("  FILE ")
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "open" {
		t.Fatal("keyword matching failed")
	}
	if _, ok := p.Find("Save"); ok {
		t.Fatal("filtered item remains interactive")
	}
	query.Set("문서")
	p.Frame()
	find(t, p, ggui.RoleMenuItem, "Open")
	query.Set("no match")
	picked = ""
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if picked != "" {
		t.Fatal("empty results ran an action")
	}
	query.Set("Save")
	p.Frame()
	p.Tap("Save")
	if picked != "save" {
		t.Fatal("pointer selection failed")
	}
}

func TestComboboxSelectionAndEscape(t *testing.T) {
	value := ggui.State("Apple")
	changes := 0
	c := ui.Combobox(value, []string{"Apple", "Banana", "Cherry"}).Named("Fruit").OnChange(func(string) { changes++ })
	p := ggui.NewProbe(ggui.Column(c, ui.Button("Outside", func() {})), ggui.Sz(360, 320))
	defer p.Close()
	p.Tap("Fruit")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyEnter)
	if ggui.Untrack(value.Get) != "Banana" || changes != 1 || c.Popup().IsOpen() {
		t.Fatalf("selection: %s, %d, %v", ggui.Untrack(value.Get), changes, c.Popup().IsOpen())
	}
	// Closing restores focus to the trigger, so Enter opens it again.
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if !c.Popup().IsOpen() {
		t.Fatal("focus did not return to the trigger")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if ggui.Untrack(value.Get) != "Banana" || c.Popup().IsOpen() {
		t.Fatal("Escape changed selection or did not close")
	}
	p.Tap("Fruit")
	p.Tap("Cherry")
	if ggui.Untrack(value.Get) != "Cherry" || changes != 2 {
		t.Fatal("pointer selection failed")
	}
	c.Disabled(true)
	p.Frame()
	if _, ok := p.Find("Fruit"); ok {
		t.Fatal("disabled trigger remains interactive")
	}
}

func TestResizableCaptureLimitsAndKeyboard(t *testing.T) {
	fraction := ggui.State(.5)
	r := ui.Resizable(fraction, ggui.Box(), ggui.Box()).WithHandle().MinSizes(40, 60)
	p := ggui.NewProbe(r, ggui.Sz(208, 100))
	defer p.Close()
	at := find(t, p, ggui.RoleSeparator, "Resize panels").Center()
	p.Press(at)
	p.Move(ggui.Pt(300.0, at.Y))
	p.Move(ggui.Pt(400.0, at.Y))
	p.Release(ggui.Pt(400.0, at.Y))
	if math.Abs(ggui.Untrack(fraction.Get)-.7) > 1e-9 {
		t.Fatal("drag did not clamp to second minimum", ggui.Untrack(fraction.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if math.Abs(ggui.Untrack(fraction.Get)-.2) > 1e-9 {
		t.Fatal("keyboard focus lost while handle moved", ggui.Untrack(fraction.Get))
	}
	p.Type(ggui.Mods{Shift: true}, ggui.KeyArrowRight)
	if math.Abs(ggui.Untrack(fraction.Get)-.3) > 1e-9 {
		t.Fatal("keyboard step failed", ggui.Untrack(fraction.Get))
	}
	fraction.Set(.9)
	p.Frame()
	rect := find(t, p, ggui.RoleSeparator, "Resize panels").Rect
	if math.Abs(rect.Origin.X-140) > 1e-9 || ggui.Untrack(fraction.Get) != .9 {
		t.Fatal("external value not clamped for display", rect, ggui.Untrack(fraction.Get))
	}
	p.Resize(ggui.Sz(58, 40))
	p.Frame()
	rect = find(t, p, ggui.RoleSeparator, "Resize panels").Rect
	if math.Abs(rect.Origin.X-20) > 1e-9 {
		t.Fatal("overconstrained minima are not proportional", rect)
	}
	r.Disabled(true)
	p.Frame()
	if _, ok := p.Find("Resize panels"); ok {
		t.Fatal("disabled splitter takes input")
	}
}

func TestResizableVertical(t *testing.T) {
	fraction := ggui.State(.5)
	p := ggui.NewProbe(ui.Resizable(fraction, ggui.Box(), ggui.Box()).Vertical().WithHandle(), ggui.Sz(100, 208))
	defer p.Close()
	p.Tap("Resize panels")
	p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	if math.Abs(ggui.Untrack(fraction.Get)-.51) > 1e-9 {
		t.Fatal(ggui.Untrack(fraction.Get))
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if ggui.Untrack(fraction.Get) != 1 {
		t.Fatal("vertical End failed")
	}
}

func TestToasterLifecycleAndNonmodalActions(t *testing.T) {
	toaster := ui.NewToaster().Limit(2)
	clicks := 0
	p := ggui.ProbeBuilder(func() ggui.Widget { return ggui.Column(ui.Button("Background", func() { clicks++ }), toaster) }, ggui.Sz(500, 500))
	p.Setup(func() { ggui.OnCleanup(toaster.Close) })
	defer p.Close()
	p.Advance(0)
	p.Tap("Background")
	toaster.Push(ui.Toast("Saved", "Changes stored").Duration(time.Second))
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if clicks != 2 {
		t.Fatal("toast stole keyboard focus")
	}
	p.Advance(2 * time.Second)
	if toaster.Len() != 0 {
		t.Fatal("toast did not expire")
	}
	first := toaster.Push(ui.Toast("One", "Persistent").Duration(0))
	toaster.Push(ui.Toast("Two", "Persistent").Duration(0))
	toaster.Push(ui.Toast("Three", "Undo available").Duration(0).Action("Undo", func() { clicks += 10 }))
	if toaster.Len() != 2 {
		t.Fatal("queue was not bounded")
	}
	toaster.Dismiss(first)
	if toaster.Len() != 2 {
		t.Fatal("unknown ID dismissed another toast")
	}
	p.Tap("Undo")
	if clicks != 12 || toaster.Len() != 1 {
		t.Fatal("action did not dismiss and run")
	}
	p.Tap("Dismiss Two")
	if toaster.Len() != 0 {
		t.Fatal("manual dismiss failed")
	}
	toaster.Push(ui.Toast("Close", "pending").Duration(0))
	p.Close()
	if toaster.Len() != 0 || toaster.Push(ui.Toast("Late", "")) != 0 {
		t.Fatal("owner cleanup did not close queue")
	}
}

func TestToasterPausesWhileInteracting(t *testing.T) {
	toaster := ui.NewToaster()
	p := ggui.NewProbe(ggui.Column(toaster), ggui.Sz(500, 400))
	defer p.Close()
	p.Advance(0)
	toaster.Push(ui.Toast("Pause", "Hover the dismiss button").Duration(time.Second))
	p.Move(find(t, p, ggui.RoleButton, "Dismiss Pause").Center())
	p.Advance(2 * time.Second)
	if toaster.Len() != 1 {
		t.Fatal("toast expired while hovered")
	}
	p.Move(ggui.Pt(0, 0))
	p.Advance(2 * time.Second)
	if toaster.Len() != 0 {
		t.Fatal("toast did not expire after pointer left")
	}
}

func TestCommandScrollsKeyboardHighlight(t *testing.T) {
	query := ggui.State("")
	picked := -1
	entries := make([]ui.CommandEntry, 20)
	for i := range entries {
		entries[i] = ui.CommandItem(fmt.Sprintf("Action %d", i), func() { picked = i })
	}
	p := ggui.NewProbe(ggui.Column(ui.Command(query, entries...).Height(60)), ggui.Sz(300, 160))
	defer p.Close()
	p.Tap("Search commands")
	for range 19 {
		p.Type(ggui.Mods{}, ggui.KeyArrowDown)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != 19 {
		t.Fatal("last command was not selected", picked)
	}
	r := find(t, p, ggui.RoleMenuItem, "Action 19").Rect
	if r.Empty() {
		t.Fatal("highlight was not scrolled into view")
	}
	query.Set("Action 0")
	p.Frame()
	if find(t, p, ggui.RoleMenuItem, "Action 0").Rect.Empty() {
		t.Fatal("filter did not reset scroll")
	}
}

func TestAdvancedBindingsInvalidateCachedLayout(t *testing.T) {
	open := ggui.State([]string{})
	fraction := ggui.State(.5)
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return ggui.Column(
			ggui.Cached(ui.Accordion(open, ui.AccordionItem("a", "Header", ui.Button("Content", func() {})))),
			ggui.Box(ggui.Cached(ui.Resizable(fraction, ggui.Box(), ggui.Box()))).Height(100),
		)
	}, ggui.Sz(208, 300))
	defer p.Close()
	p.Frame()
	open.Set([]string{"a"})
	fraction.Set(.75)
	p.Frame()
	p.Frame()
	find(t, p, ggui.RoleButton, "Content")
	r := find(t, p, ggui.RoleSeparator, "Resize panels").Rect
	if math.Abs(r.Origin.X-150) > 1e-9 {
		t.Fatal("cached splitter did not follow binding", r)
	}
}

func TestResizableCapturesTouchDrag(t *testing.T) {
	r := ui.Resizable(ggui.State(.5), ggui.Box(), ggui.Box()).Vertical()
	var capturer ggui.TouchDragCapturer = r
	if !capturer.CaptureTouchDrag() {
		t.Fatal("resize handle allows parent touch scrolling")
	}
	r.Disabled(true)
	if capturer.CaptureTouchDrag() {
		t.Fatal("disabled resize handle captures touch dragging")
	}
}
