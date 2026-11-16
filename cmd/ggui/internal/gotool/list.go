package gotool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// Module is a module as go list -m reports it.
type Module struct {
	Path      string
	Version   string
	Main      bool
	Dir       string
	GoMod     string
	GoVersion string
	Replace   *Module
	Error     *struct{ Err string }
}

// Local reports whether the module's source is a directory the developer
// edits: a workspace or main module, or one replaced by a directory, as
// opposed to a version unpacked read-only in the module cache.
func (m *Module) Local() bool {
	return m != nil && (m.Main || m.Replace != nil && m.Replace.Version == "")
}

// Package is a package as go list reports it, with the fields ggui uses.
type Package struct {
	ImportPath string
	Name       string
	Dir        string
	Standard   bool
	Module     *Module
	GoFiles    []string
	CgoFiles   []string
	EmbedFiles []string
	// Other files a build reads: assembly and the like.
	SFiles    []string
	HFiles    []string
	SysoFiles []string
	Error     *struct{ Err string }
}

// Files returns the files of the package a build reads, as absolute paths.
func (p *Package) Files() []string {
	var out []string
	for _, list := range [][]string{p.GoFiles, p.CgoFiles, p.EmbedFiles, p.SFiles, p.HFiles, p.SysoFiles} {
		for _, f := range list {
			out = append(out, filepath.Join(p.Dir, f))
		}
	}
	return out
}

// ListOptions says which build a listing describes, since the files a
// package is built from depend on the platform and the tags.
type ListOptions struct {
	Dir    string
	Tags   []string
	GOOS   string
	GOARCH string
}

func (o ListOptions) env() []string {
	var env []string
	if o.GOOS != "" {
		env = append(env, "GOOS="+o.GOOS)
	}
	if o.GOARCH != "" {
		env = append(env, "GOARCH="+o.GOARCH)
	}
	return env
}

// Deps lists pkg and every package it imports, directly or not. Errors in
// the packages are kept on them rather than failing the listing, so a
// program that does not compile yet still has files to watch.
func Deps(ctx context.Context, o ListOptions, pkg string) ([]Package, error) {
	args := []string{"list", "-e", "-deps",
		"-json=ImportPath,Name,Dir,Standard,Module,GoFiles,CgoFiles,EmbedFiles,SFiles,HFiles,SysoFiles,Error"}
	if len(o.Tags) > 0 {
		args = append(args, "-tags", strings.Join(o.Tags, ","))
	}
	args = append(args, pkg)
	out, err := output(ctx, o.Dir, o.env(), args...)
	if err != nil {
		return nil, err
	}
	return decodeStream[Package](out)
}

// Main returns pkg itself, which must be a main package.
func Main(ctx context.Context, o ListOptions, pkg string) (*Package, error) {
	args := []string{"list", "-e", "-json=ImportPath,Name,Dir,Module,Error"}
	if len(o.Tags) > 0 {
		args = append(args, "-tags", strings.Join(o.Tags, ","))
	}
	out, err := output(ctx, o.Dir, o.env(), append(args, pkg)...)
	if err != nil {
		return nil, err
	}
	pkgs, err := decodeStream[Package](out)
	if err != nil {
		return nil, err
	}
	if len(pkgs) != 1 {
		return nil, fmt.Errorf("%s names %d packages; name one main package", pkg, len(pkgs))
	}
	p := &pkgs[0]
	if p.Error != nil {
		return nil, errors.New(p.Error.Err)
	}
	if p.Name != "main" {
		return nil, fmt.Errorf("%s is package %s, not a main package: name the directory that holds func main", pkg, p.Name)
	}
	return p, nil
}

// Modules reports the named modules as the build in dir resolves them.
// A module the build does not use comes back with its Error set.
func Modules(ctx context.Context, dir string, paths ...string) ([]Module, error) {
	out, err := output(ctx, dir, nil, append([]string{"list", "-m", "-e", "-json"}, paths...)...)
	if err != nil {
		return nil, err
	}
	return decodeStream[Module](out)
}

// ModFile is what a go.mod file asks for.
type ModFile struct {
	Module  struct{ Path string }
	Go      string
	Require []struct {
		Path     string
		Version  string
		Indirect bool
	}
}

// ReadModFile reads the go.mod at path through go mod edit, which parses
// any go.mod, not only the main module's, so ggui needs no parser of its
// own.
func ReadModFile(ctx context.Context, path string) (*ModFile, error) {
	out, err := output(ctx, filepath.Dir(path), []string{"GOWORK=off", "GOFLAGS="}, "mod", "edit", "-json", path)
	if err != nil {
		return nil, err
	}
	var mf ModFile
	if err := json.Unmarshal(out, &mf); err != nil {
		return nil, fmt.Errorf("go mod edit: %w", err)
	}
	return &mf, nil
}

// Required returns the version the go.mod requires of path, or "".
func (mf *ModFile) Required(path string) string {
	for _, r := range mf.Require {
		if r.Path == path {
			return r.Version
		}
	}
	return ""
}

// decodeStream decodes the concatenated JSON objects go list writes.
func decodeStream[T any](data []byte) ([]T, error) {
	dec := json.NewDecoder(strings.NewReader(string(data)))
	var out []T
	for {
		var v T
		if err := dec.Decode(&v); err == io.EOF {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("go list: %w", err)
		}
		out = append(out, v)
	}
}
