package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestJoinedPartsSplitOnlyWhereCornersDiffer(t *testing.T) {
	t.Parallel()
	r := ggui.Rct(ggui.Pt(10, 20), ggui.Sz(61, 32))
	if parts, n := joinedParts(r, 10, JoinedSides{}, 1); n != 1 || parts[0] != (joinedPart{r, 10}) {
		t.Fatalf("nothing joined: %d parts, %+v", n, parts[0])
	}
	if parts, n := joinedParts(r, 10, JoinedSides{Left: true, Right: true}, 1); n != 1 || parts[0] != (joinedPart{r, 0}) {
		t.Fatalf("both sides joined: %d parts, %+v", n, parts[0])
	}
	parts, n := joinedParts(r, 10, JoinedSides{Right: true}, 1)
	if n != 2 || parts[0].radius != 10 || parts[1].radius != 0 {
		t.Fatalf("right side joined: %d parts, %+v", n, parts)
	}
	// The cut lies on a whole pixel, and the halves cover r without sharing
	// any of it: a translucent fill drawn twice there would show as a seam.
	left, right := parts[0].clip, parts[1].clip
	if left.Origin != r.Origin || left.Size.W != 31 || right.Origin.X != 41 || right.Size.W != 30 || left.Size.H != 32 || right.Size.H != 32 {
		t.Fatalf("halves %+v and %+v do not split %+v on a pixel", left, right, r)
	}
	if parts, n := joinedParts(r, 10, JoinedSides{Top: true}, 2); n != 2 || parts[0].radius != 0 || parts[1].radius != 10 || parts[1].clip.Origin.Y != 36 {
		t.Fatalf("top side joined: %d parts, %+v", n, parts)
	}
	parts, n = joinedParts(r, 10, JoinedSides{Left: true, Top: true}, 1)
	if n != 4 || parts[0].radius != 0 || parts[1].radius != 0 || parts[2].radius != 0 || parts[3].radius != 10 {
		t.Fatalf("left and top joined: %d parts, %+v", n, parts)
	}
}

func TestButtonSizeRadiusGivesWayInAGroup(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	env := th.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 200))
	for _, tc := range []struct {
		size *ButtonSizeVariant
		want float64
	}{
		{ButtonExtraSmall, min(th.RadiusSm, 10)}, {ButtonIconExtraSmall, min(th.RadiusSm, 10)},
		{ButtonSmall, min(th.RadiusSm, 12)}, {ButtonIconSmall, min(th.RadiusSm, 12)},
		{ButtonDefault, th.Radius}, {ButtonLarge, th.Radius}, {ButtonIcon, th.Radius},
	} {
		b := Button("Go", nil).Size(tc.size)
		b.Layout(c, env)
		if b.corner != tc.want {
			t.Errorf("%s: radius %v, want %v", tc.size, b.corner, tc.want)
		}
		b.Layout(c, env.With(JoinedSidesKey, JoinedSides{Right: true}))
		if b.corner != th.Radius {
			t.Errorf("%s in a group: radius %v, want the theme's %v", tc.size, b.corner, th.Radius)
		}
		if b.Radius(3).Layout(c, env); b.corner != 3 {
			t.Errorf("%s: Radius(3) drew %v", tc.size, b.corner)
		}
	}
}

func TestButtonInlineIconsTakeTheSizesGapAndPadding(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	env := th.Apply(ggui.Env{})
	c := ggui.Loose(ggui.Sz(400, 200))
	for _, tc := range []struct {
		size                    *ButtonSizeVariant
		icon, gap, pad, iconPad float64
	}{
		{ButtonDefault, 16, 6, 10, 8}, {ButtonLarge, 16, 6, 10, 8}, {ButtonSmall, 14, 4, 10, 6}, {ButtonExtraSmall, 12, 4, 8, 6},
	} {
		plain := Button("Branch", nil).Size(tc.size).Layout(c, env)
		lead := Button("Branch", nil).Size(tc.size).Icon(Icon(icons.Plus))
		got := lead.Layout(c, env)
		if want := plain.W + tc.icon + tc.gap - (tc.pad - tc.iconPad); got.W != want || got.H != plain.H {
			t.Errorf("%s with an icon: %v, want width %v and height %v", tc.size, got, want, plain.H)
		}
		if lead.pad.Left != tc.iconPad || lead.pad.Right != tc.pad {
			t.Errorf("%s with an icon: padding %+v, want %v before and %v after", tc.size, lead.pad, tc.iconPad, tc.pad)
		}
		both := Button("Branch", nil).Size(tc.size).Icon(Icon(icons.Plus)).TrailingIcon(Icon(icons.ChevronDown)).Layout(c, env)
		if want := plain.W + 2*(tc.icon+tc.gap) - 2*(tc.pad-tc.iconPad); both.W != want {
			t.Errorf("%s with both icons: width %v, want %v", tc.size, both.W, want)
		}
	}
	// The narrow side follows the icon to the other edge when reading
	// right to left.
	rtl := Button("Branch", nil).Icon(Icon(icons.Plus))
	rtl.Layout(c, env.With(ggui.DirectionKey, ggui.DirRTL))
	if rtl.pad.Right != 8 || rtl.pad.Left != 10 {
		t.Errorf("right to left: padding %+v", rtl.pad)
	}
}

