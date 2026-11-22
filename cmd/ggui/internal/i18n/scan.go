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
// of another type with those names and arguments would pass for. The
// app's own functions that wrap them, its Funcs, are matched by name, in
// any file.
package i18n

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
// i18n package, and for the messages passed to funcs. A file that does
// not parse is an error, after the rest are read.
func Scan(root string, funcs ...Func) (*Sources, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	s := &Sources{Root: root, Messages: map[string]*Message{}, Plurals: map[string]*Message{}}
	var errs []error
	var files []*parsed
	err = walk(root, func(path string) error {
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := shared.ReadFile(path)
		if err != nil {
			return err
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, rel(root, path), src, parser.SkipObjectResolution)
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		files = append(files, &parsed{path: path, fset: fset, file: f})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// A message may be a constant of its package, declared in any of its
	// files.
	type pkgKey struct{ dir, name string }
	consts := map[pkgKey]map[string]ast.Expr{}
	for _, p := range files {
		k := pkgKey{filepath.Dir(p.path), p.file.Name.Name}
		if consts[k] == nil {
			consts[k] = map[string]ast.Expr{}
		}
		for _, d := range p.file.Decls {
			if d, ok := d.(*ast.GenDecl); ok {
				addConsts(consts[k], d)
			}
		}
	}
	for _, p := range files {
		p.consts = consts[pkgKey{filepath.Dir(p.path), p.file.Name.Name}]
		s.scanFile(p, funcs)
	}
	return s, errors.Join(errs...)
}

// parsed is a Go file read.
type parsed struct {
	path   string
	fset   *token.FileSet
	file   *ast.File
	consts map[string]ast.Expr // its package's, by name
}

// addConsts adds the string constants decl declares to consts, by the
// expression of each, which is nil for a name declared twice, as by files
// for different systems, with different values.
func addConsts(consts map[string]ast.Expr, decl *ast.GenDecl) {
	if decl.Tok != token.CONST {
		return
	}
	// A spec without values repeats the one before's.
	var last []ast.Expr
	for _, spec := range decl.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		if len(vs.Values) > 0 {
			last = vs.Values
		}
		if len(last) != len(vs.Names) {
			continue
		}
		for i, n := range vs.Names {
			if prev, seen := consts[n.Name]; seen {
				if prev == nil || types.ExprString(prev) != types.ExprString(last[i]) {
					consts[n.Name] = nil
				}
				continue
			}
			consts[n.Name] = last[i]
		}
	}
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

func (s *Sources) scanFile(p *parsed, funcs []Func) {
	name, imported := importName(p.file)
	if !imported && len(funcs) == 0 {
		return
	}
	sc := &fileScan{
		s: s, p: p, funcs: funcs, name: name, imported: imported,
		contexts: map[*ast.CallExpr]ast.Expr{}, litNames: map[*ast.FuncLit]string{},
		scopes: map[ast.Node]*scope{},
	}
	sc.collectContexts()
	ast.PreorderStack(p.file, nil, sc.visit)
}

// fileScan is the state of one file's scan: the sources it fills, the
// parsed file, the app's functions, and what the file's calls need.
type fileScan struct {
	s        *Sources
	p        *parsed
	funcs    []Func
	name     string // the name the file gives i18n, "." for a dot import
	imported bool
	fi       *fileInfo

	contexts map[*ast.CallExpr]ast.Expr
	litNames map[*ast.FuncLit]string
	scopes   map[ast.Node]*scope
}

// pkg reports whether x names the i18n package: its import's name, or
// nothing at all for a dot import.
func (sc *fileScan) pkg(x ast.Expr, sel string) (string, bool) {
	if !sc.imported {
		return "", false
	}
	switch x := x.(type) {
	case *ast.SelectorExpr:
		if id, ok := x.X.(*ast.Ident); ok && sc.name != "." && id.Name == sc.name && x.Sel.Name == sel {
			return sc.name + "." + sel, true
		}
	case *ast.Ident:
		if sc.name == "." && x.Name == sel {
			return sel, true
		}
	}
	return "", false
}

// collectContexts records, for each call, the argument of the
// .Context(...) it is followed by, and the name of each function
// literal assigned to a variable.
func (sc *fileScan) collectContexts() {
	sc.fi = newFileInfo(sc.p.file)
	named := func(lhs, rhs []ast.Expr) {
		if len(lhs) != len(rhs) {
			return
		}
		for i, r := range rhs {
			lit, ok := ast.Unparen(r).(*ast.FuncLit)
			id, isID := lhs[i].(*ast.Ident)
			if ok && isID {
				sc.litNames[lit] = id.Name
			}
		}
	}
	ast.Inspect(sc.p.file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if sel, ok := n.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Context" && len(n.Args) == 1 {
				if inner, ok := ast.Unparen(sel.X).(*ast.CallExpr); ok {
					sc.contexts[inner] = n.Args[0]
				}
			}
		case *ast.AssignStmt:
			named(n.Lhs, n.Rhs)
		case *ast.ValueSpec:
			lhs := make([]ast.Expr, len(n.Names))
			for i, id := range n.Names {
				lhs[i] = id
			}
			named(lhs, n.Values)
		}
		return true
	})
}

