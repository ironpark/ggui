package ui_test

import (
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// shadcn's DialogTitle is text-base leading-none, a 16px box, over gap-2;
// AlertDialogTitle and SheetTitle are left in text-base, whose line is
// 24px, over gap-1.5 and gap-0.5. Each description is text-sm, a 20px line.
func TestModalTitlesAreBoxesOfTheirLineHeight(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name          string
		build         func(open ggui.Binding[bool]) ggui.Widget
		title, toDesc float64 // the title's height, and from its top to the description's
	}{
		{"dialog", func(o ggui.Binding[bool]) ggui.Widget {
			return ui.Dialog(o, ggui.Text("Body")).Title("Title here").Description("Description here")
		}, 16, 16 + 8},
		{"alert dialog", func(o ggui.Binding[bool]) ggui.Widget {
			return ui.AlertDialog(o, "Title here", "Description here")
		}, 24, 24 + 6},
		{"sheet", func(o ggui.Binding[bool]) ggui.Widget {
			return ui.Sheet(o, ggui.Text("Body")).Title("Title here").Description("Description here")
		}, 24, 24 + 2},
	} {
		p := ggui.NewProbe(c.build(ggui.State(true)), ggui.Sz(800, 600))
		p.Advance(time.Second)
		p.Advance(time.Second)
		title, ok := p.Semantics().Find(ggui.RoleHeading, "Title here")
		desc, ok2 := p.Semantics().Find(ggui.RoleText, "Description here")
		if !ok || !ok2 {
			t.Fatalf("%s: no title or description painted", c.name)
		}
		if got := title.Rect.Size.H; math.Abs(got-c.title) > 1e-6 {
			t.Errorf("%s: the title is %v tall, want %v", c.name, got, c.title)
		}
		if got := desc.Rect.Origin.Y - title.Rect.Origin.Y; math.Abs(got-c.toDesc) > 1e-6 {
			t.Errorf("%s: the description starts %v under the title's top, want %v", c.name, got, c.toDesc)
		}
		if got := desc.Rect.Size.H; math.Abs(got-20) > 1e-6 {
			t.Errorf("%s: the description is %v tall, want its 20px line", c.name, got)
		}
		p.Close()
	}
}
