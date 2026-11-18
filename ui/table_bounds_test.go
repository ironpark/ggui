package ui_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

func manyPeople(n int) []person {
	var many []person
	for i := range n {
		many = append(many, person{ID: i + 1, Name: "p" + strconv.Itoa(i+1), Age: i})
	}
	return many
}

func TestTableMaxHeightHugsItsRowsThenScrolls(t *testing.T) {
	t.Parallel()
	rows := ggui.State(manyPeople(3))
	tbl, chosen := table(rows)
	tbl.MaxHeight(300)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 500))
	defer p.Close()
	// A 40px heading and three 37px rows fit: the table is as tall as they are.
	if sz := p.Frame(); sz.H != 40+3*37 {
		t.Fatalf("height = %v with three rows, want 151", sz.H)
	}
	rows.Set(manyPeople(100))
	if sz := p.Frame(); sz.H != 300 {
		t.Fatalf("height = %v with a hundred rows, want the 300 at most", sz.H)
	}
	if n := len(p.FindAll(ggui.RoleRow)); n == 0 || n > 10 {
		t.Fatalf("visible rows = %d, want only those in view", n)
	}
	head := semNode(t, p, ggui.RoleText, "Name")
	p.Scroll(ggui.Pt(150, 150), ggui.Pt(0, -100000))
	if _, ok := p.Find("p100"); !ok {
		t.Fatal("last row not painted after scrolling to the end")
	}
	if after := semNode(t, p, ggui.RoleText, "Name"); after.Rect != head.Rect {
		t.Fatalf("the heading moved from %v to %v as the body scrolled", head.Rect, after.Rect)
	}
	// The keyboard pages and reveals rows as it does under Height.
	p.Scroll(ggui.Pt(150, 150), ggui.Pt(0, 100000))
	p.Tap("p1")
	p.Type(ggui.Mods{}, ggui.KeyPageDown)
	if got := ggui.Untrack(chosen.Get); got < 3 || got > 10 {
		t.Fatalf("PageDown selected %d, want a page below the first row", got)
	}
	p.Type(ggui.Mods{}, ggui.KeyEnd)
	if got := focusedName(t, p); got != "p100" {
		t.Fatalf("after End: focus on %q, want the last row, scrolled to", got)
	}
}

func TestTableMaxRowsCountsRowsAndHeightWins(t *testing.T) {
	t.Parallel()
	rows := ggui.State(manyPeople(3))
	tbl := footedTable(rows).MaxRows(5)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 500))
	defer p.Close()
	if sz := p.Frame(); sz.H != 40+3*37 {
		t.Fatalf("height = %v with three rows, want 151", sz.H)
	}
	rows.Set(manyPeople(50))
	if sz := p.Frame(); sz.H != 40+5*37 {
		t.Fatalf("height = %v with fifty rows, want a heading and five rows", sz.H)
	}
	// A lower MaxHeight applies too, and the limit follows RowHeight.
	tbl.MaxHeight(150)
	if sz := p.Frame(); sz.H != 150 {
		t.Fatalf("height = %v under MaxHeight(150), want 150", sz.H)
	}
	tbl.MaxHeight(0).RowHeight(30)
	if sz := p.Frame(); sz.H != 40+5*30 {
		t.Fatalf("height = %v with 30px rows, want 190", sz.H)
	}
	tbl.Height(100)
	if sz := p.Frame(); sz.H != 100 {
		t.Fatalf("height = %v under Height(100), want Height to win", sz.H)
	}
	// Without limits the body no longer scrolls: the last row lies where
	// every row above it puts it.
	tbl.Height(0).MaxRows(0)
	p.Frame()
	if last := semNode(t, p, ggui.RoleRow, "p50"); last.Rect.Origin.Y != 40+49*30 {
		t.Fatalf("last row at %v without limits, want at %v", last.Rect, 40+49*30)
	}
}

func buttonTable(rows ggui.Readable[[]person]) *ui.TableWidget[person, int] {
	return ui.Table(rows, func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
		ui.Col("", func(r ggui.Readable[person]) ggui.Widget {
			return ui.Button("Edit "+ggui.Untrack(r.Get).Name, func() {})
		}).W(120),
	).RowName(func(p person) string { return p.Name })
}

