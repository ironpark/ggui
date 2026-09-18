package main

import (
	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
	"testing"
)

func TestGalleryFiltersAndGlobalActions(t *testing.T) {
	dark, search, category, scroll := ggui.State(false), ggui.State(""), ggui.State("All"), ggui.State(0.0)
	commands := 0
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return galleryPage(dark, search, category, scroll, func() { commands++ }, []ggui.Widget{
			preview("Buttons", ui.Button("Input sample", nil)),
			preview("Resizable", ui.Button("Layout sample", nil)),
			ui.Button("Global action", nil),
		})
	}, ggui.Sz(1180, 820))
	defer p.Close()
	p.Frame()
	p.Tap("Layout")
	p.Frame()
	if category.Peek() != "Layout" {
		t.Fatal("category not selected")
	}
	if _, ok := p.Find("Input sample"); ok {
		t.Fatal("filtered input remains interactive")
	}
	if _, ok := p.Find("Layout sample"); !ok {
		t.Fatal("matching preview missing")
	}
	if _, ok := p.Find("Global action"); !ok {
		t.Fatal("global action filtered out")
	}
	p.Tap("Commands")
	if commands != 1 {
		t.Fatal("global commands not available")
	}
	search.Set("unmatched")
	p.Frame()
	p.Tap("Clear filters")
	p.Frame()
	if search.Peek() != "" || category.Peek() != "All" {
		t.Fatal("filters not cleared")
	}
	search.Set("  PRIMARY  ")
	p.Frame()
	if _, ok := p.Find("Input sample"); !ok {
		t.Fatal("description search must ignore case and outer whitespace")
	}
	if _, ok := p.Find("Layout sample"); ok {
		t.Fatal("search did not filter")
	}
}

func TestPreviewGridReflows(t *testing.T) {
	grid := &previewGrid{children: []ggui.Widget{
		preview("Buttons", ui.Button("First", nil)),
		preview("Text", ui.Button("Second", nil)),
	}}
	p := ggui.NewProbe(ggui.Scroll(grid), ggui.Sz(1120, 820))
	defer p.Close()
	p.Frame()
	first, _ := p.Find("First")
	second, _ := p.Find("Second")
	if second.Rect.Origin.X <= first.Rect.Origin.X {
		t.Fatal("wide window should have two columns")
	}
	p.Resize(ggui.Sz(600, 820))
	p.Frame()
	first, _ = p.Find("First")
	second, _ = p.Find("Second")
	if second.Rect.Origin.X != first.Rect.Origin.X || second.Rect.Origin.Y <= first.Rect.Origin.Y {
		t.Fatal("narrow window should stack cards")
	}
}

func TestGallerySearchPasteAcrossRebuilds(t *testing.T) {
	dark, search, category, scroll := ggui.State(false), ggui.State(""), ggui.State("All"), ggui.State(0.0)
	p := ggui.ProbeBuilder(func() ggui.Widget {
		return galleryPage(dark, search, category, scroll, func() {}, []ggui.Widget{preview("Buttons", ui.Button("Sample", nil))})
	}, ggui.Sz(1180, 820))
	defer p.Close()
	p.Tap("Search components")
	pasteText(p, "Accordion")
	if search.Peek() != "Accordion" {
		t.Fatalf("search text lost: %q", search.Peek())
	}
	p.Tap("All")
	p.Tap("Search components")
	p.Type(ggui.Mods{Meta: true}, ggui.KeyA)
	pasteText(p, "Text")
	if search.Peek() != "Text" {
		t.Fatalf("second search text lost: %q", search.Peek())
	}
}
