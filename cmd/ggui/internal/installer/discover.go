package installer

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// DefaultCandidates are the conventional subdirectories checked automatically
// for a Go main package defining the project's installer.
var DefaultCandidates = []string{
	filepath.Join("cmd", "installer"),
	"installer",
}

// Package describes a resolved Go main package that defines an installer using
// github.com/ironpark/ggui/installer.
type Package struct {
	// Arg is the package argument passed to the go command.
	Arg string
	// Dir is the absolute directory of the main package on disk.
	Dir string
	// ImportPath is the Go import path reported by `go list`.
	ImportPath string
	// Explicit reports whether the package was named via --installer-pkg or
	// build.installer rather than auto-discovered.
	Explicit bool
}

// Discover resolves the project's Go installer package, if any.
//
// Resolution order:
//  1. flagPkg (--installer-pkg, resolved relative to cwd if a directory)
//  2. configured (build.installer in ggui.toml, resolved relative to projectRoot)
//  3. conventional ./cmd/installer or ./installer directory containing package main
//     (skipping mainDir so an app whose own package lives in ./installer is never
//     mistaken for a separate installer package).
//
// Returns (nil, nil) when no explicit package is configured and no conventional
// installer main package exists.
func Discover(ctx context.Context, projectRoot, mainDir, configured, flagPkg string) (*Package, error) {
	if flagPkg != "" {
		arg, err := resolveArg(flagPkg, "")
		if err != nil {
			return nil, err
		}
		return loadMainPackage(ctx, projectRoot, arg, true)
	}
	if configured != "" {
		arg, err := resolveArg(configured, projectRoot)
		if err != nil {
			return nil, err
		}
		return loadMainPackage(ctx, projectRoot, arg, true)
	}
	for _, rel := range DefaultCandidates {
		cand := filepath.Join(projectRoot, rel)
		if mainDir != "" && sameDir(cand, mainDir) {
			continue
		}
		if !IsMainPackageDir(cand) {
			continue
		}
		return loadMainPackage(ctx, projectRoot, cand, false)
	}
	return nil, nil
}

// CandidateDir returns the configured or auto-discovered installer package
// directory (or import path) without running `go list`, for lightweight checks
// such as `ggui doctor`.
func CandidateDir(projectRoot, mainDir, configured string) (string, bool) {
	if configured != "" {
		if isDirArg(configured) && !filepath.IsAbs(configured) {
			return filepath.Join(projectRoot, configured), true
		}
		return configured, true
	}
	for _, rel := range DefaultCandidates {
		cand := filepath.Join(projectRoot, rel)
		if mainDir != "" && sameDir(cand, mainDir) {
			continue
		}
		if IsMainPackageDir(cand) {
			return cand, false
		}
	}
	return "", false
}

// IsMainPackageDir reports whether dir exists and contains at least one
// non-test Go source file declaring `package main`.
func IsMainPackageDir(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.PackageClauseOnly)
		if err != nil || f == nil || f.Name == nil {
			continue
		}
		if f.Name.Name == "main" {
			return true
		}
	}
	return false
}

func loadMainPackage(ctx context.Context, projectRoot, arg string, explicit bool) (*Package, error) {
	mp, err := gotool.Main(ctx, gotool.ListOptions{Dir: projectRoot}, arg)
	if err != nil {
		return nil, fmt.Errorf("installer package %s: %w", arg, err)
	}
	return &Package{
		Arg:        arg,
		Dir:        mp.Dir,
		ImportPath: mp.ImportPath,
		Explicit:   explicit,
	}, nil
}

func resolveArg(arg, baseDir string) (string, error) {
	if !isDirArg(arg) {
		return arg, nil
	}
	if filepath.IsAbs(arg) {
		return filepath.Clean(arg), nil
	}
	if baseDir != "" {
		return filepath.Clean(filepath.Join(baseDir, arg)), nil
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func isDirArg(arg string) bool {
	return arg == "." || arg == ".." || filepath.IsAbs(arg) ||
		strings.HasPrefix(arg, "./") || strings.HasPrefix(arg, "../") ||
		strings.HasPrefix(arg, `.\`) || strings.HasPrefix(arg, `..\`)
}

func sameDir(a, b string) bool {
	aa, err1 := filepath.Abs(a)
	bb, err2 := filepath.Abs(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	if ra, err := filepath.EvalSymlinks(aa); err == nil {
		aa = ra
	}
	if rb, err := filepath.EvalSymlinks(bb); err == nil {
		bb = rb
	}
	return aa == bb
}
