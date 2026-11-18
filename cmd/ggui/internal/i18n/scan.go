// Package i18n reads what a project has to translate and what it has
// translated: the messages its Go sources pass to the ggui i18n package,
// and the catalogs, Go composite literals or JSON files, that translate
// them. ggui i18n check compares the two, ggui i18n extract fills in what
// a catalog lacks, and ggui build names the catalogs' languages in a
// macOS bundle's Info.plist.
//
// It works on source, with go/parser and go/ast, and imports nothing of
// ggui's: a call is the i18n package's when its selector's package is the
// file's import of ImportPath, whatever name the file gives it. A method
// named S or N is taken for a *i18n.Localizer's when the file imports the
// package and the message's place holds a string literal, which a method
// of another type with those names and arguments would pass for.
package i18n

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// ImportPath is the package whose calls and catalogs are read.
const ImportPath = "github.com/ironpark/ggui/i18n"

// Message is a message the sources pass to the i18n package.
type Message struct {
	// Key is what a catalog stores it under: the source text, followed by
	// " [context]" for a message given a context.
	Key string
	// Source is the text, or for a plural message its singular ("one")
	// form; Other is the plural form.
	Source string
	Other  string
	Plural bool
	// Uses are where it is passed, first first.
	Uses []token.Position
}

// Skipped is a call whose message is not a literal, and so cannot be
// extracted.
type Skipped struct {
	Pos  token.Position
	Call string // as written, such as i18n.T
}

// Sources is what Scan found.
type Sources struct {
	Root string
	// Messages and Plurals are by key.
	Messages map[string]*Message
	Plurals  map[string]*Message
	Skipped  []Skipped
	// Catalogs are the composite literals of i18n.Catalog.
	Catalogs []*Catalog
}

// Keys returns the keys of the messages and of the plural messages,
// sorted.
func (s *Sources) Keys() (messages, plurals []string) {
	return sortedKeys(s.Messages), sortedKeys(s.Plurals)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Key returns the key a message with context ctx is stored under.
func Key(text, ctx string) string {
	if ctx == "" {
		return text
	}
	return text + " [" + ctx + "]"
}

// Scan reads the Go files below root, but for tests, those in vendor and
// testdata directories, in directories whose names begin with . or _, and
// in other modules nested inside, for the messages and catalogs of the
// i18n package. A file that does not parse is an error, after the rest
// are read.
func Scan(root string) (*Sources, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	s := &Sources{Root: root, Messages: map[string]*Message{}, Plurals: map[string]*Message{}}
	var errs []error
	err = walk(root, func(path string) error {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := shared.ReadFile(path)
		if err != nil {
			return err
		}
		if err := s.scanFile(path, src); err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, errors.Join(errs...)
}

// walk calls fn with each file below root, skipping what Scan skips.
func walk(root string, fn func(path string) error) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil // unreadable: nothing of the project's
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == "vendor" || name == "testdata" || name == "node_modules" ||
				strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir // another module
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		return fn(path)
	})
}

// rel names path relative to root, with forward slashes, for messages.
func rel(root, path string) string {
	if r, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(r) {
		return filepath.ToSlash(r)
	}
	return path
}

func (s *Sources) scanFile(path string, src []byte) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, rel(s.Root, path), src, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	name, ok := importName(f)
	if !ok {
		return nil
	}
	// pkg reports whether x names the package: its import's name, or
	// nothing at all for a dot import.
	pkg := func(x ast.Expr, sel string) (string, bool) {
		switch x := x.(type) {
		case *ast.SelectorExpr:
			if id, ok := x.X.(*ast.Ident); ok && name != "." && id.Name == name && x.Sel.Name == sel {
				return name + "." + sel, true
			}
		case *ast.Ident:
			if name == "." && x.Name == sel {
				return sel, true
			}
		}
		return "", false
	}

	// A call's context, from the .Context("...") it is followed by.
	type ctxOf struct {
		text string
		ok   bool // a literal
	}
	contexts := map[*ast.CallExpr]ctxOf{}
	ast.Inspect(f, func(n ast.Node) bool {
		c, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Context" && len(c.Args) == 1 {
			if inner, ok := ast.Unparen(sel.X).(*ast.CallExpr); ok {
				text, lit := literal(c.Args[0])
				contexts[inner] = ctxOf{text, lit}
			}
		}
		return true
	})

	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CompositeLit:
			if _, ok := pkg(n.Type, "Catalog"); ok {
				s.Catalogs = append(s.Catalogs, goCatalog(fset, path, rel(s.Root, path), n))
			}
		case *ast.CallExpr:
			// key is the index of the message's argument; plural, that
			// of a plural message's.
			call, key, plural, method := "", -1, false, false
			if c, ok := pkg(n.Fun, "T"); ok {
				call, key = c, 0
			} else if c, ok := pkg(n.Fun, "N"); ok {
				call, key, plural = c, 1, true
			} else if sel, ok := n.Fun.(*ast.SelectorExpr); ok {
				if id, isPkg := sel.X.(*ast.Ident); !isPkg || id.Name != name {
					switch sel.Sel.Name {
					case "S":
						call, key, method = "."+sel.Sel.Name, 0, true
					case "N":
						call, key, plural, method = "."+sel.Sel.Name, 1, true, true
					}
				}
			}
			if key < 0 {
				return true
			}
			pos := fset.Position(n.Pos())
			need := key + 1
			if plural {
				need++
			}
			if len(n.Args) < need {
				return true
			}
			text, ok := literal(n.Args[key])
			other := ""
			if ok && plural {
				other, ok = literal(n.Args[key+1])
			}
			if !ok {
				// A method is a localizer's only by its literal.
				if !method {
					s.Skipped = append(s.Skipped, Skipped{Pos: pos, Call: call})
				}
				return true
			}
			ctx := ""
			if c, has := contexts[n]; has {
				if !c.ok {
					s.Skipped = append(s.Skipped, Skipped{Pos: pos, Call: call + "(…).Context"})
					return true
				}
				ctx = c.text
			}
			m := s.Messages
			if plural {
				m = s.Plurals
			}
			k := Key(text, ctx)
			msg := m[k]
			if msg == nil {
				msg = &Message{Key: k, Source: text, Other: other, Plural: plural}
				m[k] = msg
			}
			msg.Uses = append(msg.Uses, pos)
		}
		return true
	})
	return nil
}

