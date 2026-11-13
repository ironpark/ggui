package ui

import (
	"testing"

	"github.com/ironpark/ggui"
)

// shadcn's native select has its chevron at right-2.5, 10px in from the
// edge, where the Select's sits at pr-2.
func TestNativeSelectChevronInset(t *testing.T) {
	t.Parallel()
	options := []string{"Standard", "Express"}
	plain := Select(ggui.State("Standard")).Options(options).Name("Plain")
	native := NativeSelect(ggui.State("Standard")).Options(options).Name("Native")
	p := ggui.NewProbe(ggui.Column(plain, native).Gap(8).Align(ggui.AlignStart), ggui.Sz(320, 200))
	defer p.Close()
	a, _ := p.FindRole(ggui.RoleSelect, "Plain")
	b, _ := p.FindRole(ggui.RoleSelect, "Native")
	if plain.pad.Right != 8 || native.pad.Right != 10 {
		t.Fatalf("the chevron is %v in from a Select's edge and %v from a NativeSelect's, want 8 and 10", plain.pad.Right, native.pad.Right)
	}
	if a.Rect.Size.H != 32 || b.Rect.Size.H != 32 {
		t.Fatalf("the fields are %v and %v tall, want 32", a.Rect.Size.H, b.Rect.Size.H)
	}
}
