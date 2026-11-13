package ui_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestCodeBlockCopiesOriginalAndFollowsSource(t *testing.T) {
	t.Parallel()
	source := ggui.State("package main\n\n// 한글\nfunc main() {}\n")
	var copied []string
	block := ui.CodeBlockOf(source).Language("go").Header("main.go").LineNumbers(true).
		OnCopy(func(code string) { copied = append(copied, code) })
	p := ggui.NewProbe(ggui.Column(block), ggui.Sz(380, 260))
	defer p.Close()
	node(t, p.Semantics(), ggui.RoleGroup, "main.go · go")
	p.Tap("Copy code")
	node(t, p.Semantics(), ggui.RoleStatus, "Code copied")
	if got := p.Clipboard().Read(); got != ggui.Peek(source) || len(copied) != 1 || copied[0] != got {
		t.Fatalf("copy changed source or missed callback: %q, %q", got, copied)
	}
	source.Set("changed\r\n\tvalue")
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleStatus, "Code copied"); ok {
		t.Fatal("copy feedback survived changed source")
	}
	node(t, p.Semantics(), ggui.RoleText, ggui.Peek(source))
	p.Type(ggui.Mods{}, ggui.KeyEnter)
	if got := p.Clipboard().Read(); got != ggui.Peek(source) {
		t.Fatalf("keyboard copied stale source: %q", got)
	}
	block.Code("literal")
	source.Set("detached reader")
	p.Tap("Copy code")
	if got := p.Clipboard().Read(); got != "literal" {
		t.Fatalf("literal failed to detach reader: %q", got)
	}
}

func TestCodeBlockKeyboardViewportAndSelection(t *testing.T) {
	t.Parallel()
	code := strings.Repeat(strings.Repeat("wide code ", 25)+"\n", 20)
	block := ui.CodeBlock(code).Height(100)
	p := ggui.NewProbe(ggui.Column(block), ggui.Sz(260, 220))
	defer p.Close()
	p.Type(ggui.Mods{}, ggui.KeyTab, ggui.KeyTab)
	if focused, ok := p.Semantics().Focused(); !ok || focused.Name != "Code content" {
		t.Fatal("code viewport is not a Tab stop", focused)
	}
	before := node(t, p.Semantics(), ggui.RoleText, code)
	p.Type(ggui.Mods{}, ggui.KeyArrowRight, ggui.KeyPageDown)
	after := node(t, p.Semantics(), ggui.RoleText, code)
	if after.Full.Origin.X >= before.Full.Origin.X || after.Full.Origin.Y >= before.Full.Origin.Y {
		t.Fatal("keyboard did not scroll both axes")
	}
	p.Key("cmd+a", "cmd+c")
	if p.Clipboard().Read() != code {
		t.Fatal("focused viewport cannot select and copy code")
	}
	p.Type(ggui.Mods{}, ggui.KeyHome)
	if home := node(t, p.Semantics(), ggui.RoleText, code); home.Full.Origin != before.Full.Origin {
		t.Fatal("Home did not return to start")
	}
}

func TestCodeBlockKeepsCodeLTRInsideRTL(t *testing.T) {
	t.Parallel()
	code := "short\na much longer second line"
	block := ui.CodeBlock(code).LineNumbers(true).StartLine(50)
	p := ggui.NewProbe(ggui.Directionality(ggui.DirRTL, ggui.Column(block)), ggui.Sz(360, 180))
	defer p.Close()
	text := node(t, p.Semantics(), ggui.RoleText, code)
	if copy := node(t, p.Semantics(), ggui.RoleButton, "Copy code"); copy.Rect.Origin.X != 12 {
		t.Fatal("header did not follow RTL", copy.Rect)
	}
	p.Click(text.Full.Origin.Add(ggui.Pt(1, 4)))
	text = node(t, p.Semantics(), ggui.RoleText, code)
	if text.SelStart != 0 || text.SelEnd != 0 {
		t.Fatal("RTL inherited into code alignment", text.SelStart, text.SelEnd)
	}
	p.Tap("Copy code")
	if p.Clipboard().Read() != code {
		t.Fatal("start line changed copied code")
	}
}

func TestCodeBlockSelectionExcludesLineNumbers(t *testing.T) {
	t.Parallel()
	code := "first\nsecond\nthird\n"
	block := ui.CodeBlock(code).LineNumbers(true).CopyButton(false)
	p := ggui.NewProbe(ggui.Column(block), ggui.Sz(300, 200))
	defer p.Close()
	if _, ok := p.Semantics().Find(ggui.RoleButton, "Copy code"); ok {
		t.Fatal("hidden copy action is present")
	}
	if _, ok := p.Semantics().Find(ggui.RoleText, "1\n2\n3\n4"); ok {
		t.Fatal("decorative gutter is announced")
	}
	text := node(t, p.Semantics(), ggui.RoleText, code)
	p.Perform(text.ID, ggui.Action{Kind: ggui.ActionFocus})
	p.Key("cmd+a", "cmd+c")
	if got := p.Clipboard().Read(); got != code {
		t.Fatalf("selection copied %q, want source %q", got, code)
	}
	block.CopyButton(true)
	p.Frame()
	node(t, p.Semantics(), ggui.RoleButton, "Copy code")
}

