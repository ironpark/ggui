package icons

import (
	"testing"

	"github.com/ironpark/ggui"
	uitheme "github.com/ironpark/ggui/ui/theme"
)

func TestSVGCurrentColorAndViewBox(t *testing.T) {
	t.Parallel()
	a, err := Parse([]byte(`<svg viewBox="10 20 20 10" xmlns="http://www.w3.org/2000/svg"><rect x="10" y="20" width="20" height="10" fill="currentColor"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := a.rasterize(40)
	if got := img.RGBAAt(20, 20); got.A != 255 || got.R != 255 {
		t.Fatalf("center coverage %v", got)
	}
	if img.RGBAAt(20, 2).A != 0 || img.RGBAAt(20, 37).A != 0 {
		t.Fatal("non-square SVG was stretched or viewBox origin ignored")
	}
	for _, source := range []string{`<svg>`, `<svg viewBox="0 0 0 24"/>`, `<svg viewBox="0 0 -24 24"/>`} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Fatalf("invalid SVG accepted: %s", source)
		}
	}
}
func TestSetResolutionPrecedenceAndFallback(t *testing.T) {
	t.Parallel()
	a, b, c := &SVG{}, &SVG{}, &SVG{}
	theme := uitheme.Default().Set(SetKey, Set(Map{Check: b, Close: b}))
	env := theme.Apply(ggui.Env{}).With(SetKey, Set(Map{Check: a}))
	fallback := Map{Check: c, Close: c, Search: c}
	for role, want := range map[Role]*SVG{Check: a, Close: b, Search: c, Role("missing"): nil} {
		if got := Resolve(env, fallback, role); got != want {
			t.Fatalf("%s resolved to %p, want %p", role, got, want)
		}
	}
}

func TestSVGImageIsAWhiteMask(t *testing.T) {
	t.Parallel()
	a, err := Parse([]byte(`<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><circle cx="12" cy="12" r="6" stroke="currentColor" fill="none" stroke-width="2"/></svg>`))
	if err != nil {
		t.Fatal(err)
	}
	img := a.Image(36)
	if img.Bounds().Dx() != 36 || img.Bounds().Dy() != 36 {
		t.Fatalf("image is %v, want 36 square", img.Bounds())
	}
	if got := img.RGBAAt(18, 9); got.A == 0 || got.R != got.A || got.G != got.A || got.B != got.A {
		t.Fatalf("on the ring %v, want white of its coverage", got)
	}
	if img.RGBAAt(18, 18).A != 0 || img.RGBAAt(1, 1).A != 0 {
		t.Fatal("inside or outside the ring is not clear")
	}
	if (*SVG)(nil).Image(36) != nil || a.Image(0) != nil {
		t.Fatal("an image from no icon, or of no size")
	}
}
