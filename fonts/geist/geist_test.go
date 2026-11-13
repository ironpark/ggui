package geist

import (
	"bytes"
	"crypto/sha256"
	"image/color"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/fontsys"
)

func TestFontsLoadOnceWithoutChangingDefaults(t *testing.T) {
	t.Parallel()
	if f := Font(); f == nil || f != Font() {
		t.Fatal("Font must return one shared family")
	}
	if f := Mono(); f == nil || f != Mono() || f == Font() {
		t.Fatal("Mono must return one shared family of its own")
	}
	if (ggui.Env{}).Text().Font != nil {
		t.Fatal("loading Geist changed the default text font")
	}
}

func TestEmbeddedWeightsHaveGeistMetadataAndRenderTheirRealFaces(t *testing.T) {
	t.Parallel()
	seen := map[[32]byte]bool{}
	for _, family := range []struct {
		prefix, name string
		font         *ggui.Font
	}{{"Geist", "Geist", Font()}, {"GeistMono", "Geist Mono", Mono()}} {
		for _, tc := range []struct {
			file   string
			weight ggui.FontWeight
		}{
			{"Regular", ggui.WeightRegular},
			{"Medium", ggui.WeightMedium},
			{"SemiBold", ggui.WeightSemibold},
			{"Bold", ggui.WeightBold},
		} {
			file := family.prefix + "-" + tc.file + ".ttf"
			data, err := files.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := fontsys.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			meta := parsed.Source().Metadata()
			if !strings.HasPrefix(meta.Family, family.name) || float32(meta.Weight) != float32(tc.weight) {
				t.Fatalf("%s metadata=%+v", file, meta)
			}
			style := ggui.TextStyle{Font: family.font, Size: 20, Weight: tc.weight, Color: color.Black}
			got := ggui.RenderText("Contribution History $250.00", style, 1000)
			style.Font = ggui.MustFont(data)
			want := ggui.RenderText("Contribution History $250.00", style, 1000)
			if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
				t.Fatalf("%s: weight %d did not select its embedded face", file, tc.weight)
			}
			hash := sha256.Sum256(got.Pix)
			if seen[hash] {
				t.Fatalf("%s reused another face's raster", file)
			}
			seen[hash] = true
		}
	}
}