// visit is the PreorderStack callback: a catalog literal is read, a call
// is a message.
func (sc *fileScan) visit(n ast.Node, stack []ast.Node) bool {
	switch n := n.(type) {
	case *ast.CompositeLit:
		if _, ok := sc.pkg(n.Type, "Catalog"); ok {
			sc.s.Catalogs = append(sc.s.Catalogs, goCatalog(sc.p.fset, sc.p.path, rel(sc.s.Root, sc.p.path), n))
		}
	case *ast.CallExpr:
		sc.visitCall(n, stack)
	}
	return true
}

// scopeAt returns the scope of the constants a call within stack sees:
// its package's, and those of the outermost function around it, but for
// the names that function declares otherwise.
func (sc *fileScan) scopeAt(stack []ast.Node) *scope {
	for _, n := range stack {
		switch n.(type) {
		case *ast.FuncDecl, *ast.FuncLit:
			sc2 := sc.scopes[n]
			if sc2 == nil {
				sc2 = localScope(sc.p.consts, n)
				sc.scopes[n] = sc2
			}
			return sc2
		}
	}
	return &scope{pkg: sc.p.consts}
}

// passedOn reports whether each of args that is not a constant string is
// a parameter of a function around the call that is one of funcs: the
// body of a wrapper, passing its message on.
func (sc *fileScan) passedOn(stack []ast.Node, s *scope, args ...ast.Expr) bool {
	for _, a := range args {
		if _, ok := s.str(a); ok {
			continue
		}
		id, ok := ast.Unparen(a).(*ast.Ident)
		if !ok || !paramOfWrapper(sc.fi, sc.funcs, sc.litNames, stack, id.Name) {
			return false
		}
	}
	return true
}

// callName returns the call as a message: which function was called, the
// argument indexes of its message and plural form, and whether it is a
// method, which is a localizer's only when its message is a literal.
func (sc *fileScan) callName(n *ast.CallExpr) (call string, key, other int, plural, method bool) {
	if c, ok := sc.pkg(n.Fun, "T"); ok {
		return c, 0, -1, false, false
	}
	if c, ok := sc.pkg(n.Fun, "N"); ok {
		return c, 1, 2, true, false
	}
	if w, ok := wrapperOf(sc.fi, sc.funcs, n.Fun); ok {
		return w.Call(), w.Key, w.Other, w.Plural, false
	}
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if !ok || !sc.imported {
		return "", -1, -1, false, false
	}
	if id, isPkg := sel.X.(*ast.Ident); isPkg && id.Name == sc.name {
		return "", -1, -1, false, false
	}
	switch sel.Sel.Name {
	case "S":
		return "." + sel.Sel.Name, 0, -1, false, true
	case "N":
		return "." + sel.Sel.Name, 1, 2, true, true
	}
	return "", -1, -1, false, false
}

