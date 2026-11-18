package ui_test

import (
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
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
