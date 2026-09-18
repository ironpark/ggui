package ui_test

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
	"time"
)

func date(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, time.UTC) }
func TestCalendarNavigationAndConstraints(t *testing.T) {
	value := ggui.State(date(2024, 1, 31))
	changes := 0
	c := ui.Calendar(value).Location(time.UTC).WeekStartsOn(time.Monday).OnChange(func(time.Time) { changes++ })
	p := ggui.NewProbe(c, ggui.Sz(300, 350))
	defer p.Close()
	p.Tap("2024-01-31")
	p.Type(ggui.Mods{}, ggui.KeyPageDown, ggui.KeyEnter)
	if !value.Peek().Equal(date(2024, 2, 29)) || changes != 1 {
		t.Fatal("leap month navigation", value.Peek(), changes)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyEnter)
	if !value.Peek().Equal(date(2024, 3, 1)) {
		t.Fatal("month rollover", value.Peek())
	}
	c.Bounds(date(2024, 3, 1), date(2024, 3, 10)).DisabledDate(func(d time.Time) bool { return d.Weekday() == time.Saturday })
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyEnter)
	if !value.Peek().Equal(date(2024, 3, 1)) {
		t.Fatal("disabled date selected")
	}
	act(t, p, ggui.RoleOption, "2024-03-03", ggui.Action{Kind: ggui.ActionSelect})
	if !value.Peek().Equal(date(2024, 3, 3)) {
		t.Fatal("accessible selection")
	}
	act(t, p, ggui.RoleOption, "2024-03-11", ggui.Action{Kind: ggui.ActionSelect})
	if !value.Peek().Equal(date(2024, 3, 3)) {
		t.Fatal("out of bounds selection")
	}
	value.Set(date(2025, 12, 15))
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleOption, "2025-12-15"); !ok {
		t.Fatal("external binding did not change month")
	}
	c.Disabled(true)
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyEnter)
	if !value.Peek().Equal(date(2025, 12, 15)) {
		t.Fatal("disabled calendar")
	}
}
func TestDatePickerSelectionAndCancel(t *testing.T) {
	v := ggui.State(date(2024, 2, 1))
	n := 0
	d := ui.DatePicker(v).Named("Due date").OnChange(func(time.Time) { n++ })
	d.Calendar().Location(time.UTC)
	p := ggui.NewProbe(ggui.Column(d), ggui.Sz(380, 450))
	defer p.Close()
	p.Tap("Due date")
	p.Tap("2024-02-29")
	if d.Popup().IsOpen() || !v.Peek().Equal(date(2024, 2, 29)) || n != 1 {
		t.Fatal("selection", v.Peek(), n)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if !d.Popup().IsOpen() {
		t.Fatal("focus return")
	}
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if d.Popup().IsOpen() || n != 1 {
		t.Fatal("cancel")
	}
	p.Tap("Due date")
	p.Tap("2024-02-29")
	if d.Popup().IsOpen() || n != 1 {
		t.Fatal("same day must close without callback")
	}
	p.Tap("Due date")
	d.Disabled(true)
	p.Frame()
	if d.Popup().IsOpen() {
		t.Fatal("disabled picker remains open")
	}
}
func TestMenubarSwitchAndKeyboard(t *testing.T) {
	picked := ""
	after := 0
	b := ui.Menubar(ui.Menu("File", ui.MenuItem("Unavailable", nil).Disabled(true), ui.MenuItem("New", func() { picked = "new" })), ui.Menu("Edit", ui.MenuItem("Copy", func() { picked = "copy" })))
	p := ggui.NewProbe(ggui.Column(b, ui.Button("After", func() { after++ })), ggui.Sz(400, 300))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowDown, ggui.KeyEnter)
	if picked != "new" || b.Popup().IsOpen() {
		t.Fatal("keyboard initial action", picked)
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowDown, ggui.KeyArrowRight, ggui.KeyEnter)
	if picked != "copy" || b.Popup().IsOpen() {
		t.Fatal("keyboard switch", picked)
	}
	p.Tap("File")
	edit, _ := p.Find("Edit")
	p.Move(edit.Center())
	p.Tap("Copy")
	if b.Popup().IsOpen() {
		t.Fatal("pointer item failed to close")
	}
	act(t, p, ggui.RoleMenu, "File", ggui.Action{Kind: ggui.ActionExpand})
	p.Type(ggui.Mods{}, ggui.KeyEscape)
	if b.Popup().IsOpen() {
		t.Fatal("Escape")
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyEnter)
	if after != 1 {
		t.Fatal("menubar must have one tab stop", after)
	}
}