// visitCall reads a call as a message when it is one.
func (sc *fileScan) visitCall(n *ast.CallExpr, stack []ast.Node) {
	call, key, other, plural, method := sc.callName(n)
	if key < 0 || len(n.Args) <= max(key, other) {
		return
	}
	pos := sc.p.fset.Position(n.Pos())
	s := sc.scopeAt(stack)
	args := []ast.Expr{n.Args[key]}
	if plural {
		args = append(args, n.Args[other])
	}
	text, ok := s.str(args[0])
	otherText := ""
	if ok && plural {
		otherText, ok = s.str(args[1])
	}
	if !ok {
		// A method is a localizer's only by its literal.
		if !method && !sc.passedOn(stack, s, args...) {
			sc.s.Skipped = append(sc.s.Skipped, Skipped{Pos: pos, Call: call})
		}
		return
	}
	ctx, ok := sc.callContext(n, s, pos, call)
	if !ok {
		return
	}
	m := sc.s.Messages
	if plural {
		m = sc.s.Plurals
	}
	k := Key(text, ctx)
	msg := m[k]
	if msg == nil {
		msg = &Message{Key: k, Source: text, Other: otherText, Plural: plural}
		m[k] = msg
	}
	msg.Uses = append(msg.Uses, pos)
}

// callContext returns the context a call's .Context(...) gives its
// message, and whether it could be read.
func (sc *fileScan) callContext(n *ast.CallExpr, s *scope, pos token.Position, call string) (string, bool) {
	arg, has := sc.contexts[n]
	if !has {
		return "", true
	}
	c, ok := s.str(arg)
	if !ok {
		sc.s.Skipped = append(sc.s.Skipped, Skipped{Pos: pos, Call: call + "(…).Context"})
		return "", false
	}
	return c, true
}

// wrapperOf returns the one of funcs that fun calls.
func wrapperOf(fi *fileInfo, funcs []Func, fun ast.Expr) (Func, bool) {
	for _, w := range funcs {
		if fi.matches(w, fun) {
			return w, true
		}
	}
	return Func{}, false
}

// paramOfWrapper reports whether name, used within stack, is a parameter
// of one of funcs: the innermost function around it that declares a
// parameter so called is one of them.
func paramOfWrapper(fi *fileInfo, funcs []Func, litNames map[*ast.FuncLit]string, stack []ast.Node, name string) bool {
	for _, s := range slices.Backward(stack) {
		var typ *ast.FuncType
		fn := ""
		switch n := s.(type) {
		case *ast.FuncDecl:
			typ, fn = n.Type, n.Name.Name
		case *ast.FuncLit:
			typ, fn = n.Type, litNames[n]
		default:
			continue
		}
		if !declares(typ.Params, name) {
			continue
		}
		for _, w := range funcs {
			if fn != "" && fi.declares(w, fn) {
				return true
			}
		}
		return false
	}
	return false
}

// declares reports whether a field list declares name.
func declares(fl *ast.FieldList, name string) bool {
	if fl == nil {
		return false
	}
	for _, field := range fl.List {
		for _, n := range field.Names {
			if n.Name == name {
				return true
			}
		}
	}
	return false
}

// scope is the string constants a call sees.
type scope struct {
	pkg   map[string]ast.Expr
	local map[string]ast.Expr
	// shadow holds the names a function declares as other than
	// constants, which hide its package's.
	shadow map[string]bool
	busy   map[string]bool
}

// localScope returns the scope inside fn, a function declaration or
// literal. Every name declared anywhere in it counts, whatever its block,
// so that a name is never taken for a constant it might not be.
func localScope(pkg map[string]ast.Expr, fn ast.Node) *scope {
	sc := &scope{pkg: pkg, local: map[string]ast.Expr{}, shadow: map[string]bool{}}
	ast.Inspect(fn, func(n ast.Node) bool {
		return sc.localDecl(n)
	})
	return sc
}

