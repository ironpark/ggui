package ui_test

import (
	"math"
	"strconv"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func footedTable(rows ggui.Readable[[]person]) *ui.TableWidget[person, int] {
	return ui.Table(rows, func(p person) int { return p.ID },
		ui.TextCol("Name", func(p person) string { return p.Name }),
		ui.TextCol("Age", func(p person) string { return strconv.Itoa(p.Age) }).W(80).Right(),
	).RowName(func(p person) string { return p.Name })
}

func TestTableRowsFollowTheThemeAndFooterAndCaptionSitBelow(t *testing.T) {
	t.Parallel()
	rows := ggui.State([]person{{1, "Ada", 36}, {2, "Grace", 45}, {3, "Linus", 28}})
	tbl := footedTable(rows).Footer(ggui.Text("Total"), ggui.Text("109")).Caption("Three pioneers.")
	p := ggui.NewProbe(ggui.Column(tbl).Align(ggui.AlignStretch), ggui.Sz(400, 400))
	defer p.Close()
	// A 40px heading row, then 37px rows: a 20px line in 8px of padding over a 1px line.
	ada, grace := semNode(t, p, ggui.RoleRow, "Ada"), semNode(t, p, ggui.RoleRow, "Grace")
	if ada.Rect.Origin.Y != 40 || ada.Rect.Size.H != 37 || grace.Rect.Origin.Y != 77 {
		t.Fatalf("rows at %v and %v, want 37 tall from 40", ada.Rect, grace.Rect)
	}
	// The footer is one more row; its cells keep their columns' place.
	total, sum := semNode(t, p, ggui.RoleText, "Total"), semNode(t, p, ggui.RoleText, "109")
	if y := total.Rect.Center().Y; y < 40+3*37 || y > 40+4*37 {
		t.Fatalf("footer text at %v, want inside the row under the last", total.Rect)
	}
	if total.Rect.Origin.X != 8 || sum.Rect.Origin.X+sum.Rect.Size.W != 400-8 {
		t.Fatalf("footer cells at %v and %v, want left in Name and right in Age", total.Rect, sum.Rect)
	}
	// The caption is centered 16px under the footer.
	caption := semNode(t, p, ggui.RoleText, "Three pioneers.")
	if math.Abs(caption.Rect.Origin.Y-(40+4*37+16)) > .01 || math.Abs(caption.Rect.Center().X-200) > .01 {
		t.Fatalf("caption at %v, want centered 16px under the footer", caption.Rect)
	}
	if table := semNode(t, p, ggui.RoleTable, ""); math.Abs(table.Rect.Size.H-(40+4*37+16+caption.Rect.Size.H)) > .01 {
		t.Fatalf("table is %v tall, want its rows, footer and caption", table.Rect.Size.H)
	}
	// Both go away again.
	tbl.Footer().Caption("")
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleText, "Total"); ok {
		t.Fatal("Footer() left the footer in place")
	}
	if table := semNode(t, p, ggui.RoleTable, ""); table.Rect.Size.H != 40+3*37 {
		t.Fatalf("table is %v tall without footer and caption, want 151", table.Rect.Size.H)
	}
}

func TestTableFooterStaysUnderAScrollingBody(t *testing.T) {
	t.Parallel()
	var many []person
	for i := range 50 {
		many = append(many, person{ID: i + 1, Name: "p" + strconv.Itoa(i+1), Age: i})
	}
	tbl := footedTable(ggui.State(many)).Height(200).Footer(ggui.Text("Total"), nil).Caption("Fifty rows.")
	p := ggui.NewProbe(tbl, ggui.Sz(300, 400))
	defer p.Close()
	if sz := p.Frame(); sz.H != 200 {
		t.Fatalf("height = %v, want the 200 asked for, caption included", sz.H)
	}
	caption := semNode(t, p, ggui.RoleText, "Fifty rows.")
	total := semNode(t, p, ggui.RoleText, "Total")
	if bottom := caption.Rect.Origin.Y + caption.Rect.Size.H; math.Abs(bottom-200) > .01 || total.Rect.Center().Y > caption.Rect.Origin.Y-16 {
		t.Fatalf("footer at %v and caption at %v, want them at the bottom of 200", total.Rect, caption.Rect)
	}
	p.Scroll(ggui.Pt(100, 100), ggui.Pt(0, -400))
	p.Frame()
	if after := semNode(t, p, ggui.RoleText, "Total"); after.Rect != total.Rect {
		t.Fatalf("scrolling moved the footer from %v to %v", total.Rect, after.Rect)
	}
	if _, ok := p.Find("p1"); ok {
		t.Fatal("the body did not scroll")
	}
}