func TestCalendarCivilDatesAndWeekStart(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	v := ggui.State(time.Date(2024, 3, 9, 18, 30, 0, 0, loc))
	n := 0
	c := ui.Calendar(v).Location(loc).WeekStartsOn(time.Monday).OnChange(func(time.Time) { n++ })
	p := ggui.NewProbe(c, ggui.Sz(300, 350))
	defer p.Close()
	p.Tap("2024-03-09")
	if n != 0 {
		t.Fatal("same civil date triggered callback")
	}
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyArrowRight, ggui.KeyEnter)
	if v.Peek().Day() != 11 || v.Peek().Hour() != 0 || v.Peek().Location() != loc {
		t.Fatal("DST date arithmetic", v.Peek())
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd, ggui.KeyEnter)
	if v.Peek().Day() != 17 {
		t.Fatal("Monday week end", v.Peek())
	}
	p.Type(ggui.Mods{}, ggui.KeyHome, ggui.KeyEnter)
	if v.Peek().Day() != 11 {
		t.Fatal("Monday week start", v.Peek())
	}
	v.Set(time.Time{})
	p.Frame()
	if !v.Peek().IsZero() {
		t.Fatal("layout changed empty selection")
	}
}

func TestMenubarDisabledAndRebuild(t *testing.T) {
	version := ggui.State(0)
	calls := 0
	var b *ui.MenubarWidget
	p := ggui.ProbeBuilder(func() ggui.Widget {
		_ = version.Get()
		b = ui.Menubar(ui.Menu("Disabled", ui.MenuItem("Never", nil)).Disabled(true), ui.Menu("Enabled", ui.MenuItem("Run", func() { calls++ })))
		b.Key("bar")
		return ggui.Column(b)
	}, ggui.Sz(400, 300))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowDown)
	version.Set(1)
	p.Frame()
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if calls != 1 || b.Popup().IsOpen() {
		t.Fatal("rebuild or disabled skip", calls)
	}
}

func TestMenubarKeyboardIgnoresStationaryPointer(t *testing.T) {
	chosen := ""
	b := ui.Menubar(ui.Menu("File", ui.MenuItem("New", func() { chosen = "new" })), ui.Menu("Edit", ui.MenuItem("Copy", func() { chosen = "copy" })))
	p := ggui.NewProbe(b, ggui.Sz(300, 200))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyArrowRight, ggui.KeyArrowDown, ggui.KeyEnter)
	if chosen != "copy" {
		t.Fatal("stationary pointer overrode keyboard navigation", chosen)
	}
}

func TestButtonStylesPreserveActivationAndDisabledState(t *testing.T) {
	for _, style := range []struct {
		name  string
		apply func(*ui.ButtonWidget) *ui.ButtonWidget
	}{
		{"outline", func(b *ui.ButtonWidget) *ui.ButtonWidget { return b.Outline() }},
		{"secondary", func(b *ui.ButtonWidget) *ui.ButtonWidget { return b.Secondary() }},
		{"ghost", func(b *ui.ButtonWidget) *ui.ButtonWidget { return b.Ghost() }},
		{"destructive", func(b *ui.ButtonWidget) *ui.ButtonWidget { return b.Destructive() }},
	} {
		t.Run(style.name, func(t *testing.T) {
			calls := 0
			b := style.apply(ui.Button("Action", func() { calls++ }))
			p := ggui.NewProbe(b, ggui.Sz(200, 60))
			defer p.Close()
			p.Tap("Action")
			p.Type(ggui.Mods{}, ggui.KeyEnter)
			act(t, p, ggui.RoleButton, "Action", ggui.Action{Kind: ggui.ActionPress})
			if calls != 3 {
				t.Fatal("style changed activation", calls)
			}
			b.Disabled(true)
			p.Frame()
			act(t, p, ggui.RoleButton, "Action", ggui.Action{Kind: ggui.ActionPress})
			if calls != 3 {
				t.Fatal("disabled style activated")
			}
		})
	}
}

func TestMenubarPopupHasBoundedWidth(t *testing.T) {
	b := ui.Menubar(ui.Menu("File", ui.MenuItem("New", nil).Shortcut("⌘N")))
	p := ggui.NewProbe(ggui.Padding(b, 40), ggui.Sz(900, 400))
	defer p.Close()
	p.Tap("File")
	p.Frame()
	r := b.Popup().Rect()
	if r.Size.W > 224 || r.Size.W < 180 || r.Origin.X < 40 {
		t.Fatalf("unbounded or misplaced menu: %+v", r)
	}
}
func TestCommandHoverAndStableHeight(t *testing.T) {
	q := ggui.State("")
	picked := ""
	c := ui.Command(q, ui.CommandItem("One", func() { picked = "one" }), ui.CommandItem("Two", func() { picked = "two" })).Height(160).StableHeight().Borderless()
	p := ggui.NewProbe(ggui.Column(c), ggui.Sz(360, 320))
	defer p.Close()
	p.Tap("Search commands")
	two, _ := p.Find("Two")
	p.Move(two.Center())
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "two" {
		t.Fatal("hover did not update keyboard selection", picked)
	}
	env := ggui.Env{}.WithTheme(ggui.DefaultTheme())
	before := c.Layout(ggui.Loose(ggui.Sz(360, 500)), env)
	q.Set("no results")
	after := c.Layout(ggui.Loose(ggui.Sz(360, 500)), env)
	if before.H != after.H {
		t.Fatal("palette jumps while filtering", before, after)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if picked != "two" {
		t.Fatal("empty command activated")
	}
}