// importName returns the name a file gives ImportPath, "." for a dot
// import, and whether it imports it.
func importName(f *ast.File) (string, bool) {
	for _, is := range f.Imports {
		p, err := strconv.Unquote(is.Path.Value)
		if err != nil || p != ImportPath {
			continue
		}
		if is.Name != nil {
			if is.Name.Name == "_" {
				return "", false
			}
			return is.Name.Name, true
		}
		return "i18n", true
	}
	return "", false
}

// literal returns the string a constant expression of string literals
// spells, as in "a" or "a" + "b", and whether e is one.
func literal(e ast.Expr) (string, bool) {
	switch e := ast.Unparen(e).(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		s, err := strconv.Unquote(e.Value)
		return s, err == nil
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		a, ok := literal(e.X)
		if !ok {
			return "", false
		}
		b, ok := literal(e.Y)
		return a + b, ok
	}
	return "", false
}

// goCatalog reads a composite literal of i18n.Catalog. What is not a
// literal is noted rather than read: a translation computed by the
// program counts as present, a language that is not is an error.
func goCatalog(fset *token.FileSet, path, name string, lit *ast.CompositeLit) *Catalog {
	c := &Catalog{
		Path: path, Name: name, Line: fset.Position(lit.Pos()).Line, Go: true,
		Messages: map[string]string{}, Plurals: map[string]Forms{},
		dynamic: map[string]bool{},
	}
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			c.Err = errors.New("i18n.Catalog is written without field names; ggui reads Lang, Messages and Plurals by name")
			continue
		}
		field, _ := kv.Key.(*ast.Ident)
		if field == nil {
			continue
		}
		switch field.Name {
		case "Lang":
			lang, ok := literal(kv.Value)
			if !ok {
				c.Err = errors.New("its Lang is not a string literal")
				continue
			}
			c.Lang = lang
		case "Messages":
			m, ok := ast.Unparen(kv.Value).(*ast.CompositeLit)
			if !ok {
				c.Partial = true
				continue
			}
			for _, e := range m.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, ok := literal(kv.Key)
				if !ok {
					c.Partial = true
					continue
				}
				v, ok := literal(kv.Value)
				if !ok {
					c.dynamic[k] = true
				}
				c.Messages[k] = v
			}
		case "Plurals":
			m, ok := ast.Unparen(kv.Value).(*ast.CompositeLit)
			if !ok {
				c.Partial = true
				continue
			}
			for _, e := range m.Elts {
				kv, ok := e.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				k, ok := literal(kv.Key)
				if !ok {
					c.Partial = true
					continue
				}
				forms, ok := goForms(kv.Value)
				if !ok {
					c.dynamic[k] = true
				}
				c.Plurals[k] = forms
			}
		}
	}
	if c.Lang == "" && c.Err == nil {
		c.Err = errors.New("it has no Lang")
	}
	return c
}

// goForms reads a composite literal of i18n.Forms, and reports whether
// each form was a literal.
func goForms(e ast.Expr) (Forms, bool) {
	lit, ok := ast.Unparen(e).(*ast.CompositeLit)
	if !ok {
		return nil, false
	}
	f := Forms{}
	all := true
	for _, e := range lit.Elts {
		kv, ok := e.(*ast.KeyValueExpr)
		if !ok {
			all = false
			continue
		}
		id, ok := kv.Key.(*ast.Ident)
		if !ok {
			all = false
			continue
		}
		v, ok := literal(kv.Value)
		if !ok {
			all = false
		}
		f[strings.ToLower(id.Name)] = v
	}
	return f, all
}
