package ui_test

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// drawsItsOwnImages are the widgets that build an image themselves, scaling
// and blending through GeoM and ColorScale. ggui re-exports the keys, cursors
// and buttons a widget names, but not drawing, so these three still reach for
// Ebitengine. They are where a drawing API of ggui's own would begin.
var drawsItsOwnImages = []string{"attachment.go", "avatar.go", "toast.go"}

// TestOnlyImageWidgetsImportEbiten keeps the rest of the package on ggui's own
// surface: a widget author imports one package, and what renders is ggui's
// business rather than every widget's.
func TestOnlyImageWidgetsImportEbiten(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			name := path[strings.LastIndex(path, "/")+1:]
			for _, imp := range file.Imports {
				if !strings.Contains(imp.Path.Value, "hajimehoshi/ebiten") {
					continue
				}
				if slices.Contains(drawsItsOwnImages, name) {
					continue
				}
				t.Errorf("%s imports %s; name it through ggui instead", fset.Position(imp.Pos()), imp.Path.Value)
			}
		}
	}
}
