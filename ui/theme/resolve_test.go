package theme

import (
	"image/color"
	"testing"
	"time"

	"github.com/ironpark/ggui"
)

var brand = color.NRGBA{0x25, 0x63, 0xeb, 0xff}

func TestFollowersFollowAChangedSource(t *testing.T) {
	t.Parallel()
	// A preset sets its whole palette, so only the colors it leaves unset
	// follow there; see TestASetFollowerStaysSet. Default and Dark have the
	// same colors, and follow wherever shadcn's value is the one followed:
	// the card and popover text is the page's, the sidebar's border the
	// page's. Radii follow Radius in all three.
	for name, base := range map[string]Theme{"light": Default(), "dark": Dark(), "preset": Preset{Accent: AccentBlue}.Light()} {
		preset := name == "preset"
		th := base
		border := color.NRGBA{4, 5, 6, 255}
		th.Primary, th.Fg, th.Border, th.Radius = brand, color.NRGBA{1, 2, 3, 255}, border, 4
		th = th.Resolve()
		if !sameColor(th.PrimaryHover, compositeColor(brand, th.Bg, .8)) || !sameColor(th.Selection, compositeColor(brand, th.Bg, .25)) {
			t.Errorf("%s: PrimaryHover %v and Selection %v did not follow Primary", name, th.PrimaryHover, th.Selection)
		}
		if th.Text.Color != th.Fg || !preset && (th.CardFg != th.Fg || th.PopoverFg != th.Fg || th.SidebarFg != th.Fg) {
			t.Errorf("%s: Text.Color %v, CardFg %v, PopoverFg %v or SidebarFg %v did not follow", name, th.Text.Color, th.CardFg, th.PopoverFg, th.SidebarFg)
		}
		if name == "light" && (th.InputBorder != color.Color(border) || th.SidebarBorder != color.Color(border) || th.SidebarPrimary != th.Primary) {
			t.Errorf("light: InputBorder %v, SidebarBorder %v or SidebarPrimary %v did not follow", th.InputBorder, th.SidebarBorder, th.SidebarPrimary)
		}
		if th.RadiusXs != 2.4 || th.RadiusSm != 3.2 || th.RadiusLg != 5.6 || th.RadiusXl != 7.2 {
			t.Errorf("%s: radii %v/%v/%v/%v did not follow Radius 4", name, th.RadiusXs, th.RadiusSm, th.RadiusLg, th.RadiusXl)
		}
		if c := th.Chat; c.BubbleRadius != 5.6 || c.AttachmentRadius != 5.6 || c.AttachmentXSRadius != 4 || c.QuestionRadius != 4 || c.QuestionInputRadius != 4 {
			t.Errorf("%s: chat corners %+v did not follow Radius 4", name, c)
		}
	}
}