func TestToggleIsAtLeastSquareAndHasItsOwnVariants(t *testing.T) {
	t.Parallel()
	c := ggui.Loose(ggui.Sz(400, 200))
	for _, th := range []uitheme.Theme{uitheme.Default(), uitheme.Dark()} {
		env := th.Apply(ggui.Env{})
		for _, tc := range []struct {
			size *ButtonSizeVariant
			side float64
		}{{ButtonSmall, 28}, {ButtonDefault, 32}, {ButtonLarge, 36}} {
			if got := Toggle(ggui.State(false), "I").Size(tc.size).Layout(c, env); got != ggui.Sz(tc.side, tc.side) {
				t.Errorf("%s toggle with a short label: %v, want %v square", tc.size, got, tc.side)
			}
		}
		if got := ToggleOf(ggui.State(false), Icon(icons.Search)).Name("Search").Layout(c, env); got != ggui.Sz(36, 32) {
			t.Errorf("icon toggle: %v, want shadcn's px-2.5 around a 16px icon", got)
		}
		plain, outline := Toggle(ggui.State(false), "Bold"), Toggle(ggui.State(false), "Bold").Outline()
		plain.Layout(c, env)
		outline.Layout(c, env)
		if plain.style.Hover != th.Muted || plain.style.Fill != nil || plain.style.Border != nil {
			t.Errorf("toggle style %+v, want the whole Muted surface under the pointer and nothing at rest", plain.style)
		}
		if outline.style.Hover != th.Muted || outline.style.Fill != nil || outline.style.Border != th.InputBorder {
			t.Errorf("outline toggle style %+v, want the InputBorder around nothing", outline.style)
		}
	}
}

func TestDestructiveButtonFocusBorderIsTheSameInBothModes(t *testing.T) {
	t.Parallel()
	for _, th := range []uitheme.Theme{uitheme.Default(), uitheme.Dark()} {
		if got, want := ButtonDestructive.Style(th).FocusBorder, fade(th.Destructive, .4); got != want {
			t.Errorf("focus border %v, want %v", got, want)
		}
	}
}

// withinProbe paints as a control would and keeps the sides it may draw
// joined.
type withinProbe struct {
	laid, painted JoinedSides
}

func (w *withinProbe) Layout(c ggui.Constraints, env ggui.Env) ggui.Size {
	w.laid = joinedSidesOf(env)
	return c.Constrain(ggui.Sz(20, 20))
}
func (w *withinProbe) Paint(dst *ggui.Canvas, _ ggui.Rect) { w.painted = w.laid.within(dst) }

// A popup is laid out under the environment of the child that opened it,
// so what is inside reads the child's sides; it is not painted by the
// group, and must not draw them.
func TestJoinedSidesHoldOnlyWhereTheGroupPaints(t *testing.T) {
	t.Parallel()
	inside, outside := &withinProbe{}, &withinProbe{}
	leaked := ggui.WithEnv(func(env ggui.Env) ggui.Env { return env.With(JoinedSidesKey, JoinedSides{Left: true}) }, outside)
	p := ggui.NewProbe(ggui.Column(ButtonGroup(Button("a", nil), inside), leaked), ggui.Sz(300, 200))
	defer p.Close()
	p.Frame()
	if want := (JoinedSides{Left: true}); inside.painted != want {
		t.Errorf("a child of the group draws %+v joined, want %+v", inside.painted, want)
	}
	if !outside.laid.Any() || outside.painted.Any() {
		t.Errorf("outside a group: laid out under %+v, draws %+v joined", outside.laid, outside.painted)
	}
}

func TestButtonGroupDrawsTheFocusedChildsRingItself(t *testing.T) {
	t.Parallel()
	g := ButtonGroup(Button("One", nil).Outline(), Button("Two", nil).Outline(), Button("Three", nil).Outline())
	alone := Button("Alone", nil)
	p := ggui.NewProbe(ggui.Column(g, alone), ggui.Sz(400, 200))
	defer p.Close()
	p.Frame()
	if len(g.rings) != 0 {
		t.Fatal("a ring with nothing focused")
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyTab)
	// The ring eases in, so it is looked for part of the way there on the
	// probe's own clock: two frames of the wall clock can read the same
	// time where the timer is coarse, and the ring is then not begun.
	p.Advance(0)
	p.Advance(uitheme.Default().MotionFast / 2)
	two, ok := p.FindRole(ggui.RoleButton, "Two")
	if !ok {
		t.Fatal("no Two")
	}
	ring, ok := g.ringAt(two.Rect)
	if want := (JoinedSides{Left: true, Right: true}); !ok || ring.sides != want {
		t.Fatalf("ring %+v, want one around Two at %+v joined on both sides", ring, two.Rect)
	}
	p.Advance(time.Second)
	p.Advance(0)
	if len(g.rings) != 1 {
		t.Fatalf("%d rings once focus has settled on Two, want its own alone", len(g.rings))
	}
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyTab)
	p.Advance(0) // the frame the ring starts to go in
	p.Advance(time.Second)
	p.Advance(0)
	if len(g.rings) != 0 {
		t.Fatal("the ring stayed after focus left the group")
	}
}

