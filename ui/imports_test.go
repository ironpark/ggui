package ui_test

import (
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// showsImages are the widgets that take an image to show. An image is a
// *ggfx.Image, which ggui does not rename, but they draw it through the
// Canvas like everything else.
var showsImages = []string{"attachment.go", "avatar.go"}

// TestOnlyImageWidgetsImportGgfx keeps the rest of the package on ggui's own
// surface: a widget author imports one package, and what renders is ggui's
// business rather than every widget's.
func TestOnlyImageWidgetsImportGgfx(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			name := path[strings.LastIndex(path, "/")+1:]
			for _, imp := range file.Imports {
				if !strings.Contains(imp.Path.Value, "ironpark/ggfx") {
					continue
				}
				if slices.Contains(showsImages, name) {
					continue
				}
				t.Errorf("%s imports %s; name it through ggui instead", fset.Position(imp.Pos()), imp.Path.Value)
			}
		}
	}
}
