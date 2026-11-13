package ui

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
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

// A sheet's scrim is shadcn's transition-opacity duration-150 under a
// panel that takes 200ms: it is all there while the panel is still
// sliding. A drawer's fades over the drawer's own time on its own curve,
// which is behind the panel's early on.
func TestSheetScrimFadesOnItsOwnClock(t *testing.T) {
	t.Parallel()
	open := ggui.State(false)
	sheet := Sheet(open, ggui.Text("body"))
	p := ggui.NewProbe(ggui.Column(sheet), ggui.Sz(600, 400))
	defer p.Close()
	p.Frame()
	open.Set(true)
	p.Frame()
	p.Advance(sheet.theme.MotionFast)
	p.Frame()
	if v := sheet.slide.Value(ggui.FrameTime()); sheet.dim != 1 || !(v > 0 && v < 1) {
		t.Fatalf("150ms in the scrim is %v in and the panel %v, want the scrim there and the panel on its way", sheet.dim, v)
	}

	rises := ggui.State(false)
	drawer := Drawer(rises, ggui.Box().Height(200))
	q := ggui.NewProbe(ggui.Column(drawer), ggui.Sz(600, 400))
	defer q.Close()
	q.Frame()
	rises.Set(true)
	q.Frame()
	q.Advance(drawer.theme.MotionDrawer / 5)
	q.Frame()
	now := ggui.FrameTime()
	if v := drawer.slide.Value(now); !(drawer.dim > 0 && drawer.dim < v) {
		t.Fatalf("90ms in a drawer's scrim is %v in and its panel %v, want the scrim behind on cubic-bezier(.32,.72,0,1)", drawer.dim, v)
	}
}

