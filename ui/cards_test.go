package ui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
	"github.com/ironpark/ggui/widget"
)

func TestCardSectionsStackAndAFooterRunsToTheBottom(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(360, 600))
	block := func() ggui.Widget { return widget.Box().Height(20) }

	// One child keeps the old look: CardPad on every side.
	single := Card(block()).Layout(cs, env)
	if want := 20 + theme.CardPad.Top + theme.CardPad.Bottom; single.H != want {
		t.Fatalf("a single-child card is %v tall, want %v", single.H, want)
	}

	plain := Card(CardContent(block()), CardContent(block())).Layout(cs, env)
	if want := 40 + theme.CardPad.Top*2 + theme.CardPad.Bottom; plain.H != want {
		t.Fatalf("two sections are %v tall, want %v: padding above and below and a CardPad between", plain.H, want)
	}
	footed := Card(CardContent(block()), CardFooter(block())).Layout(cs, env)
	// The footer pads itself and takes the place of the card's bottom
	// padding, leaving only the border under it.
	if want := 40 + theme.CardPad.Top*3 + theme.CardPad.Bottom + theme.BorderWidth; footed.H != want {
		t.Fatalf("a card with a footer is %v tall, want %v", footed.H, want)
	}
	small := Card(CardContent(block()), CardContent(block())).Small().Layout(cs, env)
	if small.H >= plain.H {
		t.Fatalf("a small card is not tighter: %v >= %v", small.H, plain.H)
	}
	// A child that is not a section is laid out as CardContent.
	if mixed := Card(CardHeader("Title", ""), block()).Layout(cs, env); mixed.H <= 20 {
		t.Fatalf("a bare child in a sectioned card was lost: %v", mixed)
	}
}

func TestCardHeaderTitleIsMediumAndShrinksInASmallCard(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(360, 600))
	h := CardHeader("Team", "Invite people")
	Card(h).Layout(cs, env)
	big := h.title.Layout(cs, env)
	h2 := CardHeader("Team", "Invite people")
	Card(h2).Small().Layout(cs, env)
	if got := h2.title.Layout(cs, env); got.H >= big.H {
		t.Fatalf("a small card's title is not smaller: %v >= %v", got.H, big.H)
	}
	withAction := CardHeader("Team", "Invite people").Action(Button("Add", nil))
	if got, bare := withAction.Layout(cs, env), CardHeader("Team", "Invite people").Layout(cs, env); got.H < bare.H {
		t.Fatalf("an action shrank the header: %v < %v", got, bare)
	}
}

func TestAvatarSizesAndGroupOverlap(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(400, 400))
	for _, c := range []struct {
		a    *AvatarWidget
		want float64
	}{{Avatar("Ada"), 32}, {Avatar("Ada").Small(), 24}, {Avatar("Ada").Large(), 40}} {
		if got := c.a.Layout(cs, env); got != ggui.Sz(c.want, c.want) {
			t.Fatalf("avatar is %v, want %v", got, c.want)
		}
	}
	g := AvatarGroup(Avatar("A"), Avatar("B"), Avatar("C"), Avatar("D")).Max(2)
	// Two avatars and a "+2" bubble, each overlapping the one before by a
	// Space.
	if got, want := g.Layout(cs, env).W, 3*32-2*theme.Space; got != want {
		t.Fatalf("group is %v wide, want %v", got, want)
	}
	if len(g.shown) != 3 || g.counted != 2 {
		t.Fatalf("group shows %d with a count of %d, want 3 and 2", len(g.shown), g.counted)
	}
	if got := AvatarGroup(Avatar("A"), Avatar("B")).Count(5).Layout(cs, env).W; got != 3*32-2*theme.Space {
		t.Fatalf("Count did not add a bubble: %v", got)
	}
}

func TestItemLooksLineUpAndExtraSmallIsTighter(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(360, 400))
	bare := Item("Title", "Description").Layout(cs, env)
	if got := Item("Title", "Description").Outline().Layout(cs, env); got != bare {
		t.Fatalf("an outlined item is %v, a bare one %v", got, bare)
	}
	if got := Item("Title", "Description").Muted().Layout(cs, env); got != bare {
		t.Fatalf("a muted item is %v, a bare one %v", got, bare)
	}
	if got := Item("Title", "Description").ExtraSmall().Layout(cs, env); got.H >= bare.H {
		t.Fatalf("an extra-small item is not shorter: %v >= %v", got.H, bare.H)
	}
	long := "A description long enough to need more than two lines at this width, which an item cuts off so a list keeps an even rhythm."
	if got := Item("Title", long).Layout(ggui.Loose(ggui.Sz(200, 400)), env); got.H > bare.H*2 {
		t.Fatalf("a long description was not held to two lines: %v", got.H)
	}
}