func TestCodeBlockScrollsBothAxesWithFixedHeader(t *testing.T) {
	t.Parallel()
	code := strings.Repeat(strings.Repeat("long code ", 30)+"\n", 30)
	block := ui.CodeBlock(code).Header("output.log").Height(100).LineNumbers(true)
	p := ggui.NewProbe(ggui.Column(block), ggui.Sz(260, 240))
	defer p.Close()
	before := node(t, p.Semantics(), ggui.RoleText, code)
	button := node(t, p.Semantics(), ggui.RoleButton, "Copy code")
	p.Scroll(ggui.Pt(140, 85), ggui.Pt(-8, 0))
	p.Advance(200 * time.Millisecond)
	horizontal := node(t, p.Semantics(), ggui.RoleText, code)
	if horizontal.Full.Origin.X >= before.Full.Origin.X {
		t.Fatal("long code did not scroll horizontally")
	}
	p.Scroll(ggui.Pt(140, 85), ggui.Pt(0, -8))
	p.Advance(200 * time.Millisecond)
	vertical := node(t, p.Semantics(), ggui.RoleText, code)
	if vertical.Full.Origin.Y >= before.Full.Origin.Y {
		t.Fatal("long code did not scroll vertically")
	}
	if after := node(t, p.Semantics(), ggui.RoleButton, "Copy code"); after.Rect != button.Rect {
		t.Fatal("copy header moved with the code")
	}
	p.Tap("Copy code")
	if p.Clipboard().Read() != code {
		t.Fatal("scrolling changed copied source")
	}
	block.Code("short")
	p.Frame()
	short := node(t, p.Semantics(), ggui.RoleText, "short")
	if short.Full.Origin.X < 0 || short.Full.Origin.Y < button.Rect.Origin.Y {
		t.Fatal("shrinking source did not clamp scroll offsets")
	}
}

func TestCodeBlockEmptyAndNarrowConstraints(t *testing.T) {
	t.Parallel()
	for _, size := range []ggui.Size{ggui.Sz(0, 0), ggui.Sz(24, 20), ggui.Sz(140, 120)} {
		block := ui.CodeBlock("").Header(strings.Repeat("long ", 30)).LineNumbers(true)
		got := block.Layout(ggui.Loose(size), ggui.Env{})
		if got.W < 0 || got.H < 0 || got.W > size.W || got.H > size.H {
			t.Fatalf("size %v escaped %v", got, size)
		}
	}
	p := ggui.NewProbe(ggui.Column(ui.CodeBlock("")), ggui.Sz(180, 100))
	defer p.Close()
	p.Clipboard().Write("old")
	p.Tap("Copy code")
	if p.Clipboard().Read() != "" {
		t.Fatal("empty code did not clear clipboard")
	}
}

func TestCodeBlockWrappedDiffGutterAlignsAndCopiesSource(t *testing.T) {
	t.Parallel()
	first := strings.Repeat("long source with spaces ", 7)
	code := first + "\nadded source\nremoved source\n"
	styles := map[int]ui.CodeLineStyle{1: ui.CodeLineHighlighted, 2: ui.CodeLineAdded, 3: ui.CodeLineRemoved}
	block := ui.CodeBlock(code).LineNumbers(true).StartLine(10).Wrap(true).LineStyles(styles).Height(500)
	styles[2] = ui.CodeLineRemoved // the widget owns a copy
	p := ggui.NewProbe(ggui.Column(block), ggui.Sz(260, 650))
	defer p.Close()
	firstLine := node(t, p.Semantics(), ggui.RoleText, "Highlighted line 10")
	added := node(t, p.Semantics(), ggui.RoleText, "Added line 11")
	removed := node(t, p.Semantics(), ggui.RoleText, "Removed line 12")
	text := node(t, p.Semantics(), ggui.RoleText, code)
	if text.Full.Size.W > 260 || firstLine.Full.Size.H <= added.Full.Size.H {
		t.Fatal("long first line did not wrap", text.Full, firstLine.Full, added.Full)
	}
	if added.Full.Origin.Y != firstLine.Full.Origin.Y+firstLine.Full.Size.H || removed.Full.Origin.Y != added.Full.Origin.Y+added.Full.Size.H {
		t.Fatal("logical line decorations overlap")
	}
	p.Click(ggui.Pt(text.Full.Origin.X+1, added.Full.Origin.Y+2))
	selected := node(t, p.Semantics(), ggui.RoleText, code)
	if selected.SelStart != len(first)+1 {
		t.Fatalf("wrapped gutter does not align with source: selected %d, want %d", selected.SelStart, len(first)+1)
	}
	p.Key("cmd+a", "cmd+c")
	if p.Clipboard().Read() != code {
		t.Fatal("selection included diff markers or line numbers")
	}
	block.Wrap(false)
	p.Frame()
	unwrapped := node(t, p.Semantics(), ggui.RoleText, code)
	if unwrapped.Full.Size.H >= text.Full.Size.H || unwrapped.Full.Size.W <= text.Full.Size.W {
		t.Fatal("Wrap(false) did not restore horizontal source layout")
	}
	block.LineStyles(nil)
	p.Frame()
	if _, ok := p.Semantics().Find(ggui.RoleText, "Added line 11"); ok {
		t.Fatal("clearing decorations left stale annotations")
	}
}
