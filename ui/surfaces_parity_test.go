package ui

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

// placed records where its child was last painted.
type placed struct {
	ggui.Widget
	at ggui.Rect
}

func (p *placed) Paint(dst *ggui.Canvas, r ggui.Rect) { p.at = r; dst.Paint(p.Widget, r) }

func center(r ggui.Rect) ggui.Point {
	return ggui.Pt(r.Origin.X+r.Size.W/2, r.Origin.Y+r.Size.H/2)
}

// A ghost badge takes shadcn's hover:bg-muted under the pointer, eased, and
// a link badge its underline, without claiming the pointer: the tab the
// badge sits in still has the hover and the click.
func TestBadgeLooksUnderThePointerLeaveItToWhatTheBadgeIsIn(t *testing.T) {
	t.Parallel()
	sel := ggui.State(0)
	ghost := Badge("3").Ghost()
	spot := &placed{Widget: ghost}
	tabs := Tabs(sel, Tab("Build", ggui.Text("b")), Tab("Test", ggui.Text("t")).Header(ggui.Row(ggui.Text("Test"), spot).Gap(6)))
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(400, 200))
	defer p.Close()
	p.Frame()
	if ghost.hover != 0 {
		t.Fatalf("a badge away from the pointer is %v into its hover look", ghost.hover)
	}
	p.Move(center(spot.at))
	p.Frame()
	p.Advance(ghost.theme.MotionFast / 2)
	p.Frame()
	if k := ghost.hover; !(k > 0 && k < 1) {
		t.Fatalf("halfway the badge is %v into its hover look, want it easing", k)
	}
	p.Advance(time.Second)
	p.Frame()
	if ghost.hover != 1 {
		t.Fatalf("the settled badge is %v into its hover look", ghost.hover)
	}
	p.Click(center(spot.at))
	if got := ggui.Untrack(sel.Get); got != 1 {
		t.Fatalf("a click on the badge picked tab %d, want the tab it is in", got)
	}
	p.Move(ggui.Pt(390, 190))
	p.Frame()
	p.Advance(time.Second)
	p.Frame()
	if ghost.hover != 0 {
		t.Fatalf("the badge the pointer left is %v into its hover look", ghost.hover)
	}
}

// The looks shadcn gives a badge only as a link, [a]:hover:bg-primary/80
// and its siblings, wait for an action: OnTap makes the badge a link with
// the pointer cursor that Space and Enter run.
func TestBadgeWithAnActionIsALink(t *testing.T) {
	t.Parallel()
	plain, link := Badge("Docs"), Badge("Guide")
	taps := 0
	link.OnTap(func() { taps++ })
	a, b := &placed{Widget: plain}, &placed{Widget: link}
	p := ggui.NewProbe(ggui.Row(a, b).Gap(8), ggui.Sz(300, 100))
	defer p.Close()
	p.Frame()
	p.Move(center(a.at))
	p.Frame()
	p.Advance(time.Second)
	p.Frame()
	if plain.hover != 0 || p.Cursor() == ggui.CursorShapePointer {
		t.Fatalf("a primary badge with no action reacts to the pointer: hover %v", plain.hover)
	}
	p.Move(center(b.at))
	p.Frame()
	p.Advance(time.Second)
	p.Frame()
	if link.hover != 1 || p.Cursor() != ggui.CursorShapePointer {
		t.Fatalf("a badge with an action is %v into its link look with cursor %v", link.hover, p.Cursor())
	}
	if f, ok := p.FindRole(ggui.RoleLink, "Guide"); !ok || f.Rect != b.at {
		t.Fatalf("the badge is not a link named by its label: %v %v", f, ok)
	}
	p.Click(center(b.at))
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if taps != 2 {
		t.Fatalf("a click and Enter ran the action %d times", taps)
	}
	st := BadgeOutline.Style(uitheme.Default())
	if st.LinkFill == nil || st.HoverFill != nil {
		t.Fatalf("the outline badge's muted surface is for a link only: %+v", st)
	}
}

// A header's action stands shadcn's gap-1 from the text, the gap its grid
// has between rows too.
func TestCardHeaderActionIsAGridGapFromTheText(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	action := &placed{Widget: ggui.Box().Size(40, 20)}
	h := CardHeader("Team", "Invite people to the project and give each of them a role in it.").Action(action)
	p := ggui.NewProbe(h, ggui.Sz(260, 200))
	defer p.Close()
	p.Frame()
	var title ggui.SemNode
	for _, n := range p.Semantics().All() {
		if n.Role == ggui.RoleHeading {
			title = n
		}
	}
	if title.Name != "Team" {
		t.Fatal("the header has no title")
	}
	if got, want := action.at.Origin.X+40, 260-theme.CardPad.Right; got != want {
		t.Fatalf("the action ends at %v, want the header's padded right edge %v", got, want)
	}
	if gap := action.at.Origin.X - (title.Rect.Origin.X + title.Rect.Size.W); gap != 4 {
		t.Fatalf("the action is %v from the text, want 4", gap)
	}
}

// Media in a card's first place takes the top padding's place and runs to
// the card's sides, shadcn's has-[>img:first-child]:pt-0 on a direct child
// with no px; anywhere else it keeps the spacing above it.
func TestCardMediaInFirstPlaceIsFlushWithTheTopAndSides(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	media := &placed{Widget: ggui.Box().Height(90)}
	lead := CardMedia(media)
	card := Card(lead, CardHeader("Lisbon", ""))
	p := ggui.NewProbe(ggui.Column(card).Align(ggui.AlignStretch), ggui.Sz(300, 400))
	defer p.Close()
	p.Frame()
	b := theme.BorderWidth
	if want := ggui.Rct(ggui.Pt(b, b), ggui.Sz(300-2*b, 90)); media.at != want {
		t.Fatalf("leading media is painted at %v, want %v", media.at, want)
	}
	if lead.top != theme.RadiusLg-b || lead.bottom != 0 {
		t.Fatalf("leading media rounds its top by %v and its bottom by %v, want the card's inner radius and 0", lead.top, lead.bottom)
	}
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(300, 400))
	first := Card(CardMedia(ggui.Box().Height(90)), CardContent(ggui.Box().Height(20))).Layout(cs, env)
	second := Card(CardContent(ggui.Box().Height(20)), CardMedia(ggui.Box().Height(90))).Layout(cs, env)
	if got := second.H - first.H; got != theme.CardPad.Top-b {
		t.Fatalf("media in last place makes the card %v taller than media in first place, want the top padding it does not replace", got)
	}
}