// localDecl records what n declares inside its function: parameters and
// variables shadow package constants, and a const declaration adds to
// the local constants.
func (sc *scope) localDecl(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.FuncDecl:
		sc.fields(n.Recv)
	case *ast.FuncType:
		sc.fields(n.TypeParams)
		sc.fields(n.Params)
		sc.fields(n.Results)
	case *ast.AssignStmt:
		if n.Tok == token.DEFINE {
			sc.idents(n.Lhs...)
		}
	case *ast.RangeStmt:
		if n.Tok == token.DEFINE {
			sc.idents(n.Key, n.Value)
		}
	case *ast.GenDecl:
		if n.Tok == token.CONST {
			addConsts(sc.local, n)
			return false
		}
		sc.shadowSpec(n)
	}
	return true
}

func (sc *scope) idents(es ...ast.Expr) {
	for _, e := range es {
		if id, ok := e.(*ast.Ident); ok {
			sc.shadow[id.Name] = true
		}
	}
}

func (sc *scope) fields(fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, f := range fl.List {
		for _, n := range f.Names {
			sc.shadow[n.Name] = true
		}
	}
}

func (sc *scope) shadowSpec(n *ast.GenDecl) {
	for _, spec := range n.Specs {
		switch spec := spec.(type) {
		case *ast.ValueSpec:
			for _, id := range spec.Names {
				sc.shadow[id.Name] = true
			}
		case *ast.TypeSpec:
			sc.shadow[spec.Name.Name] = true
		}
	}
}

// str returns the string a constant expression of string literals and
// constants spells, as in "a", "a" + "b" or x after const x = "a", and
// whether e is one.
func (sc *scope) str(e ast.Expr) (string, bool) {
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
		a, ok := sc.str(e.X)
		if !ok {
			return "", false
		}
		b, ok := sc.str(e.Y)
		return a + b, ok
	case *ast.Ident:
		return sc.ident(e)
	}
	return "", false
}

// ident resolves a name to the constant it spells, guarding against a
// constant that refers to itself.
func (sc *scope) ident(e *ast.Ident) (string, bool) {
	if sc == nil || sc.busy[e.Name] {
		return "", false
	}
	if sc.busy == nil {
		sc.busy = map[string]bool{}
	}
	if sc.shadow[e.Name] {
		return "", false
	}
	in := sc
	x, ok := sc.local[e.Name]
	if !ok {
		// A package's constant sees only the package's.
		in = &scope{pkg: sc.pkg, busy: sc.busy}
		x, ok = sc.pkg[e.Name]
	}
	if !ok || x == nil {
		return "", false
	}
	sc.busy[e.Name] = true
	defer delete(sc.busy, e.Name)
	return in.str(x)
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
	return (*scope)(nil).str(e)
}

// goCatalog reads a composite literal of i18n.Catalog. What is not a
// literal is noted rather than read: a translation computed by the
// program counts as present, a language that is not is an error.
func goCatalog(fset *token.FileSet, path, name string, lit *ast.CompositeLit) *Catalog {
	c := &Catalog{
		Path: path, Name: name, Line: fset.Position(lit.Pos()).Line, Go: true,
		Messages: map[string]string{}, Plurals: map[string]Forms{},
		dynamic: map[string]bool{}, dynamicPlural: map[string]bool{},
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
			c.goMessages(kv.Value)
		case "Plurals":
			c.goPlurals(kv.Value)
		}
	}
	if c.Lang == "" && c.Err == nil {
		c.Err = errors.New("it has no Lang")
	}
	return c
}

// goMessages reads the Messages table of a Go catalog literal.
func (c *Catalog) goMessages(e ast.Expr) {
	m, ok := ast.Unparen(e).(*ast.CompositeLit)
	if !ok {
		c.Partial = true
		return
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
}

// goPlurals reads the Plurals table of a Go catalog literal.
func (c *Catalog) goPlurals(e ast.Expr) {
	m, ok := ast.Unparen(e).(*ast.CompositeLit)
	if !ok {
		c.Partial = true
		return
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
			c.dynamicPlural[k] = true
		}
		c.Plurals[k] = forms
	}
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
