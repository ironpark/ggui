// Package scaffold writes a new ggui application: a module, a small
// counter to start from, its ggui.toml and a .gitignore.
package scaffold

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// GGUI is the library's module path.
const GGUI = "github.com/ironpark/ggui"

// goDirective is the go line written to go.mod: the version ggui itself
// asks for, which go get then raises if a newer ggui asks for more.
const goDirective = "1.27.0"

//go:embed main.go.tmpl
var mainTemplate string

var mainTmpl = template.Must(template.New("main.go").Parse(mainTemplate))

// Options says what to write.
type Options struct {
	Dir    string // the directory to write into, created if missing
	Module string // the module path; empty derives one from Dir
	Name   string // the app's name; empty uses Dir's name
	// Replace points ggui at a directory, a checkout, rather than a
	// published version.
	Replace string
	// Offline writes the files and stops: no go get, no go mod tidy.
	Offline bool
	// Force overwrites files that exist.
	Force bool
	Log   io.Writer // the go command's output, for Init
}

// Result is what was written.
type Result struct {
	Dir     string
	Module  string
	Name    string
	Files   []string
	Fetched bool // whether ggui was added to go.mod
	// Replaced is whether go.mod replaces ggui with a checkout.
	Replaced bool
}

// Files are the files Init writes, in the order it writes them.
var Files = []string{"go.mod", "main.go", config.FileName, ".gitignore"}

// Init writes the application, then, unless Offline, runs the go commands
// that add ggui to it, their output going to Log. It checks every file
// first, so that it either writes all of them or, refusing to overwrite,
// none.
func Init(ctx context.Context, o Options) (*Result, error) {
	r, err := Write(o)
	if err != nil || o.Offline {
		return r, err
	}
	for _, args := range r.Commands() {
		if err := Go(ctx, r.Dir, o.Log, args...); err != nil {
			if errors.Is(err, gotool.ErrNoGo) {
				return r, fmt.Errorf("%w, then run %s in %s", err, r.Finish(), r.Dir)
			}
			return r, fmt.Errorf("go %s: %w; the files are written, so fix the cause and run it again in %s, or use --offline",
				strings.Join(args, " "), err, r.Dir)
		}
	}
	r.Fetched = true
	return r, nil
}

// Commands returns the go commands, without the program name, that add
// ggui to the module Write wrote, in order.
func (r *Result) Commands() [][]string {
	if r.Replaced {
		return [][]string{{"mod", "tidy"}} // tidy adds the requirement the replace satisfies
	}
	return [][]string{{"get", GGUI + "@latest"}, {"mod", "tidy"}}
}

// Finish returns the command line that does what Commands do, for a
// person to run.
func (r *Result) Finish() string {
	var lines []string
	for _, args := range r.Commands() {
		lines = append(lines, "go "+strings.Join(args, " "))
	}
	return strings.Join(lines, " && ")
}

// Go runs one of Commands in dir, its output going to out. GOWORK is off:
// a go.work above the new directory belongs to another project, and go
// get refuses to edit a module through one. When ctx is done the go
// command is interrupted and given time to stop.
func Go(ctx context.Context, dir string, out io.Writer, args ...string) error {
	return gotool.Run(ctx, dir, []string{"GOWORK=off"}, out, args...)
}

// Write writes the application's files, checking every one first, so
// that it either writes all of them or, refusing to overwrite, none.
func Write(o Options) (*Result, error) {
	dir, err := filepath.Abs(o.Dir)
	if err != nil {
		return nil, err
	}
	base := filepath.Base(dir)
	r := &Result{Dir: dir, Module: o.Module, Name: o.Name, Replaced: o.Replace != ""}
	if r.Module == "" {
		r.Module = modulePath(base)
		if r.Module == "" {
			return nil, fmt.Errorf("cannot make a module path from %q: give one with --module", base)
		}
	}
	if err := checkModulePath(r.Module); err != nil {
		return nil, err
	}
	if r.Name == "" {
		r.Name = base
	}

	if !o.Force {
		var exist []string
		for _, f := range Files {
			if _, err := os.Lstat(filepath.Join(dir, f)); err == nil {
				exist = append(exist, f)
			}
		}
		if len(exist) > 0 {
			return nil, fmt.Errorf("%s already has %s; pass --force to overwrite, or choose an empty directory",
				dir, strings.Join(exist, ", "))
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	var main bytes.Buffer
	if err := mainTmpl.Execute(&main, map[string]string{"Name": r.Name, "Executable": base}); err != nil {
		return nil, err
	}
	gomod := fmt.Sprintf("module %s\n\ngo %s\n", r.Module, goDirective)
	if o.Replace != "" {
		rel, err := replacePath(dir, o.Replace)
		if err != nil {
			return nil, err
		}
		gomod += fmt.Sprintf("\nreplace %s => %s\n", GGUI, rel)
	}
	files := map[string][]byte{
		"go.mod":        []byte(gomod),
		"main.go":       main.Bytes(),
		config.FileName: config.Template(r.Name, project.DefaultID(r.Module), "0.1.0"),
		// go build names the program after the directory.
		".gitignore": []byte(gitignore(base)),
	}
	for _, f := range Files {
		if err := os.WriteFile(filepath.Join(dir, f), files[f], 0o644); err != nil {
			return nil, err
		}
		r.Files = append(r.Files, f)
	}
	return r, nil
}

// replacePath writes the replacement relative to the new module when the
// two are neighbors, a checkout beside the app, so the pair can be moved
// together; a path that climbs further is clearer written whole.
func replacePath(dir, target string) (string, error) {
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "go.mod")); err != nil {
		return "", fmt.Errorf("--replace %s: no go.mod there; name a checkout of %s", target, GGUI)
	}
	if rel, err := filepath.Rel(dir, abs); err == nil {
		rel = filepath.ToSlash(rel)
		if strings.Count(rel+"/", "../") <= 2 {
			if !strings.HasPrefix(rel, "../") {
				rel = "./" + rel
			}
			return rel, nil
		}
	}
	return filepath.ToSlash(abs), nil
}

// modulePath makes a module path from a directory name: lower case, with
// anything a path element should not hold turned into hyphens.
func modulePath(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)), r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-.")
}

// checkModulePath rejects what go mod would, earlier and with the reason.
func checkModulePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") || strings.Contains(p, "//") {
		return fmt.Errorf("module path %q is not valid; use something like example.com/hello or hello", p)
	}
	for _, r := range p {
		if r > unicode.MaxASCII || unicode.IsSpace(r) || strings.ContainsRune(`"'*<>?`+"`|:\\", r) {
			return fmt.Errorf("module path %q has %q, which a module path cannot hold", p, r)
		}
	}
	return nil
}

func gitignore(exe string) string {
	return fmt.Sprintf(`# ggui build's output (build.output in ggui.toml).
/build/

# What go build leaves in the directory.
/%[1]s
/%[1]s.exe
`, exe)
}
