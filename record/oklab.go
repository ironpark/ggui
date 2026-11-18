package record

import (
	"image/color"
	"math"
	"slices"
)

// Oklab (Björn Ottosson, 2020) is a color space in which equal distances
// look about equally different: L is lightness, 0 to 1, and a and b the
// green–red and blue–yellow axes, about ±0.4. A difference of 0.02 is
// about the least the eye tells apart.

// srgbLinear[v] is the sRGB value v, 0 to 255, as linear light.
var srgbLinear = func() (t [256]float64) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = c / 12.92
		} else {
			t[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return t
}()

// oklab returns the sRGB color r, g, b, 0 to 255 each, in Oklab.
func oklab(r, g, b float64) [3]float64 {
	lin := func(v float64) float64 {
		if v >= 0 && v <= 255 && v == math.Trunc(v) {
			return srgbLinear[int(v)]
		}
		c := min(max(v, 0), 255) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	lr, lg, lb := lin(r), lin(g), lin(b)
	l := math.Cbrt(0.4122214708*lr + 0.5363325363*lg + 0.0514459929*lb)
	m := math.Cbrt(0.2119034982*lr + 0.6806995451*lg + 0.1073969566*lb)
	s := math.Cbrt(0.0883024619*lr + 0.2817188376*lg + 0.6299787005*lb)
	return [3]float64{
		0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

// fromOklab returns the Oklab color c in sRGB, 0 to 255 each, clipped to
// what sRGB holds.
func fromOklab(c [3]float64) [3]float64 {
	l := c[0] + 0.3963377774*c[1] + 0.2158037573*c[2]
	m := c[0] - 0.1055613458*c[1] - 0.0638541728*c[2]
	s := c[0] - 0.0894841775*c[1] - 1.2914855480*c[2]
	l, m, s = l*l*l, m*m*m, s*s*s
	lin := [3]float64{
		4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		-1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		-0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
	var out [3]float64
	for i, v := range lin {
		v = min(max(v, 0), 1)
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		}
		out[i] = v * 255
	}
	return out
}

func oklabOf(c color.Color) [3]float64 {
	r, g, b, _ := c.RGBA()
	return oklab(float64(r>>8), float64(g>>8), float64(b>>8))
}

func labDist(a, b [3]float64) float64 {
	d0, d1, d2 := a[0]-b[0], a[1]-b[1], a[2]-b[2]
	return math.Sqrt(d0*d0 + d1*d1 + d2*d2)
}

// maxNear is how many other colors lossy compression tries for a pixel.
const maxNear = 24

// newNearness works out which of pal's colors pass for which at Lossy(n):
// those within n/1000 of each other in Oklab.
func newNearness(pal color.Palette, n int) *nearness {
	limit := float64(n) / 1000
	lab := make([][3]float64, len(pal))
	for i, c := range pal {
		lab[i] = oklabOf(c)
	}
	nr := new(nearness)
	for b := range pal {
		nr.ok[b][b] = true
		type cand struct {
			i int
			d float64
		}
		var cs []cand
		for a := range pal {
			if a == b {
				continue
			}
			if d := labDist(lab[a], lab[b]); d <= limit {
				nr.ok[a][b] = true
				cs = append(cs, cand{a, d})
			}
		}
		slices.SortFunc(cs, func(x, y cand) int {
			switch {
			case x.d < y.d:
				return -1
			case x.d > y.d:
				return 1
			}
			return x.i - y.i
		})
		for _, c := range cs[:min(len(cs), maxNear)] {
			nr.near[b] = append(nr.near[b], byte(c.i))
		}
	}
	return nr
}