// Default and Dark are shadcn's base-nova: the zero Preset token for token,
// differing only in which of them follow.
func TestStockThemesAreTheZeroPreset(t *testing.T) {
	t.Parallel()
	for name, pair := range map[string][2]Theme{"light": {Default(), Preset{}.Light()}, "dark": {Dark(), Preset{}.Dark()}} {
		stock, preset := pair[0], pair[1]
		stock.auto, preset.auto = followed{}, followed{}
		if stock != preset {
			t.Errorf("%s: the stock theme differs from Preset{}:\n%+v\n%+v", name, stock, preset)
		}
	}
	// The values the audit found the old stock themes to have wrong.
	light, dark := Default(), Dark()
	if !sameColor(light.AccentFg, color.NRGBA{0x17, 0x17, 0x17, 255}) || !sameColor(light.SecondaryFg, color.NRGBA{0x17, 0x17, 0x17, 255}) {
		t.Errorf("light AccentFg %v, SecondaryFg %v: want shadcn's #171717", light.AccentFg, light.SecondaryFg)
	}
	if !sameColor(dark.SidebarPrimary, color.NRGBA{0x14, 0x47, 0xe6, 255}) {
		t.Errorf("dark SidebarPrimary %v: want shadcn's #1447e6", dark.SidebarPrimary)
	}
	if c := light.Chat; c.BubbleRadius != 14 || c.AttachmentRadius != 14 || c.AttachmentXSRadius != 10 || c.QuestionRadius != 10 || c.BubblePadding.Top != 9 {
		t.Errorf("stock chat geometry %+v: want Nova's rounded-xl bubbles", c)
	}
	if light.Chart[0] == nil || dark.Chart[4] == nil {
		t.Error("the stock themes have no chart colors")
	}
	if s := dark.OverlayShadow; s.Offset.Y != 10 || s.Blur != 15 || s.Spread != -3 || dark.MotionDrawer != 450*time.Millisecond {
		t.Errorf("overlay shadow %+v and drawer motion %v: want shadow-lg and 450ms", s, dark.MotionDrawer)
	}
	// Tailwind v4's shadow-md and shadow-lg, two layers each at 10% black.
	layer := func(y, blur, spread float64) ggui.ShadowStyle {
		return ggui.ShadowStyle{Offset: ggui.Pt(0, y), Blur: blur, Spread: spread, Color: color.NRGBA{A: 26}}
	}
	for _, th := range []Theme{light, dark} {
		if th.PanelShadow != layer(4, 6, -1) || th.PanelContact != layer(2, 4, -2) {
			t.Errorf("panel shadow %+v over %+v: want shadow-md's 0 4px 6px -1px and 0 2px 4px -2px", th.PanelShadow, th.PanelContact)
		}
		if th.OverlayShadow != layer(10, 15, -3) || th.OverlayContact != layer(4, 6, -4) {
			t.Errorf("overlay shadow %+v over %+v: want shadow-lg's 0 10px 15px -3px and 0 4px 6px -4px", th.OverlayShadow, th.OverlayContact)
		}
	}
}

// A field paints over the surface it sits on, as shadcn's bg-transparent
// dark:bg-input/30 does.
func TestInputSurfaceIsTranslucent(t *testing.T) {
	t.Parallel()
	if _, _, _, a := Default().Input.RGBA(); a != 0 {
		t.Errorf("light Input %v is not clear", Default().Input)
	}
	dark := Dark()
	want := color.NRGBAModel.Convert(dark.InputBorder).(color.NRGBA)
	if got := color.NRGBAModel.Convert(dark.Input).(color.NRGBA); got.A == 0 || got.A >= want.A || got.R != want.R {
		t.Errorf("dark Input %v is not InputBorder %v at 30%%", got, want)
	}
	pinned := dark
	pinned.Input = dark.Card
	if pinned.Resolve().Input != dark.Card {
		t.Error("a set Input did not stay set")
	}
	// Maia's fields are bg-input/30 in light too, Luma's bg-input/50.
	for style, alpha := range map[Style]uint8{StyleMaia: 77, StyleLuma: 128, StyleMira: 51} {
		light := Preset{Style: style}.Light()
		border := color.NRGBAModel.Convert(light.InputBorder).(color.NRGBA)
		got := color.NRGBAModel.Convert(light.Input).(color.NRGBA)
		if got.A != uint8((int(border.A)*int(alpha)+127)/255) || got.R != border.R {
			t.Errorf("%s light Input %v, want InputBorder %v at alpha %d/255", style, got, border, alpha)
		}
	}
}

