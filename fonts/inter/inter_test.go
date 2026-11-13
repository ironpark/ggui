package inter

import (
	"bytes"
	"crypto/sha256"
	"image/color"
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/fontsys"
)

func TestFontLoadsOnceWithoutChangingDefaults(t *testing.T) {
	t.Parallel()
	if f := Font(); f == nil || f != Font() {
		t.Fatal("Font must return one shared family")
	}
	if (ggui.Env{}).Text().Font != nil {
		t.Fatal("loading Inter changed the default text font")
	}
}

func TestEmbeddedWeightsHaveInterMetadataAndRenderTheirRealFaces(t *testing.T) {
	t.Parallel()
	seen := map[[32]byte]bool{}
	for _, tc := range []struct {
		file   string
		weight ggui.FontWeight
	}{
		{"Inter-Regular.ttf", ggui.WeightRegular},
		{"Inter-Medium.ttf", ggui.WeightMedium},
		{"Inter-SemiBold.ttf", ggui.WeightSemibold},
		{"Inter-Bold.ttf", ggui.WeightBold},
	} {
		data, err := files.ReadFile(tc.file)
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := fontsys.Load(data)
		if err != nil {
			t.Fatal(err)
		}
		meta := parsed.Source().Metadata()
		if !strings.HasPrefix(meta.Family, "Inter") || float32(meta.Weight) != float32(tc.weight) {
			t.Fatalf("%s metadata=%+v", tc.file, meta)
		}
		style := ggui.TextStyle{Font: Font(), Size: 20, Weight: tc.weight, Color: color.Black}
		got := ggui.RenderText("Contribution History $250.00", style, 1000)
		style.Font = ggui.MustFont(data)
		want := ggui.RenderText("Contribution History $250.00", style, 1000)
		if got.Bounds() != want.Bounds() || !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("weight %d did not select its embedded face", tc.weight)
		}
		hash := sha256.Sum256(got.Pix)
		if seen[hash] {
			t.Fatalf("weight %d reused another weight's raster", tc.weight)
		}
		seen[hash] = true
	}
}
