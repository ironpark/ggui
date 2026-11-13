package ui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestCardSectionsStackAndAFooterRunsToTheBottom(t *testing.T) {
	t.Parallel()
	theme := uitheme.Default()
	env := theme.Apply(ggui.Env{})
	cs := ggui.Loose(ggui.Sz(360, 600))
	block := func() ggui.Widget { return ggui.Box().Height(20) }

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
		return ggui.Text(s).NoWrap().Style(theme.Text).Layout(ggui.Loose(ggui.Sz(300, 100)), env)
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
