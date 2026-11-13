package ui

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
)

// A trackpad's swipe is never along one axis alone. The code's horizontal
// scroll, inside its vertical one, must not keep the whole of it.
func TestCodeBlockScrollsBothWaysOnADiagonalSwipe(t *testing.T) {
	code := strings.Repeat("recipe := theme.Config{Preset: theme.Preset{Base: theme.BaseNeutral, Accent: theme.AccentViolet}, Chart: theme.AccentBlue}\n", 60)
	c := CodeBlock(code).Height(200)
	p := ggui.NewProbe(ggui.Box(c).Width(400), ggui.Sz(400, 300))
	defer p.Close()
	p.Frame()
	p.Scroll(ggui.Pt(200, 100), ggui.Pt(-1, -5))
	p.Frame()
	p.Frame()
	if c.xController.Offset() <= 0 || c.yController.Offset() <= 0 {
		t.Fatalf("a diagonal swipe scrolled x %v and y %v, want both", c.xController.Offset(), c.yController.Offset())
	}
}
