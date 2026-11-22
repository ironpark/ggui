package main

import (
	"testing"

	"github.com/ironpark/ggui"
)

func TestGalleryMarkdownStreams(t *testing.T) {
	t.Parallel()
	p := galleryProbe(ggui.Sz(1180, 820))
	defer p.Close()
	searchGallery(p, "Markdown")
	if _, ok := p.Semantics().Find(ggui.RoleHeading, "Streaming a reply"); !ok {
		t.Fatal("the reply's heading is missing")
	}
	revealGallery(p, "chat guide")
	p.Tap("chat guide")
	if _, ok := p.Semantics().Find(ggui.RoleText, "Link tapped: https://github.com/ironpark/ggui/blob/main/docs/chat.md"); !ok {
		t.Fatal("the link did not reach OnLink")
	}
	revealGallery(p, "Stream a piece")
	p.Tap("Stream a piece")
	if _, ok := p.Semantics().Find(ggui.RoleHeading, "Streaming a"); !ok {
		t.Fatal("the stream did not restart from its first piece")
	}
	for range 3 {
		p.Tap("Stream a piece")
	}
	if _, ok := p.Semantics().Find(ggui.RoleHeading, "Streaming a reply"); !ok {
		t.Fatal("the heading did not grow as the stream went on")
	}
	p.Tap("Show it all")
	if _, ok := p.Semantics().Find(ggui.RoleCell, "CodeBlock"); !ok {
		t.Fatal("the whole reply's table is missing")
	}
}
