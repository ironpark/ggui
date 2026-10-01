package icons

import (
	"math"
	"testing"
)

// FuzzParse feeds Parse arbitrary bytes. It must return an error rather
// than panic, and anything it accepts must have a finite, positive viewBox
// and rasterize into a pure coverage mask.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		squareSVG,
		`<svg viewBox="10 20 20 10" xmlns="http://www.w3.org/2000/svg"><rect x="10" y="20" width="20" height="10" fill="currentColor"/></svg>`,
		`<svg viewBox="0 0 24 24"><path d="M12 2L2 22h20z" stroke="currentColor" stroke-width="2" fill="none"/></svg>`,
		`<svg viewBox="0 0 24 24"><circle cx="12" cy="12" r="10"/><polyline points="1,2 3,4"/></svg>`,
		`<svg viewBox="0 0 NaN 24"/>`,
		`<svg viewBox="0 0 1e400 24"/>`,
		`<svg viewBox="0 0 0 24"/>`,
		`<svg width="24" height="24"/>`,
		`<svg viewBox="0 0 24 24"><path d="M"/></svg>`,
		`<svg`,
		``,
		"\x00\xff",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		a, err := Parse(data)
		if err != nil {
			if a != nil {
				t.Fatalf("Parse returned both %v and an error %v", a, err)
			}
			return
		}
		b := a.icon.ViewBox
		for _, v := range []float64{b.X, b.Y, b.W, b.H} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("accepted a viewBox with a non-finite value: %+v", b)
			}
		}
		if b.W <= 0 || b.H <= 0 {
			t.Fatalf("accepted a viewBox without area: %+v", b)
		}
		img := a.rasterize(8)
		for i := 0; i < len(img.Pix); i += 4 {
			px := img.Pix[i : i+4]
			if px[0] != px[3] || px[1] != px[3] || px[2] != px[3] {
				t.Fatalf("rasterized pixel %d is %v, want white coverage", i/4, px)
			}
		}
	})
}
