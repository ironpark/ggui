package ui_test

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

const markdownDoc = "# Release notes\n\n" +
	"Read the [upgrade guide](https://example.com/guide) before you *start*.\n\n" +
	"## Changes\n\n" +
	"- [x] Faster layout\n- [ ] Docs\n\n" +
	"1. First\n2. Second\n   - nested\n\n" +
	"> Quoted **advice**.\n\n" +
	"```go\nfmt.Println(\"hi\")\n```\n\n" +
	"---\n\n" +
	"| Name | Size |\n|:-----|-----:|\n| core | 12 |\n| ui | 30 |\n"

func TestMarkdownDescribesItsBlocks(t *testing.T) {
	t.Parallel()
	var opened []string
	md := ui.Markdown(markdownDoc).OnLink(func(url string) { opened = append(opened, url) })
	p := ggui.NewProbe(md, ggui.Sz(520, 900))
	defer p.Close()
	sem := p.Semantics()

	h1 := node(t, sem, ggui.RoleHeading, "Release notes")
	if h1.Level != 1 {
		t.Errorf("the title is a heading of level %d, want 1", h1.Level)
	}
	if h2 := node(t, sem, ggui.RoleHeading, "Changes"); h2.Level != 2 {
		t.Errorf("the section is a heading of level %d, want 2", h2.Level)
	}
	if done := node(t, sem, ggui.RoleCheckbox, "Faster layout"); done.Checked != ggui.TriOn || done.Actions != 0 {
		t.Errorf("a done task is %v with actions %v, want ticked and read only", done.Checked, done.Actions)
	}
	if open := node(t, sem, ggui.RoleCheckbox, "Docs"); open.Checked != ggui.TriOff {
		t.Errorf("an open task is %v, want unticked", open.Checked)
	}
	lists, items := 0, 0
	for range sem.Nodes(ggui.RoleList) {
		lists++
	}
	for range sem.Nodes(ggui.RoleListItem) {
		items++
	}
	if lists != 3 || items != 5 {
		t.Errorf("%d lists of %d items, want 3 of 5", lists, items)
	}
	node(t, sem, ggui.RoleGroup, "go")
	node(t, sem, ggui.RoleTable, "")
	node(t, sem, ggui.RoleHeader, "Size")
	node(t, sem, ggui.RoleCell, "30")
	node(t, sem, ggui.RoleSeparator, "")

	p.Tap("upgrade guide")
	if len(opened) != 1 || opened[0] != "https://example.com/guide" {
		t.Fatalf("tapping the link opened %q", opened)
	}
	p.Tap("Copy code")
	if got := p.Clipboard().Read(); got != `fmt.Println("hi")` {
		t.Fatalf("the code block copied %q", got)
	}
}

func TestMarkdownStreamsIntoItsLastBlock(t *testing.T) {
	t.Parallel()
	reply := ggui.State("Sure. Run **this")
	streaming := ggui.State(true)
	p := ggui.NewProbe(ui.Markdown("").BindContent(reply).BindStreaming(streaming), ggui.Sz(400, 400))
	defer p.Close()
	// Mid-stream the unclosed strong span shows as strong, not as **.
	node(t, p.Semantics(), ggui.RoleText, "Sure. Run this")

	reply.Set(ggui.Peek(reply) + "** first:\n\n```sh\ngo te")
	code := node(t, p.Semantics(), ggui.RoleText, "go te")
	reply.Set(ggui.Peek(reply) + "st ./...")
	if grown := node(t, p.Semantics(), ggui.RoleText, "go test ./..."); grown.Full.Origin != code.Full.Origin {
		t.Fatal("the open fence's code moved as it grew")
	}
	p.Tap("Copy code")
	if got := p.Clipboard().Read(); got != "go test ./..." {
		t.Fatalf("the streaming code block copied %q", got)
	}

	// Ended, a source that never closed reads as written.
	reply.Set("**unclosed")
	streaming.Set(false)
	node(t, p.Semantics(), ggui.RoleText, "**unclosed")
}

