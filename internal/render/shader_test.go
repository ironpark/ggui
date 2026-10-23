package render

import "testing"

// A shader that fails to compile panics on its first use, which is the
// first shape of its kind painted.
func TestShadersCompile(t *testing.T) {
	for _, shader := range []func() any{
		func() any { return sharedRoundRect() },
		func() any { return sharedRoundRectMask() },
		func() any { return sharedRoundRectImage() },
		func() any { return sharedShadow() },
		func() any { return sharedGradient() },
		func() any { return sharedBlur() },
	} {
		if shader() == nil {
			t.Fatal("missing shader")
		}
	}
	if first, second := sharedShadow(), sharedShadow(); first != second {
		t.Fatal("shader not shared")
	}
}

func TestBlurFactorKeepsEachPassNarrow(t *testing.T) {
	for _, c := range []struct {
		sigma float64
		want  int
	}{{0, 1}, {BlurSigma, 1}, {BlurSigma + 0.1, 2}, {20, 5}} {
		if got := BlurFactor(c.sigma); got != c.want {
			t.Errorf("BlurFactor(%v) = %d, want %d", c.sigma, got, c.want)
		}
	}
}
