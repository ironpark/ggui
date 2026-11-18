package i18n

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
)

// Func is a function of the app's own that passes its message on to the
// i18n package, such as tr in
//
//	func tr(key string, args ...any) string { return loc.S(key, args...) }
//
// Its calls are read as i18n.T's are, or for a plural one as i18n.N's.
// It is named by its name alone, tr, which matches a call of tr and of
// any method called tr, or as pkg.Name, which matches a call of Name
// qualified by an import of a package called pkg, by the name the file
// gives that import or by the package's own, or by anything else called
// pkg, and a call of Name unqualified in package pkg itself or in a file
// that dot-imports it.
type Func struct {
	// Spec is the function as it was given, such as tr or ui.T:1.
	Spec string
	// Pkg is the qualifier, "" for a function or method of any package.
	Pkg  string
	Name string
	// Key is the index of the message's argument; Other, of a plural
	// message's plural form.
	Key, Other int
	Plural     bool
}

// Call is the function's name as reports give it: pkg.Name or Name.
func (f Func) Call() string {
	if f.Pkg == "" {
		return f.Name
	}
	return f.Pkg + "." + f.Name
}

// ParseFunc reads a function as --func gives it: NAME or NAME:POS, POS
// being the index of the message among its arguments, 0 by default. With
// plural it reads one as --plural gives it: NAME or NAME:ONE,OTHER, the
// indexes of the singular and the plural form, 1 and 2 by default, after
// the count, as i18n.N has them. NAME is a function's or method's name,
// or pkg.Name.
func ParseFunc(spec string, plural bool) (Func, error) {
	f := Func{Spec: spec, Plural: plural, Key: 0, Other: -1}
	if plural {
		f.Key, f.Other = 1, 2
	}
	name, pos, hasPos := strings.Cut(spec, ":")
	if strings.ContainsAny(name, "()*") {
		return f, fmt.Errorf("%q: a method is named by its name alone, such as tr, which matches a method of any type called so", spec)
	}
	qual, n, dotted := strings.Cut(name, ".")
	if !dotted {
		qual, n = "", name
	}
	if !token.IsIdentifier(n) || (dotted && !token.IsIdentifier(qual)) {
		return f, fmt.Errorf("%q: give a name, such as tr, or a package and a name, such as ui.T", spec)
	}
	f.Pkg, f.Name = qual, n
	if !hasPos {
		return f, nil
	}
	index := func(s string) (int, bool) {
		i, err := strconv.Atoi(s)
		return i, err == nil && i >= 0
	}
	if !plural {
		i, ok := index(pos)
		if !ok {
			return f, fmt.Errorf("%q: after the colon comes the index of the message's argument, from 0, such as tr:1", spec)
		}
		f.Key = i
		return f, nil
	}
	a, b, two := strings.Cut(pos, ",")
	one, ok1 := index(a)
	other, ok2 := index(b)
	if !two || !ok1 || !ok2 || one == other {
		return f, fmt.Errorf("%q: after the colon come the indexes of the singular and the plural form's arguments, from 0, such as trn:1,2", spec)
	}
	f.Key, f.Other = one, other
	return f, nil
}

// ParseFuncs reads the functions of --func and the plural ones of
// --plural, or of ggui.toml's funcs and plurals.
func ParseFuncs(funcs, plurals []string) ([]Func, error) {
	var out []Func
	var errs []error
	for i, list := range [][]string{funcs, plurals} {
		for _, spec := range list {
			f, err := ParseFunc(spec, i == 1)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			out = append(out, f)
		}
	}
	return out, errors.Join(errs...)
}

// fileInfo is what matching a call to a Func needs of its file.
type fileInfo struct {
	pkg string // the package clause's name
	// imports maps the name each import has in the file to its package's
	// name, guessed from its path; dots holds the names of those
	// imported with a dot.
	imports map[string]string
	dots    map[string]bool
}

func newFileInfo(f *ast.File) *fileInfo {
	fi := &fileInfo{pkg: f.Name.Name, imports: map[string]string{}, dots: map[string]bool{}}
	for _, is := range f.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err != nil {
			continue
		}
		name := pathName(p)
		switch {
		case is.Name == nil:
			fi.imports[name] = name
		case is.Name.Name == ".":
			fi.dots[name] = true
		case is.Name.Name != "_":
			fi.imports[is.Name.Name] = name
		}
	}
	return fi
}

// pathName guesses the name of the package at an import path: its last
// element, or the one before a major version such as v2, up to a dot, as
// in gopkg.in/yaml.v3, and without a go- prefix.
func pathName(p string) string {
	elems := strings.Split(p, "/")
	last := elems[len(elems)-1]
	if len(elems) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = elems[len(elems)-2]
	}
	last, _, _ = strings.Cut(last, ".")
	last = strings.TrimPrefix(last, "go-")
	return strings.ReplaceAll(last, "-", "_")
}

// matches reports whether call is a call of f.
func (fi *fileInfo) matches(f Func, fun ast.Expr) bool {
	switch fun := fun.(type) {
	case *ast.Ident:
		if fun.Name != f.Name {
			return false
		}
		return f.Pkg == "" || f.Pkg == fi.pkg || fi.dots[f.Pkg]
	case *ast.SelectorExpr:
		if fun.Sel.Name != f.Name {
			return false
		}
		if f.Pkg == "" {
			return true
		}
		x, ok := fun.X.(*ast.Ident)
		if !ok {
			return false
		}
		if name, imported := fi.imports[x.Name]; imported {
			return name == f.Pkg || x.Name == f.Pkg
		}
		return x.Name == f.Pkg
	}
	return false
}

// declares reports whether a function called name, declared in the file,
// is f.
func (fi *fileInfo) declares(f Func, name string) bool {
	return name == f.Name && (f.Pkg == "" || f.Pkg == fi.pkg)
}
