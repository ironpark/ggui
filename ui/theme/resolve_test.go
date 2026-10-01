package theme

import (
	"image/color"
	"testing"
)

var brand = color.NRGBA{0x25, 0x63, 0xeb, 0xff}

func TestFollowersFollowAChangedSource(t *testing.T) {
	t.Parallel()
	// A preset sets its whole palette and its radii, so only the tokens it
	// leaves unset follow there; see TestASetFollowerStaysSet.
	for name, base := range map[string]Theme{"light": Default(), "dark": Dark(), "preset": Preset{Accent: AccentBlue}.Light()} {
		preset := name == "preset"
		th := base
		th.Primary, th.Fg, th.Radius = brand, color.NRGBA{1, 2, 3, 255}, 4
		th = th.Resolve()
		if !sameColor(th.PrimaryHover, compositeColor(brand, th.Bg, .9)) || !sameColor(th.Selection, compositeColor(brand, th.Bg, .25)) {
			t.Errorf("%s: PrimaryHover %v and Selection %v did not follow Primary", name, th.PrimaryHover, th.Selection)
		}
		if th.Text.Color != th.Fg || !preset && (th.AccentFg != th.Fg || th.SidebarPrimary != th.Primary) {
			t.Errorf("%s: Text.Color %v, AccentFg %v or SidebarPrimary %v did not follow", name, th.Text.Color, th.AccentFg, th.SidebarPrimary)
		}
		if !preset && (th.RadiusSm != 3 || th.RadiusLg != 6) {
			t.Errorf("%s: radii %v/%v did not follow Radius 4", name, th.RadiusSm, th.RadiusLg)
		}
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
	if th.Primary != color.Color(brand) || !sameColor(th.PrimaryHover, compositeColor(brand, th.Bg, .9)) {
		t.Fatalf("the literal's own Primary was lost or not followed: %v, %v", th.Primary, th.PrimaryHover)
	}
	for i, f := range followers {
		if f.field == nil || f.from == nil {
			t.Fatalf("follower %d is empty", i)
		}
	}
}
