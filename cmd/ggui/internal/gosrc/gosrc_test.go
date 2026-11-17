package gosrc

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const base = `//go:build linux || darwin

// Package p is a package.
package p

import (
	"embed"
	"fmt"
	_ "unsafe"
)

//go:embed static
var static embed.FS

//go:generate stringer -type=Kind

// F prints.
//
//go:noinline
func F(a, b int) { // the sum
	fmt.Println(a + b, "two  spaces", ` + "`raw\n  string`" + `)
}

//go:linkname nanotime runtime.nanotime
func nanotime() int64
`

const cgoBase = `package p

// #include <stdio.h>
// static int one(void) { return 1; }
import "C"

// G calls C.
//
//export G
func G() int { return int(C.one()) }
`

func TestFingerprint(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		old, new string // the edit: the first old in src becomes new
		same     bool   // the fingerprint does not change
	}{
		{"comment text", base, "// F prints.", "// F prints its arguments' sum.", true},
		{"line comment after code", base, "// the sum", "// a sum, really", true},
		{"comment added", base, "func nanotime", "// nanotime is the runtime's.\nfunc nanotime", true},
		{"comment removed", base, "// Package p is a package.\n", "", true},
		{"block comment", base, "func F(a, b int) {", "func F(a, /* first */ b int) {", true},
		{"indentation", base, "\tfmt.Println", "        fmt.Println", true},
		{"blank lines", base, "var static embed.FS\n", "var static embed.FS\n\n\n\n", true},
		{"spaces between tokens", base, "a + b", "a+b", true},
		{"explicit semicolon", base, "fmt.Println(a + b, \"two  spaces\", `raw\n  string`)\n", "fmt.Println(a + b, \"two  spaces\", `raw\n  string`);\n", true},
		{"go:generate", base, "stringer -type=Kind", "stringer -type=Kind,Other", true},

		{"go:build line", base, "linux || darwin", "linux", false},
		{"+build line", "// +build linux\n\npackage p\n", "linux", "darwin", false},
		{"go:embed pattern", base, "//go:embed static", "//go:embed static/*.png", false},
		{"go:embed added", base, "var static embed.FS", "//go:embed more\nvar static embed.FS", false},
		{"go:noinline removed", base, "//go:noinline\n", "", false},
		{"go:linkname target", base, "runtime.nanotime", "runtime.cputicks", false},
		{"line directive", base, "func nanotime", "//line other.go:10\nfunc nanotime", false},
		{"block line directive", base, "func nanotime", "/*line other.go:10:1*/func nanotime", false},
		{"comment with a newline ends a statement", "package p\nvar x = a /* */ + b\n", "/* */", "/*\n*/", false},

		{"token", base, "a + b", "a - b", false},
		{"identifier", base, "func F(", "func H(", false},
		{"string literal spacing", base, `"two  spaces"`, `"two spaces"`, false},
		{"raw string layout", base, "`raw\n  string`", "`raw\n string`", false},
		{"number", "package p\nconst n = 1\n", "1", "0x1", false},

		{"cgo preamble", cgoBase, "return 1;", "return 2;", false},
		{"cgo preamble line added", cgoBase, "// #include <stdio.h>\n", "// #include <stdio.h>\n// #include <stdlib.h>\n", false},
		{"cgo file, other comment", cgoBase, "// G calls C.", "// G calls C, once.", true},
		{"export name", cgoBase, "//export G", "//export H", false},
		{"cgo preamble in parentheses", "package p\n\nimport (\n\t\"fmt\"\n\t// int x;\n\t\"C\"\n)\n", "int x;", "int y;", false},
		{"cgo preamble of the declaration", "package p\n\n// int x;\nimport (\n\t\"C\"\n)\n", "int x;", "int y;", false},
		{"comment before another import of a cgo file", "package p\n\n// About fmt.\nimport \"fmt\"\n\n// int x;\nimport \"C\"\n", "About fmt.", "About the fmt package.", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.src, tt.old) {
				t.Fatalf("%q is not in the source", tt.old)
			}
			edited := strings.Replace(tt.src, tt.old, tt.new, 1)
			same := Fingerprint([]byte(tt.src)) == Fingerprint([]byte(edited))
			if same != tt.same {
				t.Errorf("same fingerprint = %v, want %v after\n%s", same, tt.same, edited)
			}
		})
	}
}

