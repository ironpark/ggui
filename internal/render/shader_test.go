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
		func() any { return sharedPathGradient() },
	} {
		if shader() == nil {
			t.Fatal("missing shader")
		}
	}
	if sharedShadow() != sharedShadow() {
		t.Fatal("shader not shared")
	}
}