// Raw HTML is text, and a link of a scheme the default does not open
// still calls OnLink, which decides.
func TestMarkdownShowsHTMLAsText(t *testing.T) {
	t.Parallel()
	var opened []string
	md := ui.Markdown("<script>alert(1)</script> and <javascript:alert(1)>").
		OnLink(func(url string) { opened = append(opened, url) })
	p := ggui.NewProbe(md, ggui.Sz(400, 200))
	defer p.Close()
	node(t, p.Semantics(), ggui.RoleText, "<script>alert(1)</script> and javascript:alert(1)")
	p.Tap("javascript:alert(1)")
	if len(opened) != 1 || opened[0] != "javascript:alert(1)" {
		t.Fatalf("OnLink got %q", opened)
	}
}

// At a narrow width nothing is laid out past the edge: long words wrap,
// a table's columns share the width, code scrolls within its block.
func TestMarkdownFitsANarrowWidth(t *testing.T) {
	t.Parallel()
	const w = 140
	src := "# A heading long enough to wrap twice over\n\n" +
		"Averyveryverylongwordwithnospaces and more text after it.\n\n" +
		"- item with a fairly long line of text\n  - nested deeper item text\n    1. and deeper still\n\n" +
		"> quote > quote text that goes on\n\n" +
		"```\na line of code far longer than the width of the column it is in\n```\n\n" +
		"| one | two | three | four |\n|---|---|---|---|\n| alpha beta | gamma delta | epsilon | zeta eta theta |\n"
	p := ggui.NewProbe(widget.Column(ui.Markdown(src)).Stretch(), ggui.Sz(w, 2000))
	defer p.Close()
	sem := p.Semantics()
	seen := map[ggui.Role]bool{}
	for _, n := range sem.All() {
		if n.Role == ggui.RoleText && strings.HasPrefix(n.Name, "a line of code") {
			continue // scrolls inside its block
		}
		r := n.Full
		if r.Size.W < 0 || r.Size.H < 0 || r.Origin.X < -0.5 || r.Origin.X+r.Size.W > w+0.5 {
			t.Errorf("%s %q is laid out at %v, outside 0 to %d", n.Role, n.Name, r, w)
		}
		seen[n.Role] = true
	}
	for _, role := range []ggui.Role{ggui.RoleHeading, ggui.RoleList, ggui.RoleTable, ggui.RoleCell, ggui.RoleGroup} {
		if !seen[role] {
			t.Errorf("no %s laid out", role)
		}
	}
	// The table's cells wrap rather than overflow, so its rows grow.
	cell := node(t, sem, ggui.RoleCell, "zeta eta theta")
	head := node(t, sem, ggui.RoleHeader, "four")
	if cell.Rect.Size.H <= head.Rect.Size.H {
		t.Errorf("a long cell is %v tall, its heading %v", cell.Rect.Size.H, head.Rect.Size.H)
	}
}

// A short reply is as wide as its text; one holding a block that spans
// the width fills it.
func TestMarkdownHugsShortText(t *testing.T) {
	t.Parallel()
	short := ui.Markdown("Hi *there*")
	block := ui.Markdown("Hi\n\n---")
	p := ggui.NewProbe(widget.Column(short, block), ggui.Sz(400, 200))
	defer p.Close()
	sem := p.Semantics()
	if text := node(t, sem, ggui.RoleText, "Hi there"); text.Rect.Size.W >= 200 {
		t.Errorf("a short paragraph is %v wide", text.Rect.Size.W)
	}
	if rule := node(t, sem, ggui.RoleSeparator, ""); rule.Rect.Size.W != 400 {
		t.Errorf("a rule is %v wide, want the width", rule.Rect.Size.W)
	}
}

func TestMarkdownSettersReplaceTheSource(t *testing.T) {
	t.Parallel()
	src := ggui.State("bound")
	md := ui.Markdown("literal").BindContent(src)
	p := ggui.NewProbe(md, ggui.Sz(300, 100))
	defer p.Close()
	node(t, p.Semantics(), ggui.RoleText, "bound")
	md.Content("**literal**")
	src.Set("detached")
	node(t, p.Semantics(), ggui.RoleText, "literal")
	md.Selectable(false).Size(20)
	node(t, p.Semantics(), ggui.RoleText, "literal")
}