// A checked command entry carries shadcn's trailing check,
// group-data-[checked=true]/command-item:opacity-100, which an entry with
// a shortcut hides: group-has-data-[slot=command-shortcut]/command-item:hidden.
func TestCommandEntryCheckedShowsATrailingCheckUnlessItHasAShortcut(t *testing.T) {
	t.Parallel()
	c := Command(ggui.State(""),
		CommandItem("Light", nil).Checked(true),
		CommandItem("Dark", nil).Checked(false),
		CommandItem("System", nil).Checked(true).Shortcut("cmd+s"),
		CommandItem("Plain", nil),
	)
	p := ggui.NewProbe(c, ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	light, dark, system, plain := c.items[0], c.items[1], c.items[2], c.items[3]
	if !light.checkable || !light.isChecked() || light.markHidden {
		t.Fatal("a checked entry shows no check")
	}
	if !dark.checkable || dark.isChecked() {
		t.Fatal("an unchecked entry is not a checkable row left unchecked")
	}
	if !system.isChecked() || !system.markHidden || system.pad.Right != light.theme.Space {
		t.Fatalf("an entry with a shortcut keeps room for a check: right padding %v", system.pad.Right)
	}
	if plain.checkable {
		t.Fatal("an entry never given Checked is checkable")
	}
	if n := light.Describe(); n.Checked != ggui.Tri(true) {
		t.Fatalf("a checked entry is read out as %v", n.Checked)
	}
}

// Moving from one open menu of a bar to the next is, in shadcn, one popup
// playing data-closed:animate-out where it was while another plays
// data-open:animate-in: MenubarContent hands its classes to
// DropdownMenuContent, whose exit classes stay through the merge. The bar
// has the menu left go out as the one moved to comes in.
func TestMenubarCrossesTheMenuLeftWithTheMenuMovedTo(t *testing.T) {
	t.Parallel()
	bar := Menubar(Menu("File", MenuItem("New", nil)), Menu("Edit", MenuItem("Copy", nil)))
	p := ggui.NewProbe(bar, ggui.Sz(400, 300))
	defer p.Close()
	p.Tap("File")
	p.Advance(time.Second)
	p.Frame()
	if bar.left != -1 || bar.arriving {
		t.Fatal("opening a menu from a closed bar crosses with nothing")
	}
	first := bar.popup.Rect()
	p.Type(ggui.Mods{}, ggui.KeyArrowRight)
	p.Frame()
	if bar.active != 1 || bar.left != 0 || bar.leftAt != first || !bar.arriving {
		t.Fatalf("after Right the bar is on %d with %d leaving from %v (arriving %v), want 1 with 0 leaving from %v", bar.active, bar.left, bar.leftAt, bar.arriving, first)
	}
	p.Advance(bar.env.PopupDuration() / 2)
	p.Frame()
	now := ggui.FrameTime()
	if in, out := bar.arrive.Value(now), bar.depart.Value(now); !(in > 0 && in < 1) || !(out > 0 && out < 1) {
		t.Fatalf("halfway the menu moved to is %v in and the one left %v, want both on their way", in, out)
	}
	p.Advance(time.Second)
	p.Frame()
	p.Frame()
	if bar.left != -1 || bar.arriving || !bar.popup.IsOpen() {
		t.Fatalf("settled, menu %d is still leaving (arriving %v, open %v)", bar.left, bar.arriving, bar.popup.IsOpen())
	}
}

// The navigation panel leaves as it came, at data-ending-style:scale-90:
// its own scale runs back down over the 150ms the popup fades it in.
func TestNavigationMenuPanelScalesDownAsItLeaves(t *testing.T) {
	t.Parallel()
	nav := NavigationMenu(NavigationMenuItem("Products", ggui.Column(NavigationMenuLink("Editor", nil))))
	p := ggui.NewProbe(ggui.Column(nav), ggui.Sz(500, 300))
	defer p.Close()
	p.Tap("Products")
	p.Frame()
	p.Advance(time.Second)
	p.Frame()
	if got := nav.enter.Value(ggui.FrameTime()); got != 1 {
		t.Fatalf("the settled panel is %v of the way to its size", got)
	}
	p.Move(ggui.Pt(490, 290))
	nav.popup.Hide()
	p.Frame()
	p.Advance(navigationMenuExit / 2)
	p.Frame()
	if got := nav.enter.Value(ggui.FrameTime()); !(got > 0 && got < 1) || nav.popup.IsOpen() {
		t.Fatalf("75ms after closing the panel is %v of the way to its size (open %v), want it on its way down", got, nav.popup.IsOpen())
	}
}

// A rail's tip is shadcn's Tooltip, bg-foreground: the page's text color,
// where a palette gives the sidebar a foreground of its own.
func TestSidebarTipIsFilledWithThePagesForeground(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	theme.SidebarFg = color.NRGBA{R: 200, G: 30, B: 30, A: 255}
	collapsed := ggui.State(true)
	bar := Sidebar(ggui.State("a"), SidebarItem("a", "Alpha").Icon(icons.File).Tooltip("Alpha tip")).
		Collapsible(SidebarCollapseIcon).BindCollapsed(collapsed)
	p := ggui.NewProbe(ggui.WithEnv(theme.Apply, ggui.Row(bar, ggui.Expanded(ggui.Box()))), ggui.Sz(600, 400))
	defer p.Close()
	p.Frame()
	p.Advance(time.Second)
	alpha, ok := p.FindRole(ggui.RoleTab, "Alpha")
	if !ok {
		t.Fatal("no row on the rail")
	}
	p.Move(alpha.Center())
	p.Advance(time.Second)
	p.Frame()
	if bar.tip.surface == nil {
		t.Fatal("no tip over the rail's row")
	}
	if got := bar.tip.surface.fill; !sameColor(got, theme.Fg) || sameColor(got, bar.theme.Fg) {
		t.Fatalf("the tip is filled with %v, want the page's %v and not the sidebar's %v", got, theme.Fg, bar.theme.Fg)
	}
}

// shadcn's Questionnaire and its item are both flex-col gap-4, and the
// item is a fieldset whose title is its legend, which no flex gap reaches:
// 16 from the progress to the title, the description straight under the
// title, 16 on to the choices, and the title's mb-4 in the description's
// place where there is none.
func TestQuestionnairePartsAreAGapApartAndTheDescriptionUnderTheTitle(t *testing.T) {
	t.Parallel()
	rects := func(q Question) (progress, title, description, choice ggui.Rect) {
		w := Questionnaire(ggui.State(QuestionAnswers{}), q)
		p := ggui.NewProbe(w, ggui.Sz(450, 450))
		defer p.Close()
		for _, n := range p.Semantics().All() {
			switch {
			case n.Role == ggui.RoleProgress:
				progress = n.Rect
			case n.Name == q.Title:
				title = n.Rect
			case q.Description != "" && n.Name == q.Description:
				description = n.Rect
			case n.Name == "Task A":
				choice = n.Rect
			}
		}
		return
	}
	bottom := func(r ggui.Rect) float64 { return r.Origin.Y + r.Size.H }
	choices := []QuestionOption{{Value: "a", Label: "Task A"}}
	progress, title, description, choice := rects(Question{Name: "one", Title: "Choose a task", Description: "Pick the one to start with.", Choices: choices})
	if title.Size.H == 0 || description.Size.H == 0 || choice.Size.H == 0 {
		t.Fatalf("the parts were not found: %v %v %v", title, description, choice)
	}
	if got := title.Origin.Y - bottom(progress); math.Abs(got-16) > .01 {
		t.Errorf("the title is %v under the progress, want 16", got)
	}
	if got := description.Origin.Y - bottom(title); math.Abs(got) > .01 {
		t.Errorf("the description is %v under the title, want 0", got)
	}
	if got := choice.Origin.Y - bottom(description); math.Abs(got-16) > .01 {
		t.Errorf("the choices are %v under the description, want 16", got)
	}
	_, title, _, choice = rects(Question{Name: "one", Title: "Choose a task", Choices: choices})
	if got := choice.Origin.Y - bottom(title); math.Abs(got-16) > .01 {
		t.Errorf("with no description the choices are %v under the title, want 16", got)
	}
}
