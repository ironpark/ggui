package gofont_test

import (
	"bytes"
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

func TestEnableItalicSlantsItalicText(t *testing.T) {
	upright := ggui.TextStyle{Font: gofont.Font(), Size: 20}
	italic := upright
	italic.Italic = true
	before := ggui.RenderText("Slant", italic, 0)
	if !bytes.Equal(before.Pix, ggui.RenderText("Slant", upright, 0).Pix) {
		t.Fatal("italic text is slanted before EnableItalic")
	}
	gofont.EnableItalic()
	if bytes.Equal(ggui.RenderText("Slant", italic, 0).Pix, before.Pix) {
		t.Fatal("italic text is drawn upright after EnableItalic")
	}
}
