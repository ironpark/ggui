package ui_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/gfxtest"
)

// The tests paint with no renderer: see gfxtest.MainHeadless.
func TestMain(m *testing.M) { gfxtest.MainHeadless(m) }

// showsImages are the widgets that take an image to show. An image is a
// *ggfx.Image, which ggui does not rename, but they draw it through the
// Canvas like everything else.
var showsImages = []string{"avatar.go", "chat/attachment.go"}

// TestOnlyImageWidgetsImportGgfx keeps the rest of the package on ggui's own
// surface: a widget author imports one package, and what renders is ggui's
// business rather than every widget's.
func TestOnlyImageWidgetsImportGgfx(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	for name, file := range parseControlSources(t, fset, parser.ImportsOnly, true) {
		for _, imp := range file.Imports {
			if !strings.Contains(imp.Path.Value, "ironpark/ggfx") {
				continue
			}
			if slices.Contains(showsImages, filepath.ToSlash(name)) {
				continue
			}
			t.Errorf("%s imports %s; name it through ggui instead", fset.Position(imp.Pos()), imp.Path.Value)
		}
	}
}

// TestUIIsWrittenOnThePublicAPI keeps the themed library, its themes and its
// icon sets on what a module outside ggui can import: a control here is what
// another design system could have written, and what it needs and cannot
// reach is missing from the public API rather than borrowed from internal.
// Tests are exempt; they start the headless renderer.
func TestUIIsWrittenOnThePublicAPI(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			if strings.Contains(imp.Path.Value, "ironpark/ggui/internal/") {
				t.Errorf("%s imports %s; ui is written on ggui's public API", fset.Position(imp.Pos()), imp.Path.Value)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
