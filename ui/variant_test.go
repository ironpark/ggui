package ui

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui/icons"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

var testBrand = color.NRGBA{0x25, 0x63, 0xeb, 0xff}

func TestRestyleChangesAVariantUnderItsTheme(t *testing.T) {
	t.Parallel()
	th := ButtonPrimary.Restyle(uitheme.Default(), func(t uitheme.Theme) ButtonStyle {
		s := ButtonPrimary.Base(t)
		s.Fill, s.Shadow = testBrand, false
		return s
	})
	loose := ggui.Loose(ggui.Sz(300, 300))
	restyled, plain, outline := Button("a", nil), Button("b", nil), Button("c", nil).Outline()
	restyled.Layout(loose, th.Apply(ggui.Env{}))
	plain.Layout(loose, ggui.Env{})
	outline.Layout(loose, th.Apply(ggui.Env{}))
	if restyled.style.Fill != color.Color(testBrand) || restyled.style.Shadow {
		t.Fatalf("restyled primary = %+v, want the brand fill and no shadow", restyled.style)
	}
	if restyled.style.Label != th.Resolve().PrimaryFg {
		t.Fatalf("restyled primary label = %v, want Base's PrimaryFg", restyled.style.Label)
	}
	if plain.style.Fill == color.Color(testBrand) || outline.style.Fill == color.Color(testBrand) {
		t.Fatal("Restyle reached a button under another theme or of another variant")
	}
}

func TestACustomVariantDrawsAButton(t *testing.T) {
	t.Parallel()
	brand := NewVariant("brand", func(t uitheme.Theme) ButtonStyle {
		return ButtonStyle{Fill: testBrand, Hover: t.Hovered(testBrand), Label: t.PrimaryFg}
	})
	b := Button("go", nil).Variant(brand)
	b.Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
	if b.style.Fill != color.Color(testBrand) || brand.String() != "brand" {
		t.Fatalf("custom variant %q drew %+v", brand, b.style)
	}
}

func TestButtonSizes(t *testing.T) {
	t.Parallel()
	loose := ggui.Loose(ggui.Sz(300, 300))
	normal := Button("go", nil).Layout(loose, ggui.Env{})
	extraSmall := Button("go", nil).Size(ButtonExtraSmall).Layout(loose, ggui.Env{})
	small := Button("go", nil).Size(ButtonSmall).Layout(loose, ggui.Env{})
	large := Button("go", nil).Size(ButtonLarge).Layout(loose, ggui.Env{})
	if !(extraSmall.W < small.W && extraSmall.H < small.H && small.W < normal.W && small.H < normal.H && large.W >= normal.W && large.H > normal.H) {
		t.Fatalf("sizes small %v, default %v, large %v: want them in order", small, normal, large)
	}
	icon := ButtonOf(Icon("x"), nil).Name("x").Size(ButtonIcon).Layout(loose, ggui.Env{})
	if icon.W != icon.H {
		t.Fatalf("an icon button is %v, want a square", icon)
	}
	b := Button("go", nil).Radius(2)
	b.Layout(loose, ggui.Env{})
	if b.radius != 2 {
		t.Fatalf("Radius(2) left %v", b.radius)
	}
}

func TestButtonVariantsReadTheirTokens(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	if s := ButtonDestructive.Style(th); s.Fill != fade(th.Destructive, .1) || s.Label != th.Destructive {
		t.Errorf("destructive = %+v, want the Destructive surface with a DestructiveFg label", s)
	}
	for _, v := range []*ButtonVariant{ButtonOutline, ButtonGhost} {
		if s := v.Style(th); s.Hover != th.Muted || s.HoverLabel != th.Fg {
			t.Errorf("%s = %+v, want Accent under the pointer with an AccentFg label", v, s)
		}
	}
	if s := ButtonLink.Style(th); s.Fill != nil || s.Label != th.Primary || !s.Underline {
		t.Errorf("link = %+v, want an underlined Primary label", s)
	}
}

func TestBadgeVariants(t *testing.T) {
	t.Parallel()
	th := uitheme.Default()
	for _, tc := range []struct {
		badge       *BadgeWidget
		fill, label color.Color
	}{
		{Badge("a"), th.Primary, th.PrimaryFg},
		{Badge("a").Secondary(), th.Secondary, th.SecondaryFg},
		{Badge("a").Primary(), th.Primary, th.PrimaryFg},
		{Badge("a").Destructive(), fade(th.Destructive, .1), th.Destructive},
		{Badge("a").Outline(), nil, th.Fg},
	} {
		tc.badge.Layout(ggui.Loose(ggui.Sz(300, 300)), ggui.Env{})
		if s := tc.badge.style; s.Fill != tc.fill || s.Label != tc.label {
			t.Errorf("%s badge = %+v, want fill %v label %v", tc.badge.variant, s, tc.fill, tc.label)
		}
	}
}

func TestIconSizeVariantsAndPaddingOverride(t *testing.T) {
	t.Parallel()
	loose := ggui.Loose(ggui.Sz(300, 300))
	previous := 0.0
	for _, variant := range []*ButtonSizeVariant{ButtonIconExtraSmall, ButtonIconSmall, ButtonIcon, ButtonIconLarge} {
		size := ButtonOf(Icon(icons.Plus), nil).Size(variant).Layout(loose, ggui.Env{})
		if size.W != size.H || size.H <= previous {
			t.Fatalf("%s: %v after height %v", variant, size, previous)
		}
		previous = size.H
		overridden := Button("Padding", nil).Size(variant).Pad(3, 7).Layout(loose, ggui.Env{})
		plain := Button("Padding", nil).Pad(3, 7).Layout(loose, ggui.Env{})
		if overridden != plain {
			t.Fatalf("%s overrides explicit padding: %v != %v", variant, overridden, plain)
		}
	}
}
