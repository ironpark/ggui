package ui_test

import (
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

type person struct {
	ID   int
	Name string
	Age  int
}

func people() *ggui.StateValue[[]person] {
	return ggui.State([]person{{1, "Ada", 36}, {2, "Grace", 45}, {3, "Linus", 28}})
}

func table(rows ggui.Readable[[]person]) (*ui.TableWidget[person, int], *ggui.StateValue[int]) {
	chosen := ggui.State(0)
	t := ui.Table(rows, func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }).W(60).Right(),
	).BindSelected(chosen).RowName(func(p person) string { return p.Name })
	return t, chosen
}

func TestTableRowsSelectOnClick(t *testing.T) {
	rows := people()
	tbl, chosen := table(rows)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()

	if got := len(p.FindAll(ggui.RoleRow)); got != 3 {
		t.Fatalf("rows = %d, want 3", got)
	}
	p.Tap("Grace")
	if ggui.Untrack(chosen.Get) != 2 {
		t.Fatalf("chosen = %d after tapping Grace, want 2", ggui.Untrack(chosen.Get))
	}

	// Rows follow the list: a removal drops its row, an edit updates it.
	ggui.Remove(rows, func(x person) bool { return x.ID == 1 })
	if _, ok := p.Find("Ada"); ok {
		t.Fatal("Ada still has a row after removal")
	}
	if got := len(p.FindAll(ggui.RoleRow)); got != 2 {
		t.Fatalf("rows = %d after removal, want 2", got)
	}
}

func TestTableHeightScrollsBody(t *testing.T) {
	var many []person
	for i := range 100 {
		many = append(many, person{ID: i + 1, Name: "p" + strconv.Itoa(i+1), Age: i})
	}
	rows := ggui.State(many)
	tbl, _ := table(rows)
	tbl.Height(150).RowHeight(30)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 400))
	defer p.Close()

	if sz := p.Frame(); sz.H != 150 {
		t.Fatalf("height = %v, want 150", sz.H)
	}
	// Only the rows in view are laid out and painted.
	if n := len(p.FindAll(ggui.RoleRow)); n == 0 || n > 10 {
		t.Fatalf("visible rows = %d, want a handful", n)
	}
	if _, ok := p.Find("p100"); ok {
		t.Fatal("last row painted before scrolling")
	}
	p.Scroll(ggui.Pt(150, 100), ggui.Pt(0, -100000))
	if _, ok := p.Find("p100"); !ok {
		t.Fatal("last row not painted after scrolling to the end")
	}
}

func TestTableAlignsCellsAndHonorsRowHeight(t *testing.T) {
	for _, align := range []float64{0, .5, 1} {
		var cellRect ggui.Rect
		cell := ggui.FromFuncs(func(c ggui.Constraints, _ ggui.Env) ggui.Size {
			return c.Constrain(ggui.Sz(10, 10))
		}, func(_ *ggui.Canvas, r ggui.Rect) { cellRect = r })
		col := ui.Col("Value", func(ggui.Readable[person]) ggui.Widget { return cell }).W(80)
		col.Align = align
		rows := ggui.State([]person{{ID: 1, Name: "Ada"}})
		tbl := ui.Table(rows, func(p person) int { return p.ID },
			ui.TextCol("Name", func(p person) string { return p.Name }), col,
		).RowHeight(52)
		p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
		p.Frame()
		wantX := 228 + 54*align
		if cellRect.Origin.X != wantX || cellRect.Origin.Y != 61 || cellRect.Size != ggui.Sz(10, 10) {
			t.Errorf("align %v: cell = %+v, want 10x10 at (%v, 61)", align, cellRect, wantX)
		}
		p.Close()
	}
}

func TestTableRowNameFollowsEdits(t *testing.T) {
	rows := people()
	tbl, chosen := table(rows)
	p := ggui.NewProbe(tbl, ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	rows.Set([]person{{1, "Augusta", 37}, {2, "Grace", 45}})
	if _, ok := p.Find("Ada"); ok {
		t.Fatal("stale row name after edit")
	}
	if _, ok := p.Find("Augusta"); !ok {
		t.Fatal("updated row name missing")
	}
	p.Tap("Augusta")
	if ggui.Untrack(chosen.Get) != 1 {
		t.Fatal("edited row lost its selection key")
	}
}