// Every corner is a step of --radius in the proportions of the style, so
// setting Radius on a copy, in CSS or in an Override restyles them all.
func TestRadiiFollowRadiusInEveryStyle(t *testing.T) {
	t.Parallel()
	for _, style := range Styles() {
		th := Preset{Style: style}.Light()
		steps := styles[style].radii
		if th.Radius != snap(10*steps.control) {
			t.Errorf("%s: Radius %v is not its step of 10", style, th.Radius)
		}
		css, err := th.ApplyCSS("--radius: 1.25rem")
		if err != nil {
			t.Fatal(err)
		}
		set := th
		set.Radius = css.Radius
		set = set.Resolve()
		for i, p := range css.radii() {
			want := snap(20 * steps.follow[i])
			if i == stepLg && steps.lgMax != 0 {
				want = min(want, steps.lgMax)
			}
			if *p != want || *set.radii()[i] != want {
				t.Errorf("%s: radius %d is %v from CSS and %v from Radius, want %v", style, i, *p, *set.radii()[i], want)
			}
		}
		if css.Resolve() != css {
			t.Errorf("%s: resolving again changed the radii", style)
		}
	}
	lyra := Preset{Style: StyleLyra}.Light()
	if lyra.Radius != 0 || lyra.RadiusLg != 0 || lyra.Chat.BubbleRadius != 0 || lyra.Text.Size != 12 {
		t.Errorf("Lyra is not square in small text: %v/%v/%v at %v", lyra.Radius, lyra.RadiusLg, lyra.Chat.BubbleRadius, lyra.Text.Size)
	}
	if maia := (Preset{Style: StyleMaia}).Light(); maia.Radius != 18 || maia.RadiusSm != 14 || maia.RadiusLg != 18 || maia.Chat.BubbleRadius != 22 {
		t.Errorf("Maia radii %v/%v/%v/%v, want 18, 14, 18 and 22", maia.Radius, maia.RadiusSm, maia.RadiusLg, maia.Chat.BubbleRadius)
	}
	if mira := (Preset{Style: StyleMira}).Light(); mira.Radius != 8 || mira.RadiusLg != 10 || mira.Text.Size != 12 || mira.ButtonPad.Top != 4 {
		t.Errorf("Mira is not compact: radii %v/%v, text %v, button %+v", mira.Radius, mira.RadiusLg, mira.Text.Size, mira.ButtonPad)
	}
	if rhea := (Preset{Style: StyleRhea}).Light(); rhea.Radius != 18 || rhea.RadiusLg != 24 || rhea.Chat.BubbleRadius != 22 {
		t.Errorf("Rhea radii %v/%v/%v, want 18, a capped 24 and 22", rhea.Radius, rhea.RadiusLg, rhea.Chat.BubbleRadius)
	}
	pinned := Default()
	pinned.Chat.BubbleRadius, pinned.Radius = 24, 6
	if pinned = pinned.Resolve(); pinned.Chat.BubbleRadius != 24 || pinned.Chat.AttachmentRadius != 8.4 {
		t.Errorf("a set chat corner followed, or an unset one did not: %+v", pinned.Chat)
	}
}

func TestASetFollowerStaysSet(t *testing.T) {
	t.Parallel()
	th := Default()
	th.PrimaryHover, th.RadiusLg = color.NRGBA{9, 9, 9, 255}, 20
	th.Primary, th.Radius = brand, 2
	th = th.Resolve()
	if th.PrimaryHover != (color.NRGBA{9, 9, 9, 255}) || th.RadiusLg != 20 {
		t.Fatalf("a set token followed its source: PrimaryHover %v, RadiusLg %v", th.PrimaryHover, th.RadiusLg)
	}
	// A preset's own palette is set, not followed.
	p := Preset{Base: BaseStone}.Light()
	card := p.Card
	p.Bg = brand
	if p.Resolve().Card != card {
		t.Fatal("a preset's Card followed Bg")
	}
}

func TestResolveIsStable(t *testing.T) {
	t.Parallel()
	for _, th := range []Theme{Default(), Dark(), Preset{Style: StyleRhea}.Dark()} {
		if again := th.Resolve(); again != th {
			t.Fatalf("resolving a resolved theme changed it:\n%+v\n%+v", th, again)
		}
	}
}

func TestALiteralThemeDraws(t *testing.T) {
	t.Parallel()
	th := Theme{Primary: brand}.Resolve()
	if th.Bg == nil || th.Fg == nil || th.Ring == nil || th.Popover == nil || th.SidebarRing == nil || th.Text.Color == nil {
		t.Fatalf("unset colors stayed unset: %+v", th)
	}
	if th.ControlSize == 0 || th.IconSize == 0 || th.MenuWidth == 0 || th.Chat == (ChatTokens{}) || th.Mono.Font == nil {
		t.Fatalf("unset sizes stayed zero: %+v", th)
	}
	if th.Primary != color.Color(brand) || !sameColor(th.PrimaryHover, compositeColor(brand, th.Bg, .8)) {
		t.Fatalf("the literal's own Primary was lost or not followed: %v, %v", th.Primary, th.PrimaryHover)
	}
	for i, f := range followers {
		if f.field == nil || f.from == nil {
			t.Fatalf("follower %d is empty", i)
		}
	}
}
