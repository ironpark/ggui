package theme

import (
	"image/color"
	"testing"
)

func TestParseColor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want color.NRGBA
	}{
		{"#fff", color.NRGBA{255, 255, 255, 255}},
		{"#0008", color.NRGBA{0, 0, 0, 0x88}},
		{"#2563EB", color.NRGBA{0x25, 0x63, 0xeb, 255}},
		{"#2563eb80", color.NRGBA{0x25, 0x63, 0xeb, 0x80}},
		{"rgb(37, 99, 235)", color.NRGBA{37, 99, 235, 255}},
		{"rgba(37, 99, 235, 0.5)", color.NRGBA{37, 99, 235, 128}},
		{"rgb(100% 0% 0% / 50%)", color.NRGBA{255, 0, 0, 128}},
		{"hsl(0 100% 50%)", color.NRGBA{255, 0, 0, 255}},
		{"hsla(120, 100%, 25%, 1)", color.NRGBA{0, 128, 0, 255}},
		{"0 0% 100%", color.NRGBA{255, 255, 255, 255}},               // shadcn v3
		{"222.2 84% 4.9%", color.NRGBA{2, 8, 23, 255}},               // shadcn v3 --foreground
		{"oklch(1 0 0 / 10%)", color.NRGBA{255, 255, 255, 26}},       // shadcn v4 dark --border
		{"oklch(62.8% 0.2577 29.23)", color.NRGBA{255, 0, 0, 255}},   // percent lightness
		{"oklch(0.628 64.4% 29.23deg)", color.NRGBA{255, 0, 0, 255}}, // percent chroma
		{"transparent", color.NRGBA{}},
	} {
		got, err := ParseColor(tc.in)
		if err != nil {
			t.Errorf("ParseColor(%q): %v", tc.in, err)
			continue
		}
		if !near(got, tc.want) {
			t.Errorf("ParseColor(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
	for _, bad := range []string{"", "#12345", "rgb(1 2)", "lab(50 0 0)", "oklch(a b c)", "red"} {
		if c, err := ParseColor(bad); err == nil {
			t.Errorf("ParseColor(%q) = %v, want an error", bad, c)
		}
	}
}

// shadcnCSS is the shape shadcn/ui's theme generator exports.
const shadcnCSS = `
@layer base {
  :root {
    --radius: 0.625rem; /* 10px */
    --background: oklch(1 0 0);
    --foreground: oklch(0.145 0 0);
    --primary: #2563eb;
    --primary-foreground: oklch(0.985 0 0);
    --ring: var(--primary);
    --font-sans: Inter, sans-serif;
  }
  .dark {
    --background: oklch(0.145 0 0);
    --foreground: oklch(0.985 0 0);
    --border: oklch(1 0 0 / 10%);
  }
}
@theme inline { --color-primary: var(--primary); }
`

func TestFromCSS(t *testing.T) {
	t.Parallel()
	light, dark, err := FromCSS(shadcnCSS)
	if err != nil {
		t.Fatal(err)
	}
	blue := color.NRGBA{0x25, 0x63, 0xeb, 255}
	if light.Primary != color.Color(blue) || light.Ring != color.Color(blue) {
		t.Errorf("light Primary %v, Ring %v: want %v from --primary and var(--primary)", light.Primary, light.Ring, blue)
	}
	if light.Radius != 8 || light.RadiusSm != 6 || light.RadiusLg != 14 {
		t.Errorf("light radii %v/%v/%v, want 8/6/14 from --radius 10px", light.Radius, light.RadiusSm, light.RadiusLg)
	}
	if !sameColor(light.PrimaryHover, compositeColor(blue, light.Bg, .9)) {
		t.Errorf("PrimaryHover %v did not follow the CSS --primary", light.PrimaryHover)
	}
	// .dark inherits what it does not set from :root, and its own wins.
	if dark.Primary != color.Color(blue) || dark.Radius != 8 {
		t.Errorf("dark Primary %v, Radius %v: want :root's", dark.Primary, dark.Radius)
	}
	if !near(dark.Bg, color.NRGBA{10, 10, 10, 255}) || !near(dark.Border, color.NRGBA{255, 255, 255, 26}) {
		t.Errorf("dark Bg %v, Border %v: want .dark's", dark.Bg, dark.Border)
	}
	if dark.InputBorder != dark.Border {
		t.Errorf("dark InputBorder %v did not follow Border %v", dark.InputBorder, dark.Border)
	}
	if _, _, err := FromCSS(`:root { --primary: oklch(nope); }`); err == nil {
		t.Error("a bad color parsed")
	}
	if _, err := Default().ApplyCSS(`--ring: var(--missing);`); err == nil {
		t.Error("an undefined var() parsed")
	}
}
