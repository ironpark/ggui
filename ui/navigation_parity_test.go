package ui

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestCarouselTakesItsDirectionAndGapFromTheEnv(t *testing.T) {
	t.Parallel()
	c := Carousel(ggui.State(0), carouselSlides(3, 1)...)
	p := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, ggui.DirRTL, c), ggui.Sz(416, 240))
	defer p.Close()
	p.Frame()
	if !c.rtl {
		t.Fatal("a carousel under a right-to-left Env runs left to right")
	}
	if want := 2 * uitheme.Default().Space; c.gap != want {
		t.Fatalf("slides are %v apart, want two spacing units, %v", c.gap, want)
	}
	prev, _ := p.Semantics().Find(ggui.RoleButton, "Previous slide")
	next, _ := p.Semantics().Find(ggui.RoleButton, "Next slide")
	if prev.Rect.Origin.X < next.Rect.Origin.X {
		t.Fatal("Previous is left of Next under right-to-left")
	}
	if prev.Rect.Size != ggui.Sz(28, 28) {
		t.Fatalf("a navigation button is %v, want the 28px small icon button", prev.Rect.Size)
	}

	forced := Carousel(ggui.State(0), carouselSlides(3, 1)...).RTL(false).Gap(4)
	q := ggui.NewProbe(ggui.Provide(ggui.DirectionKey, ggui.DirRTL, forced), ggui.Sz(416, 240))
	defer q.Close()
	q.Frame()
	if forced.rtl || forced.gap != 4 {
		t.Fatalf("RTL(false) and Gap(4) were overridden: rtl=%v gap=%v", forced.rtl, forced.gap)
	}
}

func TestAccordionHeaderDrawsAWidgetAndKeepsTheTitleAsItsName(t *testing.T) {
	t.Parallel()
	open := ggui.State([]string{})
	a := Accordion(open,
		AccordionItem("billing", "Billing", ggui.Text("Body")).Header(ggui.Row(Icon(icons.File).Alt("Invoice"), ggui.Text("Billing and invoices")).Gap(8)),
		AccordionItem("plain", "Plain", ggui.Text("Body")),
	)
	p := ggui.NewProbe(a, ggui.Sz(400, 300))
	defer p.Close()
	tree := p.Semantics()
	if _, ok := tree.Find(ggui.RoleDisclosure, "Billing"); !ok {
		t.Fatalf("the title does not name a section with a Header:\n%s", tree)
	}
	if _, ok := tree.Find(ggui.RoleImage, "Invoice"); !ok {
		t.Fatal("the Header's widget was not painted")
	}
	p.Tap("Billing")
	if got := ggui.Untrack(open.Get); len(got) != 1 || got[0] != "billing" {
		t.Fatalf("clicking a custom header opened %v", got)
	}
	if a.headers[0].size.H != a.headers[1].size.H || a.heights[0] != a.heights[1] {
		t.Fatalf("a one-line custom header is %v high in a %v row; a text one %v in %v", a.headers[0].size.H, a.heights[0], a.headers[1].size.H, a.heights[1])
	}
}

func TestResizableDividerIsALineThePanesMeetAt(t *testing.T) {
	t.Parallel()
	r := Resizable(ggui.State(.5), ggui.Box(), ggui.Box())
	p := ggui.NewProbe(r, ggui.Sz(201, 100))
	defer p.Close()
	p.Frame()
	if r.firstRect.Size.W != 100 || r.secondRect.Origin.X != 101 || r.secondRect.Size.W != 100 {
		t.Fatalf("the panes are %v and %v, want 100 wide either side of a 1px line", r.firstRect, r.secondRect)
	}
	if r.handle.Size.W != 8 || r.handle.Origin.X != 96.5 {
		t.Fatalf("the divider is grabbed at %v, want 8 wide about the line", r.handle)
	}
}
