package fontsys

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomedium"
	"golang.org/x/image/font/gofont/goregular"
)

func mustLoad(t *testing.T, data []byte) *Font {
	t.Helper()
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
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
