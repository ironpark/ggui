package ui_test

import (
	"math"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A padded button beside a label lines its text up with the label's under
// AlignBaseline, since the button reports its label's baseline.
func TestButtonBaselineAlignsWithText(t *testing.T) {
	label := ggui.Text("Go")
	button := ui.Button("Run", nil).Pad(8, 16)
	p := ggui.NewProbe(ggui.Row(label, button).Gap(8).Align(ggui.AlignBaseline), ggui.Sz(300, 100))
	defer p.Close()
	tree := p.Semantics()
	text, ok1 := tree.Find(ggui.RoleText, "Go")
	btn, ok2 := tree.Find(ggui.RoleButton, "Run")
	if !ok1 || !ok2 {
		t.Fatal(tree)
	}
	lb, _ := label.Baseline()
	bb, ok := button.Baseline()
	if !ok || math.Abs((text.Rect.Origin.Y+lb)-(btn.Rect.Origin.Y+bb)) > 1e-6 {
		t.Fatalf("label baseline %v, button baseline %v (%v)", text.Rect.Origin.Y+lb, btn.Rect.Origin.Y+bb, ok)
	}
	if btn.Rect.Origin.Y >= text.Rect.Origin.Y {
		t.Fatalf("button top %v should sit above the label top %v", btn.Rect.Origin.Y, text.Rect.Origin.Y)
	}
}
