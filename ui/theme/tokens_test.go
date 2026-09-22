package theme

import (
	"image/color"
	"testing"

	"github.com/ironpark/ggui"
)

func TestThemeTokens(t *testing.T) {
	k := ggui.NewEnvKey[color.Color]("danger")
	base := Default()
	red := base.Set(k, color.Color(color.RGBA{0xd3, 0x2f, 0x2f, 0xff}))
	if _, ok := base.Get(k); ok {
		t.Fatal("Set changed its receiver")
	}
	if c, ok := red.Get(k); !ok || c != color.Color(color.RGBA{0xd3, 0x2f, 0x2f, 0xff}) {
		t.Fatalf("Get = %v, %v", c, ok)
	}
	if got, _ := red.Set(k, color.Color(color.Black)).Get(k); got != color.Color(color.Black) {
		t.Fatal("a later Set does not win")
	}
}
