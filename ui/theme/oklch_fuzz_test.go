package theme

import (
	"fmt"
	"image/color"
	"math"
	"testing"
)

// toOKLCH is Björn Ottosson's forward transform, sRGB to OKLab, written
// independently of parseOKLCH's inverse so the two can check each other.
func toOKLCH(c color.NRGBA) (l, ch, h float64) {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= .04045 {
			return x / 12.92
		}
		return math.Pow((x+.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	lm := math.Cbrt(.4122214708*r + .5363325363*g + .0514459929*b)
	mm := math.Cbrt(.2119034982*r + .6806995451*g + .1073969566*b)
	sm := math.Cbrt(.0883024619*r + .2817188376*g + .6299787005*b)
	l = .2104542553*lm + .7936177850*mm - .0040720468*sm
	a := 1.9779984951*lm - 2.4285922050*mm + .4505937099*sm
	bb := .0259040371*lm + .7827717662*mm - .8086757660*sm
	h = math.Atan2(bb, a) * 180 / math.Pi
	if h < 0 {
		h += 360
	}
	return l, math.Hypot(a, bb), h
}

// near reports whether every channel of got is within one step of want.
func near(got color.Color, want color.NRGBA) bool {
	g, ok := got.(color.NRGBA)
	if !ok {
		return false
	}
	d := func(x, y uint8) bool { return max(x, y)-min(x, y) <= 1 }
	return d(g.R, want.R) && d(g.G, want.G) && d(g.B, want.B) && d(g.A, want.A)
}

func TestParseOKLCHMatchesReferenceColors(t *testing.T) {
	t.Parallel()
	// Published OKLCH coordinates of the sRGB primaries and secondaries,
	// and of the shadcn neutral scale the presets are built from.
	cases := []struct {
		s    string
		want color.NRGBA
	}{
		{"oklch(0.627955 0.257683 29.233885)", color.NRGBA{255, 0, 0, 255}},
		{"oklch(0.866440 0.294827 142.495339)", color.NRGBA{0, 255, 0, 255}},
		{"oklch(0.452014 0.313214 264.052021)", color.NRGBA{0, 0, 255, 255}},
		{"oklch(0.967983 0.211006 109.769232)", color.NRGBA{255, 255, 0, 255}},
		{"oklch(0.905399 0.154625 194.768948)", color.NRGBA{0, 255, 255, 255}},
		{"oklch(0.701674 0.322491 328.363418)", color.NRGBA{255, 0, 255, 255}},
		{"oklch(0.145 0 0)", color.NRGBA{10, 10, 10, 255}},
		{"oklch(0.985 0 0)", color.NRGBA{250, 250, 250, 255}},
		{"oklch(0.5 0 0 / 0.5)", color.NRGBA{99, 99, 99, 128}},
		{"oklch(0.5 0 0 / 150%)", color.NRGBA{99, 99, 99, 255}}, // alpha is clamped
	}
	for _, c := range cases {
		if got := parseOKLCH(c.s); !near(got, c.want) {
			t.Errorf("parseOKLCH(%q) = %v, want %v within one step", c.s, got, c.want)
		}
	}
	// A green far outside sRGB needs negative red and blue; they clip to
	// zero rather than wrapping around to bright values.
	if got := parseOKLCH("oklch(0.7 0.4 150)").(color.NRGBA); got.R != 0 || got.B != 0 || got.G < 200 {
		t.Errorf("out-of-gamut green = %v, want red and blue clipped to 0", got)
	}
}

// FuzzParseOKLCHRoundTrip converts an sRGB color to OKLCH with the
// reference transform and checks parseOKLCH brings it back.
func FuzzParseOKLCHRoundTrip(f *testing.F) {
	f.Add(uint8(0), uint8(0), uint8(0), uint8(255))
	f.Add(uint8(255), uint8(255), uint8(255), uint8(26))
	f.Add(uint8(255), uint8(0), uint8(0), uint8(128))
	f.Add(uint8(1), uint8(2), uint8(3), uint8(0))
	f.Add(uint8(59), uint8(130), uint8(246), uint8(200))
	f.Fuzz(func(t *testing.T, r, g, b, a uint8) {
		want := color.NRGBA{r, g, b, a}
		l, c, h := toOKLCH(want)
		s := fmt.Sprintf("oklch(%.9f %.9f %.9f / %.9f%%)", l, c, h, float64(a)/255*100)
		if got := parseOKLCH(s); !near(got, want) {
			t.Fatalf("parseOKLCH(%q) = %v, want %v within one step", s, got, want)
		}
	})
}

// FuzzParseOKLCH checks any well-formed value converts without panicking,
// with alpha clamped into range and a colorless value coming out grey.
func FuzzParseOKLCH(f *testing.F) {
	f.Add(0.5, 0.1, 120.0, 1.0, false)
	f.Add(1.2, 0.0, 0.0, 50.0, true)
	f.Add(-0.3, 0.5, -720.0, -2.0, false)
	f.Add(0.0, 3.0, 1e9, 1e3, true)
	f.Fuzz(func(t *testing.T, l, c, h, a float64, percent bool) {
		for _, v := range []float64{l, c, h, a} {
			// The presets are generated data; NaN and infinities never
			// reach this parser, and their conversion to uint8 is
			// implementation-defined.
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
				return
			}
		}
		alpha, unit := a, ""
		if percent {
			alpha, unit = a/100, "%"
		}
		s := fmt.Sprintf("oklch(%g %g %g / %g%s)", l, c, h, a, unit)
		got, ok := parseOKLCH(s).(color.NRGBA)
		if !ok {
			t.Fatalf("parseOKLCH(%q) returned %T, want color.NRGBA", s, got)
		}
		if want := uint8(math.Round(max(0, min(1, alpha)) * 255)); got.A != want {
			t.Fatalf("parseOKLCH(%q) alpha = %d, want %d", s, got.A, want)
		}
		if c == 0 && l >= 0 && (max(got.R, got.G, got.B)-min(got.R, got.G, got.B) > 1) {
			t.Fatalf("parseOKLCH(%q) = %v, want a grey for zero chroma", s, got)
		}
	})
}

// FuzzParseColor feeds arbitrary text: ParseColor reports malformed input
// as an error and never panics.
func FuzzParseColor(f *testing.F) {
	for _, s := range []string{"oklch(0.5 0.1 120)", "oklch(1 0 0 / 10%)", "oklch(", "", "1 2", "oklch(1 0 0 / )", "a/b/c",
		"oklch(1 0 0 / 1 / 2)", "#fff", "#12345", "rgb(1, 2, 3, .5)", "hsl(210 40% 98%)", "222.2 84% 4.9%", "var(--x)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := ParseColor(s)
		if (err == nil) == (c == nil) {
			t.Fatalf("ParseColor(%q) = %v, %v: want a color or an error", s, c, err)
		}
	})
}

// parseOKLCH is ParseColor for values known to parse.
func parseOKLCH(s string) color.Color {
	c, err := ParseColor(s)
	if err != nil {
		panic(err)
	}
	return c
}
