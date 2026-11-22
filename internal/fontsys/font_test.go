package fontsys

import (
	"os"
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
)

func mustLoad(t testing.TB, data []byte) *Font {
	t.Helper()
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// emojiChain is Go Regular with the emoji test subset as its fallback.
func emojiChain(t testing.TB) *Font {
	t.Helper()
	data, err := os.ReadFile("../../assets/testdata/emoji-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := mustLoad(t, goregular.TTF)
	f.AddFallbacks(mustLoad(t, data))
	return f
}

func TestFontPicksTheNearestWeight(t *testing.T) {
	t.Parallel()
	medium, bold := mustLoad(t, gomedium.TTF), mustLoad(t, gobold.TTF)
	f := mustLoad(t, goregular.TTF)
	f.WithWeight(500, medium)
	f.WithWeight(700, bold)
	for _, tc := range []struct {
		ask  Weight
		want *Font
	}{
		{0, f}, {300, f}, {400, f}, {450, medium}, {500, medium},
		{600, bold}, // equally near 500 and 700: the heavier, as CSS does
		{900, bold},
	} {
		if got, _ := f.ForWeight(tc.ask); got != tc.want {
			t.Errorf("weight %d drew the face of weight %d, want %d", tc.ask, got.weight, tc.want.weight)
		}
	}
	if f.weight != weightRegular || medium.weight != 500 {
		t.Fatalf("weights %d and %d, want the metadata's and the one WithWeight gave", f.weight, medium.weight)
	}
}

func TestFontConcurrentConfigAndRead(t *testing.T) {
	t.Parallel()
	f := mustLoad(t, goregular.TTF)
	bold := mustLoad(t, gobold.TTF)
	med := mustLoad(t, gomedium.TTF)
	var wg sync.WaitGroup
	wg.Go(func() {
		f.WithWeight(500, med)
		f.WithWeight(700, bold)
		f.SetNoFallback()
	})
	for range 4 {
		wg.Go(func() {
			for range 20 {
				f.ForWeight(700)
				f.Face(14, 700, nil)
				f.SpacedFace(14, 400, 1, nil)
				f.DrawsEmoji()
				f.Picker(500)
			}
		})
	}
	wg.Wait()
}

func TestWeightAxisCachedZeroAllocs(t *testing.T) {
	f := mustLoad(t, goregular.TTF)
	f.weightAxis()
	if n := testing.AllocsPerRun(50, func() { f.weightAxis() }); n != 0 {
		t.Fatalf("weightAxis allocated %v times after warm-up", n)
	}
}