func TestEmptyHasNoBorderUntilOutlineAndHoldsItsMeasure(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(900, 600))
	e := Empty("Nothing here", "A description long enough to run past the measure of an empty state when nothing holds it back from the panel's edges.")
	e.Layout(cs, env)
	if e.outline {
		t.Fatal("an empty state is outlined by default")
	}
	if w := e.text.Layout(cs, env).W; w > emptyMeasure {
		t.Fatalf("text runs %v wide, past the %v measure", w, emptyMeasure)
	}
	if got, want := Empty("A", "B").Outline().Layout(cs, env), Empty("A", "B").Layout(cs, env); got != want {
		t.Fatalf("Outline changed the size: %v, want %v", got, want)
	}
}

func TestAlertIconLeadsAndActionHangsTopRight(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(400, 400))
	a := Alert("Saved", "").Icon(icons.Check)
	a.Layout(cs, env)
	if a.icon == nil {
		t.Fatal("an icon was not built")
	}
	if got := a.Icon("").Layout(cs, env); a.icon != nil || got.H <= 0 {
		t.Fatalf("an empty role kept the icon: %v", got)
	}
	b := Alert("Saved", "").Action(Button("Undo", nil))
	size := b.Layout(cs, env)
	if size.H < b.actionSize.H+2*b.inset {
		t.Fatalf("alert is %v tall, the action needs %v", size.H, b.actionSize.H+2*b.inset)
	}
	if b.actionSize.W <= 0 || b.column == nil {
		t.Fatal("the action was laid out in the column")
	}
}

func TestBadgeIconsWidenItAndGhostHasNoSurface(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(400, 100))
	bare := Badge("New").Layout(cs, env)
	lead := Badge("New").Icon(icons.Check).Layout(cs, env)
	if lead.H != bare.H || lead.W <= bare.W {
		t.Fatalf("a badge with an icon is %v, without %v", lead, bare)
	}
	if both := Badge("New").Icon(icons.Check).TrailingIcon(icons.ChevronRight).Layout(cs, env); both.W <= lead.W {
		t.Fatalf("a trailing icon did not widen the badge: %v <= %v", both.W, lead.W)
	}
	for _, v := range []*BadgeVariant{BadgeGhost, BadgeLink} {
		if s := v.Style(theme); s.Fill != nil || s.Border != nil {
			t.Fatalf("%v draws a surface: %+v", v, s)
		}
	}
}

func TestProgressLabelAndValueSitAboveTheBar(t *testing.T) {
	t.Parallel()
	env := uitheme.Default().Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(300, 100))
	value := ggui.State(.42)
	bar := Progress(value).Layout(cs, env)
	if bar.H != 4 {
		t.Fatalf("a bare bar is %v tall", bar.H)
	}
	p := Progress(value).Label("Upload").ShowValue()
	if got := p.Layout(cs, env); got.H <= bar.H+p.gap || p.gap <= 0 {
		t.Fatalf("label and value took no room: %v", got)
	}
	theme := uitheme.Of(env)
	reads := func(s string) ggui.Size {
		return widget.Text(s).NoWrap().Style(theme.Text).Layout(ggui.Loose(ggui.Sz(300, 100)), env)
	}
	if got, want := p.readSize, reads("42%"); got != want {
		t.Fatalf("value is %v, want the size of 42%%, %v", got, want)
	}
	f := Progress(value).FormatValue(func(v float64) string { return "5 of 12" })
	f.Layout(cs, env)
	if got, want := f.readSize, reads("5 of 12"); got != want {
		t.Fatalf("formatted value is %v, want the size of \"5 of 12\", %v", got, want)
	}
	if Progress(value).Label("").label != nil {
		t.Fatal("an empty label was kept")
	}
}

