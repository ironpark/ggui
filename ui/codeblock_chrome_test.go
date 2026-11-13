package ui_test

import (
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

// A block has a header row only when it has a title or a language; without
// one the copy action floats over the top end corner of the code.
func TestCodeBlockHeaderIsOptionalAndTheActionFloatsWithoutOne(t *testing.T) {
	t.Parallel()
	bare, titled := ui.CodeBlock("go build ./..."), ui.CodeBlock("go build ./...").Header("build.sh")
	p := ggui.NewProbe(ggui.Column(bare, titled).Gap(20).Align(ggui.AlignStretch), ggui.Sz(320, 300))
	defer p.Close()
	groups := p.Semantics().Nodes(ggui.RoleGroup)
	var plain, headed ggui.SemNode
	for _, g := range groups {
		switch g.Name {
		case "Code":
			plain = g
		case "build.sh":
			headed = g
		}
	}
	if plain.Rect.Size.H <= 0 || headed.Rect.Size.H <= plain.Rect.Size.H {
		t.Fatalf("heights without and with a header: %v, %v", plain.Rect.Size.H, headed.Rect.Size.H)
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "Code"); ok {
		t.Fatal("a block without a title prints a fallback caption")
	}
	var floating, inHeader ggui.SemNode
	for _, b := range p.Semantics().Nodes(ggui.RoleButton) {
		if b.Rect.Origin.Y < headed.Rect.Origin.Y {
			floating = b
		} else {
			inHeader = b
		}
	}
	if floating.Rect.Origin.Y != plain.Rect.Origin.Y+8 || floating.Rect.Origin.X+floating.Rect.Size.W != plain.Rect.Origin.X+plain.Rect.Size.W-8 {
		t.Fatalf("floating action at %v in %v", floating.Rect, plain.Rect)
	}
	if floating.Rect.Size != ggui.Sz(24, 24) || inHeader.Rect.Size != ggui.Sz(24, 24) {
		t.Fatalf("copy actions are %v and %v, want 24px icon buttons", floating.Rect.Size, inHeader.Rect.Size)
	}
}

// The copy action says it copied for two seconds, then turns back.
func TestCodeBlockCopyFeedbackRevertsAfterTwoSeconds(t *testing.T) {
	t.Parallel()
	p := ggui.NewProbe(ggui.Column(ui.CodeBlock("go test ./...").Language("shell")), ggui.Sz(320, 200))
	defer p.Close()
	p.Tap("Copy code")
	node(t, p.Semantics(), ggui.RoleStatus, "Code copied")
	p.Advance(1500 * time.Millisecond)
	node(t, p.Semantics(), ggui.RoleStatus, "Code copied")
	p.Advance(600 * time.Millisecond)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleStatus, "Code copied"); ok {
		t.Fatal("copy feedback did not revert")
	}
}

// Small tightens the block, and a footer adds a row under the code.
func TestCodeBlockSmallDestructiveAndFooter(t *testing.T) {
	t.Parallel()
	code := "first\nsecond\nthird"
	size := func(w ggui.Widget) ggui.Size { return w.Layout(ggui.Loose(ggui.Sz(320, 600)), ggui.Env{}) }
	normal, small := size(ui.CodeBlock(code).Header("a.txt")), size(ui.CodeBlock(code).Header("a.txt").Small())
	if small.H >= normal.H {
		t.Fatalf("small block is %v tall, default %v", small.H, normal.H)
	}
	footed := size(ui.CodeBlock(code).Header("a.txt").Footer(ggui.Text("3 lines")))
	if footed.H <= normal.H {
		t.Fatalf("footer added no row: %v against %v", footed.H, normal.H)
	}
	p := ggui.NewProbe(ggui.Column(ui.CodeBlock(code).Destructive().Small().Footer(ggui.Text("3 lines"))), ggui.Sz(320, 300))
	defer p.Close()
	footer := node(t, p.Semantics(), ggui.RoleText, "3 lines")
	text := node(t, p.Semantics(), ggui.RoleText, code)
	if footer.Rect.Origin.Y < text.Rect.Origin.Y+text.Rect.Size.H {
		t.Fatalf("footer at %v overlaps the code at %v", footer.Rect, text.Rect)
	}
}
