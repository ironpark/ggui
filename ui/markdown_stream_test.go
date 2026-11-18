package ui

import (
	"testing"

	"github.com/ironpark/ggui"
)

// A reply streaming in changes its last block alone: the blocks before it
// keep their widgets, and the last one grows in place, rather than every
// block being built again at every piece that arrives.
func TestMarkdownStreamingKeepsEarlierBlocks(t *testing.T) {
	t.Parallel()
	reply := ggui.State("# Plan\n\n- one\n- two\n\nFirst we")
	md := Markdown("").BindContent(reply).Streaming(true)
	p := ggui.NewProbe(md, ggui.Sz(400, 600))
	defer p.Close()
	p.Frame()
	if n := len(md.doc.views); n != 3 {
		t.Fatalf("%d blocks, want 3", n)
	}
	heading, list, para := md.doc.views[0].w, md.doc.views[1].w, md.doc.views[2].w
	built := md.builds

	appendText := func(s string) {
		t.Helper()
		reply.Set(ggui.Peek(reply) + s)
		p.Frame()
	}
	for _, piece := range []string{" parse", " the", " **whole", " reply**", " again."} {
		appendText(piece)
		if md.doc.views[0].w != heading || md.doc.views[1].w != list || md.doc.views[2].w != para {
			t.Fatalf("after %q a block's widget was replaced", piece)
		}
		if md.builds != built {
			t.Fatalf("after %q %d block widgets were built", piece, md.builds-built)
		}
	}
	if got := md.doc.views[2].w.(*mdText).name; got != "First we parse the whole reply again." {
		t.Fatalf("the paragraph shows %q", got)
	}

	// A new block builds its widget, and only it; the code it holds then
	// grows in place.
	appendText("\n\n```go\nfunc")
	if md.builds != built+1 || len(md.doc.views) != 4 {
		t.Fatalf("a new code block built %d widgets, %d blocks", md.builds-built, len(md.doc.views))
	}
	code := md.doc.views[3].w
	appendText(" main() {}\n```")
	if md.doc.views[3].w != code || md.builds != built+1 {
		t.Fatal("the code block was built again as it grew")
	}
	if got := ggui.Peek(&code.(mdCode).code); got != "func main() {}" {
		t.Fatalf("the code block shows %q", got)
	}

	// An item added to the list updates the list, keeping its items.
	item := md.doc.views[1].w.(*mdList).items[0]
	reply.Set("# Plan\n\n- one\n- two\n- three")
	p.Frame()
	if md.doc.views[1].w != list || md.doc.views[1].w.(*mdList).items[0] != item || len(md.doc.views) != 2 {
		t.Fatal("the list was built again for an item more")
	}
}

func TestMarkdownOpensOnlyWebAndMailLinks(t *testing.T) {
	t.Parallel()
	for url, want := range map[string]bool{
		"https://example.com/a?b": true, "http://example.com": true, "mailto:me@example.com": true, "HTTPS://EXAMPLE.COM": true,
		"javascript:alert(1)": false, "file:///etc/passwd": false, "/relative": false, "ftp://example.com": false,
		"http://": false, "mailto:": false, "vscode://open": false, "": false,
	} {
		if got := openable(url); got != want {
			t.Errorf("openable(%q) = %v, want %v", url, got, want)
		}
	}
}