// A card given OnTap is a button named by Name: it tints toward Muted under
// the pointer, eased as an Item does, shows the pointer cursor, and runs
// on a click or on Enter and Space, while a button inside it keeps its own
// click. A card without OnTap is still a group that takes no input.
func TestCardWithOnTapIsAButton(t *testing.T) {
	t.Parallel()
	taps, runs := 0, 0
	plain := Card(widget.Box().Height(40)).Name("Notes")
	tappable := Card(widget.Column(widget.Box().Height(40), Button("Run", func() { runs++ }))).Name("Project").OnTap(func() { taps++ })
	a, b := &placed{Widget: plain}, &placed{Widget: tappable}
	p := ggui.NewProbe(widget.Column(a, b).Gap(10).Align(widget.AlignStretch), ggui.Sz(320, 400))
	defer p.Close()
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Notes"); !ok {
		t.Fatalf("a card without OnTap is not a group:\n%s", p.Semantics())
	}
	p.Move(center(a.at))
	p.Frame()
	if p.Cursor() == ggui.CursorShapePointer || plain.Hovered {
		t.Fatal("a card without OnTap took the pointer")
	}
	if f, ok := p.FindRole(ggui.RoleButton, "Project"); !ok || f.Rect != b.at {
		t.Fatalf("the card is not a button named by Name: %v %v\n%s", f, ok, p.Semantics())
	}
	at := ggui.Pt(b.at.Origin.X+20, b.at.Origin.Y+30)
	p.Move(at)
	p.Frame()
	p.Advance(tappable.theme.MotionFast / 3)
	p.Frame()
	if k := tappable.hover; !(k > 0 && k < 1) || p.Cursor() != ggui.CursorShapePointer {
		t.Fatalf("50ms in the card is %v onto its hover tint with cursor %v, want it easing under a pointer cursor", k, p.Cursor())
	}
	p.Advance(tappable.theme.MotionFast)
	p.Frame()
	if tappable.hover != 1 {
		t.Fatalf("the settled card is %v onto its hover tint", tappable.hover)
	}
	p.Click(at)
	p.Tap("Run")
	if taps != 1 || runs != 1 {
		t.Fatalf("a click on the card and one on its button ran the card %d times and the button %d, want 1 and 1", taps, runs)
	}
	p.Move(ggui.Pt(1, 399))
	p.Type(ggui.Mods{}, ggui.KeyTab)
	p.Frame()
	if !tappable.Focused || !tappable.FocusVisible {
		t.Fatal("Tab did not bring the focus to the card")
	}
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	p.Type(ggui.Mods{}, ggui.KeySpace)
	if taps != 3 {
		t.Fatalf("Enter and Space on the focused card ran it %d times in all, want 3", taps)
	}
	tappable.OnTap(nil)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleGroup, "Project"); !ok {
		t.Fatal("a card whose OnTap was taken away is not a group again")
	}
}

// Clip keeps a card's content inside its border, so a full-bleed table's
// last row cannot paint over the rounded corners.
func TestCardClipKeepsContentInsideTheBorder(t *testing.T) {
	t.Parallel()
	var clip ggui.Rect
	var clipped bool
	content := ggui.FromFuncs(
		func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(200, 60)) },
		func(dst *ggui.Canvas, _ ggui.Rect) { clip, clipped = dst.ClipBounds() },
	)
	card := Card(content).Pad(0).Border(2, color.Black)
	b := &placed{Widget: card}
	p := ggui.NewProbe(widget.Align(b).At(0, 0), ggui.Sz(320, 200))
	defer p.Close()
	p.Frame()
	inside := ggui.Rct(ggui.Pt(2, 2), ggui.Sz(b.at.Size.W-4, b.at.Size.H-4))
	if clipped && clip == inside {
		t.Fatal("a card without Clip clipped its content")
	}
	card.Clip()
	p.Frame()
	if !clipped || clip != inside {
		t.Fatalf("content clipped to %v (%v), want the inside of the border %v", clip, clipped, inside)
	}
}

func TestSpinnerIsIconSizedInTheTextColor(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(100, 100))
	s := Spinner()
	if got := s.Layout(cs, env); got != ggui.Sz(theme.IconSize, theme.IconSize) {
		t.Fatalf("spinner is %v, want the icon size", got)
	}
	red := color.NRGBA{R: 200, A: 255}
	s.Layout(cs, env.WithText(ggui.TextStyle{Color: red}))
	if s.ink != color.Color(red) {
		t.Fatalf("spinner is %v, not the text color around it", s.ink)
	}
	s.Color(theme.Destructive).Layout(cs, env)
	if s.ink != ggui.ResolveColor(theme.Destructive, env) {
		t.Fatalf("Color did not apply: %v", s.ink)
	}
}