// A joined button's ring eases out as it eased in: shadcn's button is
// transition-all in a group as out of one. The group draws the ring of the
// button that lost the focus, fading, beside the one coming in.
func TestButtonGroupRingEasesOut(t *testing.T) {
	t.Parallel()
	g := ButtonGroup(Button("One", nil).Outline(), Button("Two", nil).Outline())
	p := ggui.NewProbe(ggui.Column(g).Align(ggui.AlignStart), ggui.Sz(400, 120))
	defer p.Close()
	th := uitheme.Default()
	alpha := func(c color.Color) uint32 { _, _, _, a := c.RGBA(); return a }
	p.Key("tab")
	p.Advance(0)
	p.Advance(time.Second)
	p.Advance(0)
	one, _ := p.FindRole(ggui.RoleButton, "One")
	two, _ := p.FindRole(ggui.RoleButton, "Two")
	full, ok := g.ringAt(one.Rect)
	if !ok || len(g.rings) != 1 {
		t.Fatalf("%d rings with One focused, want its own", len(g.rings))
	}
	p.Key("tab")
	p.Advance(0)
	p.Advance(th.MotionFast / 2)
	out, leaving := g.ringAt(one.Rect)
	in, arriving := g.ringAt(two.Rect)
	if !leaving || !arriving {
		t.Fatalf("half way through MotionFast the group draws %d rings, want One's going and Two's coming", len(g.rings))
	}
	if a := alpha(out.line); a == 0 || a >= alpha(full.line) {
		t.Fatalf("One's ring is at %v of %v half way out, want between", a, alpha(full.line))
	}
	if a := alpha(in.line); a == 0 || a >= alpha(full.line) {
		t.Fatalf("Two's ring is at %v of %v half way in, want between", a, alpha(full.line))
	}
	p.Advance(time.Second)
	p.Advance(0)
	if _, still := g.ringAt(one.Rect); still || len(g.rings) != 1 {
		t.Fatalf("%d rings after the motion, want Two's alone", len(g.rings))
	}
}

// A Select in a ButtonGroup is joined as a button is: it squares the side
// it meets its neighbour on, and its keyboard ring is the group's to draw.
func TestSelectJoinsAButtonGroup(t *testing.T) {
	t.Parallel()
	unit := Select(ggui.State("USD")).Options([]string{"USD", "EUR"}).Name("Currency")
	g := ButtonGroup(unit, Button("Convert", nil).Outline())
	alone := Select(ggui.State("USD")).Options([]string{"USD", "EUR"}).Name("Alone")
	p := ggui.NewProbe(ggui.Column(g, alone).Gap(8), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	if want := (JoinedSides{Right: true}); unit.sides != want {
		t.Fatalf("the select in the group is joined on %+v, want %+v", unit.sides, want)
	}
	if alone.sides.Any() {
		t.Fatalf("a select outside a group is joined on %+v", alone.sides)
	}
	field, _ := p.FindRole(ggui.RoleSelect, "Currency")
	button, _ := p.FindRole(ggui.RoleButton, "Convert")
	if field.Rect.Size.H != button.Rect.Size.H || field.Rect.Origin.X+field.Rect.Size.W != button.Rect.Origin.X {
		t.Fatalf("select %v and button %v do not meet at one height", field.Rect, button.Rect)
	}
	if len(g.rings) != 0 {
		t.Fatal("a ring with nothing focused")
	}
	p.Key("tab")
	p.Frame()
	if ring, ok := g.ringAt(field.Rect); !ok || ring.sides != unit.sides {
		t.Fatalf("ring %+v, want the group to draw one around the select at %v", ring, field.Rect)
	}
	// The list it opens is not part of the strip.
	p.Key("space")
	p.Frame()
	if _, ok := p.FindRole(ggui.RoleOption, "EUR"); !ok {
		t.Fatal("the joined select did not open its list")
	}
	p.Key("escape", "tab")
	p.Frame()
	if _, ok := g.ringAt(field.Rect); ok {
		t.Fatal("the select's ring stayed after focus moved on")
	}
}