// A file that does not parse still has a fingerprint, which changes with
// its tokens and not with its comments.
func TestFingerprintBroken(t *testing.T) {
	src := "package p\nfunc F( {\n// note\n"
	if Fingerprint([]byte(src)) != Fingerprint([]byte(strings.Replace(src, "note", "other", 1))) {
		t.Error("a comment changed the fingerprint of a broken file")
	}
	if Fingerprint([]byte(src)) == Fingerprint([]byte(strings.Replace(src, "F(", "G(", 1))) {
		t.Error("a token did not change the fingerprint of a broken file")
	}
	broken := "package p\n// int x;\nimport \"C\"\nfunc ("
	if Fingerprint([]byte(broken)) == Fingerprint([]byte(strings.Replace(broken, "int x", "int y", 1))) {
		t.Error("the preamble of a cgo file that does not parse was not kept")
	}
}

func TestCanParse(t *testing.T) {
	own := runtime.Version()
	tests := []struct {
		v    string
		want bool
	}{
		{"go1.18", true},
		{"go1.21.0", true},
		{"go9.99.0", false},
		{"go9.99", false},
		{"devel go1.99-abcdef", false},
		{"", false},
	}
	if strings.HasPrefix(own, "go") {
		tests = append(tests, struct {
			v    string
			want bool
		}{own + " X:nocoverageredesign", true})
	}
	for _, tt := range tests {
		if got := CanParse(tt.v); got != tt.want {
			t.Errorf("CanParse(%q) = %v, want %v (built with %s)", tt.v, got, tt.want, own)
		}
	}
}

func TestCheck(t *testing.T) {
	if err := Check("ok.go", []byte(base)); err != nil {
		t.Errorf("a valid file: %v", err)
	}
	err := Check("./bad.go", []byte("package p\n\nfunc F() {\n\tx := \n}\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "./bad.go:5:1: ") {
		t.Errorf("err = %v, want one at ./bad.go:5:1", err)
	}
	// Type errors are the compiler's to find.
	if err := Check("types.go", []byte("package p\nvar x int = \"s\"\nfunc F() { undefined() }\n")); err != nil {
		t.Errorf("a type error was reported: %v", err)
	}
}

func TestCheckFiles(t *testing.T) {
	dir := t.TempDir()
	good, bad, half := filepath.Join(dir, "good.go"), filepath.Join(dir, "bad.go"), filepath.Join(dir, "half.go")
	full := "package p\n\nfunc F() int {\n\treturn 1\n}\n"
	os.WriteFile(good, []byte(full), 0o644)
	os.WriteFile(bad, []byte("package p\nfunc (\n"), 0o644)
	name := filepath.Base
	ctx := context.Background()

	if err := CheckFiles(ctx, []string{good, filepath.Join(dir, "gone.go")}, name, time.Second); err != nil {
		t.Errorf("good and missing files: %v", err)
	}
	start := time.Now()
	err := CheckFiles(ctx, []string{good, bad}, name, 50*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "bad.go:") {
		t.Errorf("err = %v, want bad.go's", err)
	}
	if d := time.Since(start); d < 50*time.Millisecond {
		t.Errorf("reported after %s, before checking again", d)
	}

	// Half written when first read, whole by the second.
	os.WriteFile(half, []byte(full[:20]), 0o644)
	go func() {
		time.Sleep(20 * time.Millisecond)
		os.WriteFile(half, []byte(full), 0o644)
	}()
	if err := CheckFiles(ctx, []string{half}, name, 200*time.Millisecond); err != nil {
		t.Errorf("a file completed before the second read: %v", err)
	}
}
