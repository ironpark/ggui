package ui_test

import (
	"bytes"
	"go/ast"
	"go/format"
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

// Controls built on Interactive, including indirect embeddings, must expose
// their own fluent Key rather than only the implementation-level SetKey.
func TestPublicKeySettersReturnTheirOwnControl(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]*ast.StructType{}
	keys := map[string]*ast.FuncDecl{}
	for _, file := range pkgs["ui"].Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.Name == "Key" && receiverType(d) != "" {
					keys[receiverType(d)] = d
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						if st, ok := ts.Type.(*ast.StructType); ok {
							types[ts.Name.Name] = st
						}
					}
				}
			}
		}
	}
	var base func(ast.Expr) string
	base = func(e ast.Expr) string {
		switch e := e.(type) {
		case *ast.Ident:
			return e.Name
		case *ast.StarExpr:
			return base(e.X)
		case *ast.IndexExpr:
			return base(e.X)
		case *ast.IndexListExpr:
			return base(e.X)
		case *ast.SelectorExpr:
			return base(e.X) + "." + e.Sel.Name
		}
		return ""
	}
	var hasKey func(string, map[string]bool) bool
	hasKey = func(name string, visiting map[string]bool) bool {
		if name == "ggui.Interactive" || keys[name] != nil {
			return true
		}
		if visiting[name] || types[name] == nil {
			return false
		}
		visiting[name] = true
		defer delete(visiting, name)
		for _, f := range types[name].Fields.List {
			if len(f.Names) == 0 && hasKey(base(f.Type), visiting) {
				return true
			}
		}
		return false
	}
	printed := func(e ast.Expr) string {
		var b bytes.Buffer
		if err := format.Node(&b, fset, e); err != nil {
			t.Fatal(err)
		}
		return b.String()
	}
	checked := 0
	for name := range types {
		if !ast.IsExported(name) || !hasKey(name, map[string]bool{}) {
			continue
		}
		checked++
		method := keys[name]
		if method == nil {
			t.Errorf("%s inherits Key without returning its own pointer type", name)
			continue
		}
		if method.Type.Results == nil || len(method.Type.Results.List) != 1 ||
			printed(method.Type.Results.List[0].Type) != printed(method.Recv.List[0].Type) {
			t.Errorf("%s.Key must return its receiver type", name)
		}
	}
	if checked == 0 {
		t.Fatal("no keyed controls found")
	}
}

// TestDisabledSettersComeInPairs checks every exported pointer-receiver
// Disabled setter, including wrappers and indirectly embedded controls.
// Description values such as CommandEntry and AccordionSection have value
// receivers and intentionally keep their static Disabled option.
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

	setters := map[string]map[string]*ast.FuncDecl{}
	for _, file := range pkg.Files {
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				if d.Name.Name != "Disabled" && d.Name.Name != "BindDisabled" {
					continue
				}
				recv := receiverType(d)
				if recv == "" {
					continue
				}
				if _, pointer := d.Recv.List[0].Type.(*ast.StarExpr); !pointer || !ast.IsExported(recv) {
					continue
				}
				if setters[recv] == nil {
					setters[recv] = map[string]*ast.FuncDecl{}
				}
				setters[recv][d.Name.Name] = d
			}
		}
	}
	if len(setters) == 0 {
		t.Fatal("no controls found; the scan is broken, not the package")
	}

	for name, got := range setters {
		_, hasSet := got["Disabled"]
		_, hasBind := got["BindDisabled"]
		switch {
		case hasSet && !hasBind:
			t.Errorf("%s has Disabled but no BindDisabled: a caller cannot follow a signal without rebuilding it", name)
		case hasBind && !hasSet:
			t.Errorf("%s has BindDisabled but no Disabled: a caller cannot set the state outright", name)
		}
	}
}

// Reactive property methods are discoverable by one prefix and keep chaining
// on the concrete widget. The literal counterpart uses the same value type.
func TestBindingSettersHaveTypedFluentCounterparts(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	methods := map[string]map[string]*ast.FuncDecl{}
	print := func(n ast.Node) string { var b bytes.Buffer; format.Node(&b, fset, n); return b.String() }
	for _, file := range pkgs["ui"].Files {
		for _, decl := range file.Decls {
			f, ok := decl.(*ast.FuncDecl)
			if !ok || f.Recv == nil {
				continue
			}
			typ := receiverType(f)
			if methods[typ] == nil {
				methods[typ] = map[string]*ast.FuncDecl{}
			}
			methods[typ][f.Name.Name] = f
		}
	}
	checked := 0
	for typ, ms := range methods {
		for name, f := range ms {
			if !strings.HasPrefix(name, "Bind") || f.Type.Params == nil || len(f.Type.Params.List) != 1 {
				continue
			}
			arg, ok := f.Type.Params.List[0].Type.(*ast.IndexExpr)
			if !ok {
				continue
			}
			base := print(arg.X)
			if base != "ggui.Readable" && base != "ggui.Binding" {
				continue
			}
			checked++
			if f.Type.Results == nil || len(f.Type.Results.List) != 1 || print(f.Type.Results.List[0].Type) != print(f.Recv.List[0].Type) {
				t.Errorf("%s.%s must return its concrete receiver", typ, name)
			}
			literal := ms[strings.TrimPrefix(name, "Bind")]
			if literal == nil || literal.Type.Params == nil || len(literal.Type.Params.List) != 1 {
				t.Errorf("%s.%s has no literal counterpart", typ, name)
				continue
			}
			if print(literal.Type.Params.List[0].Type) != print(arg.Index) {
				t.Errorf("%s.%s literal type mismatch", typ, name)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no bindings checked")
	}
}
