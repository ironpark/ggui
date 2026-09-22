package theme

import (
	"image/color"
	"testing"
)

func TestThemePresetsHaveCompleteSemanticPairs(t *testing.T) {
	for _, base := range BaseColors() {
		for _, accent := range AccentColors() {
			for _, style := range Styles() {
				p := Preset{base, accent, style}
				for _, theme := range []Theme{p.Light(), p.Dark()} {
					colors := []color.Color{theme.Bg, theme.Fg, theme.Card, theme.CardFg, theme.Popover, theme.PopoverFg, theme.Primary, theme.PrimaryFg, theme.Secondary, theme.SecondaryFg, theme.Muted, theme.MutedFg, theme.Accent, theme.AccentFg, theme.Input, theme.InputBorder, theme.Border, theme.Ring, theme.Destructive, theme.Sidebar, theme.SidebarFg, theme.SidebarPrimary, theme.SidebarPrimaryFg, theme.SidebarAccent, theme.SidebarAccentFg, theme.SidebarBorder, theme.SidebarRing}
					colors = append(colors, theme.Chart[:]...)
					for _, c := range colors {
						if c == nil {
							t.Fatalf("missing color: %+v", p)
						}
					}
					if theme.Text.Color != theme.Fg || theme.Caption.Color != theme.MutedFg {
						t.Fatalf("text tokens not synchronized: %+v", p)
					}
				}
			}
		}
	}
}
func TestPresetAccentPreservesSurfacesAndCopies(t *testing.T) {
	base := Preset{Base: BaseStone}.Dark()
	accented := Preset{Base: BaseStone, Accent: AccentBlue}.Dark()
	if base.Bg != accented.Bg || base.Card != accented.Card || base.Border != accented.Border {
		t.Fatal("accent replaced base surfaces")
	}
	if base.Primary == accented.Primary || base.Chart == accented.Chart {
		t.Fatal("accent did not affect primary/chart")
	}
	accented.Chat.BubbleRadius = 999
	if (Preset{}).Dark().Chat.BubbleRadius == 999 {
		t.Fatal("mutable preset shared state")
	}
}
func TestPresetColorConversionAndGeometry(t *testing.T) {
	for _, tc := range []struct {
		s string
		c color.NRGBA
	}{{"oklch(0 0 0)", color.NRGBA{0, 0, 0, 255}}, {"oklch(1 0 0)", color.NRGBA{255, 255, 255, 255}}, {"oklch(1 0 0 / 10%)", color.NRGBA{255, 255, 255, 26}}} {
		if got := parseOKLCH(tc.s); got != tc.c {
			t.Fatalf("%s = %v", tc.s, got)
		}
	}
	nova, rhea := Preset{Style: StyleNova}.Light(), Preset{Style: StyleRhea}.Light()
	if rhea.Chat.BubbleRadius <= nova.Chat.BubbleRadius || rhea.Chat.AttachmentRadius <= nova.Chat.AttachmentRadius {
		t.Fatal("styles do not change chat geometry")
	}
	for _, p := range []Preset{{Base: "invalid"}, {Accent: "invalid"}, {Style: "invalid"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Error("invalid preset accepted")
				}
			}()
			p.Light()
		}()
	}
}
