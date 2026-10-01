package ui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	uitheme "github.com/ironpark/ggui/ui/theme"
)

// TestEveryThemeTokenIsDrawn guards against a token a palette sets that no
// control reads, as DestructiveFg and AccentFg once were: every exported
// Theme field must be read by a control in this package, or mapped onto the
// core by Apply, or read by a state helper such as Theme.Disabled.
func TestEveryThemeTokenIsDrawn(t *testing.T) {
	t.Parallel()
	read := map[string]bool{}
	files, _ := filepath.Glob("*.go")
	files = append(files, filepath.Join("theme", "environment.go"), filepath.Join("theme", "color.go"))
	fset := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		f, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				read[sel.Sel.Name] = true
			}
			return true
		})
	}
	typ := reflect.TypeFor[uitheme.Theme]()
	for i := range typ.NumField() {
		if f := typ.Field(i); f.IsExported() && !read[f.Name] {
			t.Errorf("Theme.%s is never read by a control", f.Name)
		}
	}
}
