package ui

import (
	"strings"
	"testing"
	"time"

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

// The code's two scrolls are clipped to one viewport, so their hit regions
// have one Rect. The outer, vertical one must keep its own offset from one
// frame to the next and not take the inner's.
func TestCodeBlockKeepsScrollingDownInAWindow(t *testing.T) {
	code := strings.Repeat("recipe := theme.Config{Preset: theme.Preset{Base: theme.BaseNeutral, Accent: theme.AccentViolet}}\n", 60)
	c := CodeBlock(code).Language("go").LineNumbers(true).Height(340)
	p := ggui.NewWindowProbe(func() ggui.Widget { return ggui.Center(ggui.Box(c).Width(600)) }, ggui.Sz(1000, 700))
	defer p.Close()
	p.Run(time.Second, 60)
	for range 6 {
		p.Trackpad(ggui.Pt(500, 350), ggui.Pt(-6, -10))
		p.Step(16 * time.Millisecond)
	}
	if x, y := c.xController.Offset(), c.yController.Offset(); x != 36 || y != 60 {
		t.Fatalf("six swipes of (-6, -10) scrolled to x %v, y %v, want 36 and 60", x, y)
	}
}