func TestTableRowGrowsToAButtonInACell(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(buttonTable(people()), ggui.Sz(400, 500))
	defer p.Close()
	natural := ggui.NewProbe(widget.Column(ui.Button("Edit Ada", func() {})), ggui.Sz(400, 500))
	want := semNode(t, natural, ggui.RoleButton, "Edit Ada").Rect.Size.H
	natural.Close()
	button := semNode(t, p, ggui.RoleButton, "Edit Ada")
	if button.Rect.Size.H != want {
		t.Fatalf("button is %v tall in the row, want its own %v", button.Rect.Size.H, want)
	}
	// The row is the button in the cell padding; the next starts under it.
	ada, grace := semNode(t, p, ggui.RoleRow, "Ada"), semNode(t, p, ggui.RoleRow, "Grace")
	if ada.Rect.Origin.Y != 40 || ada.Rect.Size.H != want+16 || grace.Rect.Origin.Y != 40+want+16 {
		t.Fatalf("rows at %v and %v, want %v tall from 40", ada.Rect, grace.Rect, want+16)
	}
	if math.Abs(button.Rect.Center().Y-ada.Rect.Center().Y) > .01 {
		t.Fatalf("button at %v, want centered in its row %v", button.Rect, ada.Rect)
	}
}

func TestTableScrollingRowsKeepOneHeight(t *testing.T) {
	t.Parallel()
	tbl := buttonTable(people()).MaxRows(10)
	p := ggui.NewProbe(tbl, ggui.Sz(400, 500))
	defer p.Close()
	// Under a scrolling body rows keep the text row's height ...
	if ada := semNode(t, p, ggui.RoleRow, "Ada"); ada.Rect.Size.H != 37 {
		t.Fatalf("row is %v tall under MaxRows, want 37", ada.Rect.Size.H)
	}
	// ... unless RowHeight makes room for the button.
	tbl.RowHeight(53)
	p.Frame()
	if ada := semNode(t, p, ggui.RoleRow, "Ada"); ada.Rect.Size.H != 53 {
		t.Fatalf("row is %v tall under RowHeight(53), want 53", ada.Rect.Size.H)
	}
	if sz := p.Frame(); sz.H != 40+3*53 {
		t.Fatalf("height = %v, want a heading and three 53px rows", sz.H)
	}
}

func TestTableToolbarSitsAboveTheHeadingAndStays(t *testing.T) {
	t.Parallel()
	tbl := footedTable(ggui.State(manyPeople(50))).Height(300).Toolbar(widget.Text("Filters"))
	p := ggui.NewProbe(tbl, ggui.Sz(300, 500))
	defer p.Close()
	if sz := p.Frame(); sz.H != 300 {
		t.Fatalf("height = %v, want the 300 asked for, toolbar included", sz.H)
	}
	// The toolbar's text in 8px of padding, over a 1px line, then the heading.
	bar, head := semNode(t, p, ggui.RoleText, "Filters"), semNode(t, p, ggui.RoleText, "Name")
	barH := 8 + bar.Rect.Size.H + 8 + 1
	if bar.Rect.Origin != ggui.Pt(8, 8) {
		t.Fatalf("toolbar at %v, want inside the cell padding", bar.Rect)
	}
	if head.Rect.Origin.Y < barH || head.Rect.Origin.Y > barH+40 {
		t.Fatalf("heading at %v, want in the 40px under a %vpx toolbar", head.Rect, barH)
	}
	if first := semNode(t, p, ggui.RoleRow, "p1"); first.Rect.Origin.Y != barH+40 {
		t.Fatalf("first row at %v, want under the toolbar and heading at %v", first.Rect, barH+40)
	}
	p.Scroll(ggui.Pt(150, 200), ggui.Pt(0, -100000))
	if after := semNode(t, p, ggui.RoleText, "Filters"); after.Rect != bar.Rect {
		t.Fatalf("scrolling moved the toolbar from %v to %v", bar.Rect, after.Rect)
	}
	if _, ok := p.Find("p1"); ok {
		t.Fatal("the body did not scroll")
	}
	// A maximum counts it too, and Toolbar(nil) takes it away.
	tbl.Height(0).MaxRows(2)
	if sz := p.Frame(); sz.H != barH+40+2*37 {
		t.Fatalf("height = %v under MaxRows(2), want toolbar, heading and two rows", sz.H)
	}
	tbl.Toolbar(nil)
	if sz := p.Frame(); sz.H != 40+2*37 {
		t.Fatalf("height = %v without the toolbar, want 114", sz.H)
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "Filters"); ok {
		t.Fatal("Toolbar(nil) left the toolbar in place")
	}
}

func TestTableToolbarTakesInput(t *testing.T) {
	t.Parallel()
	taps := 0
	tbl := footedTable(people()).Toolbar(widget.Row(ui.Button("Add", func() { taps++ })))
	p := ggui.NewProbe(tbl, ggui.Sz(300, 500))
	defer p.Close()
	p.Tap("Add")
	if taps != 1 {
		t.Fatalf("taps = %d, want the toolbar's button pressed", taps)
	}
}
