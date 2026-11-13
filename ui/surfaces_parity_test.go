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

// A group's count bubble is shadcn's AvatarGroupCount, which has neither
// the avatar's after:border hairline nor the fallback's text-xs at the
// small size: text-sm whatever the avatars beside it.
func TestAvatarGroupCountHasNoHairlineAndKeepsTheBodyText(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	g := AvatarGroup(Avatar("Ada").Small(), Avatar("Bea").Small(), Avatar("Cy").Small()).Max(1)
	g.Layout(ggui.Loose(ggui.Sz(300, 100)), env)
	if g.count == nil || !g.count.counts || g.avatars[0].counts {
		t.Fatal("only the bubble at the end counts")
	}
	if got := g.sizes[len(g.sizes)-1]; got != ggui.Sz(24, 24) {
		t.Fatalf("the bubble beside small avatars is %v, want 24 across", got)
	}
	body := ggui.Text("+2").NoWrap().Style(theme.Text).Layout(ggui.Loose(ggui.Sz(100, 100)), env)
	if g.count.textSize != body {
		t.Fatalf("the count is set %v, want the body text's %v", g.count.textSize, body)
	}
	if small := g.avatars[0].textSize; small.H >= body.H {
		t.Fatalf("a small avatar's initials are %v tall, want the caption's, under %v", small.H, body.H)
	}
}

// A line strip's bar hangs off the trigger's padding box: after:inset-x-0
// after:bottom-[-5px] h-0.5 under a 25px trigger centered in the list's
// 26px inside, which is 30.5 to 32.5 down a 32px list, and
// after:inset-y-0 after:-right-1 w-0.5 in the last two pixels of a
// vertical one.
func TestTabsLineBarHangsOffTheTriggersPaddingBox(t *testing.T) {
	t.Parallel()
	tabs := Tabs(ggui.State(0), Tab("One", nil), Tab("Two", nil)).Line()
	p := ggui.NewProbe(ggui.Column(tabs), ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	strip, lr := ggui.Rct(ggui.Pt(0, 0), ggui.Sz(tabs.stripW, tabs.stripH)), tabs.labelRect[0]
	if got, want := tabs.barRect(strip, lr), ggui.Rct(ggui.Pt(lr.Origin.X+1, strip.Size.H-1.5), ggui.Sz(lr.Size.W-2, 2)); got != want {
		t.Fatalf("the bar is at %v, want %v", got, want)
	}
	side := Tabs(ggui.State(0), Tab("One", nil), Tab("Two", nil)).Line().Vertical()
	q := ggui.NewProbe(ggui.Column(side), ggui.Sz(320, 200))
	defer q.Close()
	q.Frame()
	strip, lr = ggui.Rct(ggui.Pt(0, 0), ggui.Sz(side.stripW, side.stripH)), side.labelRect[0]
	if got, want := side.barRect(strip, lr), ggui.Rct(ggui.Pt(strip.Size.W-2, lr.Origin.Y+1), ggui.Sz(2, lr.Size.H-2)); got != want {
		t.Fatalf("the vertical bar is at %v, want %v", got, want)
	}
}

// An item given an action is shadcn's Item rendered as a link: the Muted
// surface eased in under the pointer over 100ms, the pointer cursor, a
// link named by the title that Enter runs, while its own Action button
// keeps its click. An item with no action takes no input at all.
func TestItemWithAnActionIsALinkAndOneWithoutTakesNoInput(t *testing.T) {
	t.Parallel()
	taps, runs := 0, 0
	plain := Item("Backups", "Last run 2 hours ago")
	link := Item("Changelog", "What is new").Action(Button("Run", func() { runs++ })).OnTap(func() { taps++ })
	a, b := &placed{Widget: plain}, &placed{Widget: link}
	p := ggui.NewProbe(ggui.Column(a, b).Align(ggui.AlignStretch), ggui.Sz(320, 300))
	defer p.Close()
	p.Frame()
	p.Move(center(a.at))
	p.Frame()
	if p.Cursor() == ggui.CursorShapePointer || plain.Hovered {
		t.Fatal("an item with no action took the pointer")
	}
	if _, ok := p.FindRole(ggui.RoleLink, "Backups"); ok {
		t.Fatal("an item with no action is a link")
	}
	at := ggui.Pt(b.at.Origin.X+20, b.at.Origin.Y+b.at.Size.H/2)
	p.Move(at)
	p.Frame()
	p.Advance(link.theme.MotionFast / 3)
	p.Frame()
	if k := link.hover; !(k > 0 && k < 1) || p.Cursor() != ggui.CursorShapePointer {
		t.Fatalf("50ms in the row is %v onto its hover surface with cursor %v, want it easing under a pointer cursor", k, p.Cursor())
	}
	p.Advance(link.theme.MotionFast)
	p.Frame()
	if link.hover != 1 {
		t.Fatalf("the settled row is %v onto its hover surface", link.hover)
	}
	p.Click(at)
	if f, ok := p.FindRole(ggui.RoleLink, "Changelog"); !ok || f.Rect != b.at {
		t.Fatalf("the row is not a link named by its title: %v %v", f, ok)
	}
	p.Tap("Run")
	if taps != 1 || runs != 1 {
		t.Fatalf("a click on the row and one on its button ran the row %d times and the button %d, want 1 and 1", taps, runs)
	}
	p.Type(ggui.Mods{}, ggui.KeyTab)
	p.Frame()
	if !link.Focused || !link.FocusVisible {
		t.Fatal("Tab did not bring the focus to the row")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if taps != 2 {
		t.Fatalf("Enter on the focused row ran it %d times in all, want 2", taps)
	}
}

// MenubarLabel hands DropdownMenuLabel "text-sm" over its "text-xs" and
// leaves its text-muted-foreground: a heading in a menubar's menu is a
// row's 20px line where a dropdown's is the caption's 16.
func TestMenuLabelInAMenubarIsTheSizeOfARow(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 400))
	inMenu, inBar := MenuLabel("Appearance"), MenuLabel("Appearance")
	Menu("View", inMenu, MenuItem("Zoom", nil))
	Menubar(Menu("View", inBar, MenuItem("Zoom", nil)))
	small, large := inMenu.Layout(c, env), inBar.Layout(c, env)
	rows := theme.ItemPad.Top + theme.ItemPad.Bottom
	body := ggui.Text("Appearance").Style(theme.Text).Layout(c, env).H
	caption := Caption("Appearance").Layout(c, env).H
	if small.H != caption+rows || large.H != body+rows || large.H <= small.H {
		t.Fatalf("a dropdown's label is %v tall and a menubar's %v, want %v and %v", small.H, large.H, caption+rows, body+rows)
	}
}
