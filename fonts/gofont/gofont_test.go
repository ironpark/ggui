package gofont_test

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/widget"
)

func TestEnableDrawsBoldInTheGoFonts(t *testing.T) {
	var regular, bold ggui.Size
	p := ggui.NewProbe(widget.Text("x"), ggui.Sz(10, 10)).Setup(gofont.Enable)
	defer p.Close()
	p.Frame()
	p.Post(func() {
		loose := ggui.Loose(ggui.Sz(1000, 100))
		regular = widget.Text("Weighty words").Layout(loose, ggui.Env{})
		bold = widget.Text("Weighty words").Weight(ggui.WeightBold).Layout(loose, ggui.Env{})
	})
	p.Frame()
	if bold.W <= regular.W {
		t.Fatalf("bold %v is no wider than regular %v under gofont.Enable", bold.W, regular.W)
	}
}
