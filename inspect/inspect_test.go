package inspect

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui/a11y"
	"github.com/ironpark/ggui/geom"
)

func node(name string, depth int, x, y, w, h float64) Node {
	return Node{Name: name, Depth: depth, Rect: geom.Rct(geom.Pt(x, y), geom.Sz(w, h))}
}

// fakeSource answers as an app would, counting the calls so a test can
// see what the frame remembers.
type fakeSource struct {
	describes int
	boxes     map[int]Box
	semantic  int
}

func (s *fakeSource) Describe(i int) (string, string) { s.describes++; return "label", "button" }
func (s *fakeSource) Fields(int) []Field {
	return []Field{{Key: "Own"}, {Key: "x", Value: "1", Number: true}}
}
func (s *fakeSource) Semantic(int) int      { return s.semantic }
func (s *fakeSource) Box(i int) (Box, bool) { b, ok := s.boxes[i]; return b, ok }

func TestFrameRemembersWhatItAskedFor(t *testing.T) {
	src := &fakeSource{}
	f := &Frame{Nodes: []Node{node("Button", 0, 0, 0, 10, 10)}, Source: src}
	f.Describe(0)
	f.Describe(0)
	if src.describes != 1 || f.Nodes[0].Label != "label" || f.Badge(0) != "button" {
		t.Fatalf("described %d times, node %+v", src.describes, f.Nodes[0])
	}
	if !f.Matches(0, "lab") || f.Matches(0, "zzz") {
		t.Fatal("filter did not match on the label")
	}
	f.Nodes[0].Kind = "flex"
	if f.Badge(0) != "flex" {
		t.Fatal("layout kind should win over the role as a badge")
	}
}

func TestTreeHelpers(t *testing.T) {
	nodes := []Node{node("Column", 0, 0, 0, 100, 100), node("Box", 1, 10, 10, 80, 80), node("Text", 2, 20, 20, 30, 10), node("Other", 1, 0, 90, 100, 10)}
	nodes[2].Clipped, nodes[2].Clip = true, geom.Rct(geom.Pt(20, 20), geom.Sz(5, 5))
	if got := Deepest(nodes, geom.Pt(21, 21)); got != 2 {
		t.Fatalf("deepest = %d, want the Text", got)
	}
	if got := Deepest(nodes, geom.Pt(40, 25)); got != 1 {
		t.Fatalf("deepest = %d, want the Box: the Text is clipped away there", got)
	}
	if !HasChildren(nodes, 0) || HasChildren(nodes, 2) || HasChildren(nodes, 3) {
		t.Fatal("HasChildren is wrong")
	}
	if got := Ancestors(nodes, 2); !slices.Equal(got, []int{1, 0}) {
		t.Fatalf("ancestors = %v", got)
	}
}

func TestDetailsPanes(t *testing.T) {
	src := &fakeSource{boxes: map[int]Box{1: {Padding: Insets{Top: 4, Right: 4, Bottom: 4, Left: 4}, Border: 1, Content: geom.Sz(72, 72), Valid: true}}, semantic: 0}
	sem := a11y.Build([]a11y.SemNode{{Node: a11y.Node{Role: a11y.RoleButton, Name: "Save", Checked: a11y.TriOn}, Rect: geom.Rct(geom.Pt(0, 0), geom.Sz(100, 100)), Parent: -1}}, -1)
	f := &Frame{Nodes: []Node{node("Button", 0, 0, 0, 100, 100), node("Box", 1, 0, 0, 100, 100)}, Sem: sem, Source: src}
	f.Nodes[0].ID = "save"

	has := func(fields []Field, key, value string) bool {
		return slices.ContainsFunc(fields, func(fl Field) bool { return fl.Key == key && fl.Value == value })
	}
	layout := f.Details(Layout, 0)
	if !has(layout, "children", "1") || !has(layout, "parent", "root") || !has(layout, "identity", "save") {
		t.Fatalf("layout = %+v", layout)
	}
	computed := f.Details(Computed, 0)
	if !has(computed, "type", "Button") || !has(computed, "label", "label") || !has(computed, "x", "1") {
		t.Fatalf("computed = %+v", computed)
	}
	semantics := f.Details(Semantics, 0)
	if !has(semantics, "role", "button") || !has(semantics, "name", "Save") || !has(semantics, "checked", "true") {
		t.Fatalf("semantics = %+v", semantics)
	}
	// A control's decorated Box of the same bounds stands in for its box model.
	if box := f.BoxOf(0); !box.Valid || box.Border != 1 {
		t.Fatalf("box of the control = %+v, want its child Box's", box)
	}
	if got := f.SemanticAt(geom.Pt(5, 5)); got != 0 {
		t.Fatalf("SemanticAt = %d", got)
	}
	if f.Details(Layout, 7) != nil || (&Frame{}).SemanticAt(geom.Pt(0, 0)) != -1 {
		t.Fatal("out-of-range and empty frames should answer nothing")
	}
	if Num(1.23456) != "1.23" || Color(nil) != "none" {
		t.Fatal("formatting helpers changed")
	}
}
