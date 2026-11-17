// Package gosrc answers two questions about a Go file that ggui dev asks
// before it builds: did an edit change anything the compiler acts on, and
// does the file parse at all.
package gosrc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"go/version"
	"os"
	"runtime"
	"strings"
	"time"
)

// Sum is a Fingerprint.
type Sum [sha256.Size]byte

// Fingerprint returns a digest of what in src a build depends on: its
// tokens, without their positions, the layout between them or comments,
// except the comments that change a build:
//
//   - //go:build and // +build lines, which decide whether the file is
//     built;
//   - //go: directives, such as //go:embed, //go:linkname and
//     //go:noinline, but not //go:generate, which only go generate reads;
//   - //line and /*line directives, which set the positions the compiler
//     records;
//   - //export, which cgo turns into a C function;
//   - the comment before import "C", which is C code.
//
// Two versions of a file with the same fingerprint build the same program,
// but for the line numbers it records: an edit to a comment or to the
// layout changes those of what follows.
func Fingerprint(src []byte) Sum {
	keep := cgoPreamble(src)
	h := sha256.New()
	var buf [binary.MaxVarintLen64 + 1]byte
	put := func(tok token.Token, lit string) {
		buf[0] = byte(tok)
		n := 1 + binary.PutUvarint(buf[1:], uint64(len(lit)))
		h.Write(buf[:n])
		h.Write([]byte(lit))
	}
	fset := token.NewFileSet()
	file := fset.AddFile("", -1, len(src))
	var s scanner.Scanner
	// Errors are left to the build; the tokens around one still count.
	s.Init(file, src, func(token.Position, string) {}, scanner.ScanComments)
	for {
		pos, tok, lit := s.Scan()
		switch tok {
		case token.EOF:
			var sum Sum
			h.Sum(sum[:0])
			return sum
		case token.COMMENT:
			off := file.Offset(pos)
			if !buildComment(lit) && !keep.has(off) {
				continue
			}
		case token.SEMICOLON:
			lit = "" // ";" written or "\n" implied: the same token
		}
		put(tok, lit)
	}
}

// buildComment reports whether a comment, as the scanner returns it, can
// change a build wherever it is.
func buildComment(c string) bool {
	switch {
	case strings.HasPrefix(c, "//go:"):
		return !strings.HasPrefix(c, "//go:generate")
	case strings.HasPrefix(c, "// +build"), strings.HasPrefix(c, "//line "), strings.HasPrefix(c, "/*line "),
		strings.HasPrefix(c, "//export "):
		return true
	}
	return false
}

// spans are byte ranges of a file.
type spans [][2]int

func (s spans) has(off int) bool {
	for _, r := range s {
		if off >= r[0] && off < r[1] {
			return true
		}
	}
	return false
}

// cgoPreamble returns where in src the comments are that cgo reads as C:
// the doc comment of import "C", found the way cgo finds it. A file that
// imports "C" but does not parse keeps every comment before its first
// declaration other than an import, to be safe.
func cgoPreamble(src []byte) spans {
	if !bytes.Contains(src, []byte(`"C"`)) {
		return nil
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ImportsOnly|parser.ParseComments|parser.SkipObjectResolution)
	if f == nil {
		return spans{{0, len(src)}}
	}
	base := fset.File(f.Pos()).Base()
	var out spans
	add := func(cg *ast.CommentGroup) {
		if cg != nil {
			out = append(out, [2]int{int(cg.Pos()) - base, int(cg.End()) - base})
		}
	}
	for _, d := range f.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.IMPORT {
			continue
		}
		for _, sp := range g.Specs {
			is := sp.(*ast.ImportSpec)
			if is.Path.Value != `"C"` {
				continue
			}
			if is.Doc != nil {
				add(is.Doc)
			} else if len(g.Specs) == 1 {
				add(g.Doc)
			}
		}
	}
	if err != nil {
		// Parsed in part: keep all up to where the imports stop.
		end := len(src)
		if len(f.Decls) > 0 {
			end = int(f.Decls[len(f.Decls)-1].End()) - base
		}
		out = append(out, [2]int{0, end})
	}
	return out
}

// CanParse reports whether this program's parser knows the syntax of the
// Go release named by goVersion, as go env GOVERSION gives it: whether it
// is from that release or a later one. An older parser would reject what
// the newer release added.
func CanParse(goVersion string) bool {
	goVersion, _, _ = strings.Cut(goVersion, " ")
	own, want := version.Lang(runtime.Version()), version.Lang(goVersion)
	return own != "" && want != "" && version.Compare(own, want) >= 0
}

// CheckFiles parses each of files and returns the syntax errors of those
// that do not parse, each file named as name returns. A file that does not
// parse is read again after wait and parsed once more first, since it may
// have been caught half-written by an editor or a tool. A file that cannot
// be read is passed over: the build says what became of it.
func CheckFiles(ctx context.Context, files []string, name func(string) string, wait time.Duration) error {
	check := func(fs []string) (bad []string, errs []error) {
		for _, f := range fs {
			src, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			if err := Check(name(f), src); err != nil {
				bad, errs = append(bad, f), append(errs, err)
			}
		}
		return bad, errs
	}
	bad, _ := check(files)
	if len(bad) == 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
	}
	_, errs := check(bad)
	return errors.Join(errs...)
}

// Check parses src, a Go file, and returns its syntax errors in the
// file:line:col form compilers and editors use, with name as the file.
func Check(name string, src []byte) error {
	_, err := parser.ParseFile(token.NewFileSet(), name, src, parser.SkipObjectResolution|parser.AllErrors)
	el, ok := err.(scanner.ErrorList)
	if !ok {
		return err
	}
	el.RemoveMultiples()
	lines := make([]string, 0, len(el))
	for i, e := range el {
		if i == 10 {
			lines = append(lines, "too many errors")
			break
		}
		lines = append(lines, e.Error())
	}
	return errors.New(strings.Join(lines, "\n"))
}
