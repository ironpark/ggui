package ui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// receiverType returns the base type name of a method receiver, dropping the
// pointer and any type parameters: "*ToggleGroupWidget[T]" is "ToggleGroupWidget".
func receiverType(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name
		}
	case *ast.IndexListExpr:
		if id, ok := e.X.(*ast.Ident); ok {
			return id.Name
		}
	}
	return ""
}

// embedsInteractive reports whether the struct has ggui.Interactive as an
// embedded field.
func embedsInteractive(st *ast.StructType) bool {
	for _, f := range st.Fields.List {
		if len(f.Names) > 0 {
			continue
		}
		sel, ok := f.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == "ggui" && sel.Sel.Name == "Interactive" {
			return true
		}
	}
	return false
}

// TestDisabledSettersComeInPairs holds the package to one convention: a
// control built on ggui.Interactive offers Disabled and DisabledWhen
// together or offers neither. Half a pair is the failure this catches — a
// caller that can set the state but not bind it has to rebuild the control
// to change it, which is the thing InertWhen exists to avoid.
//
// Types that do not embed Interactive are left alone: AccordionSection's
// Disabled is a value option on a description, not a control's setter.
func TestDisabledSettersComeInPairs(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatalf("parse ui package: %v", err)
	}
	pkg, ok := pkgs["ui"]
	if !ok {
		t.Fatal("package ui not found")
	}

	controls := map[string]bool{}                    // exported type -> embeds Interactive
	setters := map[string]map[string]*ast.FuncDecl{} // type -> setter name -> decl
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					if st, ok := ts.Type.(*ast.StructType); ok && embedsInteractive(st) {
						controls[ts.Name.Name] = true
					}
				}
			case *ast.FuncDecl:
				if d.Name.Name != "Disabled" && d.Name.Name != "DisabledWhen" {
					continue
				}
				recv := receiverType(d)
				if recv == "" {
					continue
				}
				if setters[recv] == nil {
					setters[recv] = map[string]*ast.FuncDecl{}
				}
				setters[recv][d.Name.Name] = d
			}
		}
	}
	if len(controls) == 0 {
		t.Fatal("no controls found; the scan is broken, not the package")
	}

	for name := range controls {
		got := setters[name]
		_, hasSet := got["Disabled"]
		_, hasBind := got["DisabledWhen"]
		switch {
		case hasSet && !hasBind:
			t.Errorf("%s has Disabled but no DisabledWhen: a caller cannot follow a signal without rebuilding it", name)
		case hasBind && !hasSet:
			t.Errorf("%s has DisabledWhen but no Disabled: a caller cannot set the state outright", name)
		}
	}
}
