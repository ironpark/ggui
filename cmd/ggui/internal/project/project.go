// Package project resolves what a ggui command works on: the ggui.toml in
// effect, the directory its paths are relative to, and the main package
// with the names derived from it.
package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// Project is a resolved configuration.
type Project struct {
	// Root is the directory paths in the configuration are relative to and
	// the go command runs in: the file's directory, or the working
	// directory when there is no file.
	Root string
	// File is the ggui.toml read, or "" when the defaults are in effect.
	File   string
	Config config.Config
}

// Open finds and reads the configuration for a command run in dir on the
// package arg, "" for the configured one. An explicit path, from --config,
// must exist. Otherwise the file is looked for from the package's
// directory when arg names one, so that each app in a repository can have
// its own, and from dir when it does not; with no file found, the defaults
// apply, relative to dir.
func Open(dir, arg, explicit string) (*Project, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	file := explicit
	if file == "" {
		from := dir
		if isDirArg(arg) {
			if fi, err := os.Stat(arg); err == nil && fi.IsDir() {
				from = arg
			}
		}
		if file, err = config.Find(from); err != nil {
			return nil, err
		}
	} else if file, err = filepath.Abs(file); err != nil {
		return nil, err
	}
	p := &Project{Root: dir, Config: config.Defaults()}
	if file == "" {
		return p, nil
	}
	cfg, err := config.Load(file)
	if err != nil {
		return nil, err
	}
	p.Root, p.File, p.Config = filepath.Dir(file), file, cfg
	return p, nil
}

// Path resolves a path from the configuration against Root.
func (p *Project) Path(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(p.Root, rel)
}

// Package returns the main package to build, as the go command running in
// Root should be given it. arg is the package named on the command line,
// relative to the working directory, or "" for the configured one.
func (p *Project) Package(arg string) (string, error) {
	if arg == "" {
		return pathArg(p.Path(p.Config.App.Package)), nil
	}
	if isDirArg(arg) {
		abs, err := filepath.Abs(arg)
		if err != nil {
			return "", err
		}
		return pathArg(abs), nil
	}
	return arg, nil // an import path
}

// isDirArg reports whether a package argument is a directory rather than
// an import path, by the go command's own rule.
func isDirArg(arg string) bool {
	return arg == "." || arg == ".." || filepath.IsAbs(arg) ||
		strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, `.\`) || strings.HasPrefix(arg, `..\`)
}

// pathArg writes an absolute directory the way the go command takes it.
func pathArg(abs string) string { return filepath.Clean(abs) }

// Main is the main package resolved, with the names derived from it.
type Main struct {
	Package    string // as given to the go command
	Dir        string // its directory
	ImportPath string
	// Executable is the program's file name without extension: the
	// directory's name, as go build would call it.
	Executable string
	// Name is the name people see: app.name, or Executable.
	Name string
	// ID is the bundle identifier: app.id, or one made from the import
	// path.
	ID string
	// SysoFiles are the object files in Dir the go command links into
	// the program, for the platform and tags it was listed with.
	SysoFiles []string
}

// ResolveMain lists the main package, which also checks it is one.
func (p *Project) ResolveMain(ctx context.Context, arg string, lo gotool.ListOptions) (*Main, error) {
	pkg, err := p.Package(arg)
	if err != nil {
		return nil, err
	}
	lo.Dir = p.Root
	mp, err := gotool.Main(ctx, lo, pkg)
	if err != nil {
		return nil, err
	}
	m := &Main{Package: pkg, Dir: mp.Dir, ImportPath: mp.ImportPath, SysoFiles: mp.SysoFiles}
	m.Executable = filepath.Base(mp.Dir)
	if m.Executable == "." || m.Executable == string(filepath.Separator) {
		m.Executable = "app"
	}
	m.Name = p.Config.App.Name
	if m.Name == "" {
		m.Name = m.Executable
	}
	m.ID = p.Config.App.ID
	if m.ID == "" {
		m.ID = DefaultID(mp.ImportPath)
	}
	return m, nil
}

// DefaultID makes a bundle identifier from an import path, reversing its
// host as Apple's identifiers are: github.com/alice/hello becomes
// com.github.alice.hello. A path with no host, such as hello, becomes
// com.example.hello, which works on this Mac and says it is a placeholder.
func DefaultID(importPath string) string {
	parts := strings.Split(importPath, "/")
	var out []string
	if host := parts[0]; strings.Contains(host, ".") {
		hp := strings.Split(host, ".")
		for _, h := range slices.Backward(hp) {
			out = append(out, h)
		}
		parts = parts[1:]
	} else {
		out = append(out, "com", "example")
	}
	out = append(out, parts...)
	for i, s := range out {
		out[i] = idPart(s)
	}
	var kept []string
	for _, s := range out {
		if s != "" {
			kept = append(kept, s)
		}
	}
	if len(kept) < 2 {
		kept = append([]string{"com", "example"}, kept...)
	}
	return strings.Join(kept, ".")
}

// idPart keeps what a bundle identifier allows: letters, digits and
// hyphens.
func idPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '-':
			b.WriteRune(r)
		case r == '_' || r == '.' || r == ' ':
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// OutDir returns the output directory: out from the command line,
// relative to the working directory, or build.output.
func (p *Project) OutDir(out string) (string, error) {
	if out != "" {
		return filepath.Abs(out)
	}
	return p.Path(p.Config.Build.Output), nil
}

// CheckIcon reports a configured icon that is missing, before a build
// spends time to fail on it at the end.
func (p *Project) CheckIcon() error {
	if p.Config.App.Icon == "" {
		return nil
	}
	path := p.Path(p.Config.App.Icon)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("app.icon: %w; add the PNG or remove the key from %s", err, p.fileName())
	}
	return nil
}

func (p *Project) fileName() string {
	if p.File == "" {
		return config.FileName
	}
	return p.File
}
