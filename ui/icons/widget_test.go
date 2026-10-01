package icons

import (
	"errors"
	"image/color"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ironpark/ggui"
)

const squareSVG = `<svg viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg"><rect width="24" height="24" fill="currentColor"/></svg>`

func TestIconIsDecorativeUnlessGivenAnAlt(t *testing.T) {
	t.Parallel()
	asset, err := Parse([]byte(squareSVG))
	if err != nil {
		t.Fatal(err)
	}
	p := ggui.NewProbe(ggui.Row(
		New(asset).Size(20),
		New(asset).Alt("Saved").Color(color.Black).Rotate(1),
		Placeholder(Check).Fallback(Map{Check: asset}).Alt("Done"),
		Placeholder(Role("unknown")).Alt("Missing"), // resolves to nothing, still named
	), ggui.Sz(200, 40))
	defer p.Close()

	images := 0
	for _, n := range p.Semantics().Nodes(ggui.RoleImage) {
		images++
		if n.Name == "" {
			t.Errorf("an image node has no name: %+v", n)
		}
	}
	if images != 3 {
		t.Errorf("found %d image nodes, want 3: one per icon with an Alt", images)
	}
	for _, name := range []string{"Saved", "Done", "Missing"} {
		if _, ok := p.Semantics().Find(ggui.RoleImage, name); !ok {
			t.Errorf("no image named %q in the semantics tree", name)
		}
	}
}

func TestIconLayoutUsesItsSizeWithinConstraints(t *testing.T) {
	t.Parallel()
	loose := ggui.Loose(ggui.Sz(100, 100))
	cases := []struct {
		name string
		w    *Widget
		c    ggui.Constraints
		want ggui.Size
	}{
		{"default", Placeholder(Check), loose, ggui.Sz(16, 16)},
		{"explicit", New(nil).Size(24), loose, ggui.Sz(24, 24)},
		{"negative clamps to zero", New(nil).Size(-5), loose, ggui.Sz(0, 0)},
		{"larger than the space", New(nil).Size(300), ggui.Loose(ggui.Sz(50, 30)), ggui.Sz(50, 30)},
		{"smaller than the minimum", New(nil).Size(4), ggui.Constraints{MinW: 10, MinH: 10, MaxW: 50, MaxH: 50}, ggui.Sz(10, 10)},
	}
	for _, c := range cases {
		if got := c.w.Layout(c.c, ggui.Env{}); got != c.want {
			t.Errorf("%s: Layout = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestPaintingWithoutASurfaceOrAssetIsSafe(t *testing.T) {
	t.Parallel()
	// A headless canvas has no image, and a role no set knows has no asset;
	// neither may panic.
	w := Placeholder(Role("unknown"))
	w.Layout(ggui.Loose(ggui.Sz(16, 16)), ggui.Env{})
	w.Paint(nil, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(16, 16)))
	var none *SVG
	none.Draw(nil, ggui.Rct(ggui.Pt(0, 0), ggui.Sz(16, 16)), color.White, 0)
}

func TestLoadReadsFromAFilesystem(t *testing.T) {
	t.Parallel()
	files := fstest.MapFS{
		"ok.svg":  {Data: []byte(squareSVG)},
		"bad.svg": {Data: []byte(`<svg/>`)},
	}
	if a, err := Load(files, "ok.svg"); err != nil || a == nil {
		t.Errorf("Load(ok.svg) = %v, %v; want an asset", a, err)
	}
	if _, err := Load(files, "missing.svg"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Load(missing.svg) = %v, want fs.ErrNotExist", err)
	}
	if _, err := Load(files, "bad.svg"); err == nil || !strings.HasPrefix(err.Error(), "icons:") {
		t.Errorf("Load(bad.svg) = %v, want a parse error from icons", err)
	}
}
