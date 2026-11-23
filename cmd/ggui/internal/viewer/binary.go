package viewer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// Package is the viewer's main package, in ggui's module.
const Package = "github.com/ironpark/ggui/cmd/ggviewer"

// Unavailable is why remote display cannot be had: the reason ggui dev
// opens the app's own window instead.
type Unavailable struct{ Reason string }

func (u *Unavailable) Error() string { return u.Reason }

// Supported reports, with an *Unavailable, why remote display is not
// offered on this system. The viewer builds on unix systems and Windows,
// and is made and tried on macOS, Linux with X11, and Windows 10 1803 or
// later, the first with unix sockets: on an older Windows it does not
// start, and the app opens its own window.
func Supported() error {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return nil
	}
	return &Unavailable{"remote display is available on macOS, Linux and Windows"}
}

// Binary is the viewer a project's module graph builds.
type Binary struct {
	Dir  string // where the go command runs: the project
	Key  string // what the binary is built from, hashed
	Path string // the binary in the cache, which may not exist yet

	// files are the editable files the viewer is built from, and mods the
	// files that decide the module graph.
	files map[string]bool
}

// The environment that changes what go build makes of the same sources.
var buildEnv = []string{"GOVERSION", "GOOS", "GOARCH", "GOFLAGS", "CGO_ENABLED", "GOEXPERIMENT", "GOAMD64", "GOARM64", "GOWORK"}

// Resolve finds the viewer in the module graph of the project in dir and
// says where in cacheDir its binary is kept. When the graph has no viewer,
// or a viewer without the ggwd it needs, the error is an *Unavailable.
//
// The key covers what decides the binary: the Go version, the target and
// the build environment, and for every package the viewer is built from,
// its module's version, or, for a module being edited (the main module,
// a workspace module or a replacement by a directory), the contents of
// the package's files. A changed dependency thus makes another binary,
// and a binary built once is found again by every later session.
//
// It does not ask whether the system is one remote display is offered on:
// see Supported.
func Resolve(ctx context.Context, dir, cacheDir string) (*Binary, error) {
	env, err := gotool.Env(ctx, dir, nil, buildEnv...)
	if err != nil {
		return nil, err
	}
	pkgs, err := gotool.Deps(ctx, gotool.ListOptions{Dir: dir}, Package)
	if err != nil {
		return nil, err
	}
	if err := missing(ctx, dir, pkgs); err != nil {
		return nil, err
	}

	b := &Binary{Dir: dir, files: map[string]bool{}}
	h := sha256.New()
	for _, k := range buildEnv {
		fmt.Fprintf(h, "%s=%s\n", k, env[k])
	}
	slices.SortFunc(pkgs, func(a, b gotool.Package) int { return strings.Compare(a.ImportPath, b.ImportPath) })
	for i := range pkgs {
		p := &pkgs[i]
		if p.Standard {
			continue // GOVERSION covers it
		}
		m := p.Module
		switch {
		case m == nil:
			fmt.Fprintf(h, "%s no module\n", p.ImportPath)
		case m.Local() || m.Version == "":
			fmt.Fprintf(h, "%s local\n", p.ImportPath)
			for _, f := range p.Files() {
				b.files[f] = true
				if err := hashFile(h, f); err != nil {
					return nil, err
				}
			}
		case m.Replace != nil:
			fmt.Fprintf(h, "%s %s@%s => %s@%s\n", p.ImportPath, m.Path, m.Version, m.Replace.Path, m.Replace.Version)
		default:
			fmt.Fprintf(h, "%s %s@%s\n", p.ImportPath, m.Path, m.Version)
		}
	}
	b.Key = hex.EncodeToString(h.Sum(nil))[:20]
	name := "ggviewer"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	b.Path = filepath.Join(cacheDir, b.Key, name)
	return b, nil
}

// missing returns an *Unavailable when the listing has no viewer, or a
// viewer whose packages fail to load, saying which module is too old.
func missing(ctx context.Context, dir string, pkgs []gotool.Package) error {
	var top *gotool.Package
	for i := range pkgs {
		if pkgs[i].ImportPath == Package {
			top = &pkgs[i]
		}
	}
	if top == nil || top.Error != nil {
		return &Unavailable{fmt.Sprintf("the project's ggui (%s) has no %s", moduleVersion(ctx, dir, "github.com/ironpark/ggui"), Package)}
	}
	for _, p := range pkgs {
		if p.Error == nil {
			continue
		}
		// A package of ggwd's or ggfx's the viewer imports and the
		// project's version lacks: ggwd/remote, ggfx/remote.
		for _, mod := range []string{"github.com/ironpark/ggwd", "github.com/ironpark/ggfx"} {
			if strings.HasPrefix(p.ImportPath, mod+"/") {
				name := mod[strings.LastIndex(mod, "/")+1:]
				return &Unavailable{fmt.Sprintf("the project's %s (%s) has no %s", name, moduleVersion(ctx, dir, mod), p.ImportPath)}
			}
		}
		return &Unavailable{fmt.Sprintf("the viewer does not build: %s", firstLine(p.Error.Err))}
	}
	return nil
}

// moduleVersion describes the module at path as the project resolves it.
func moduleVersion(ctx context.Context, dir, path string) string {
	mods, err := gotool.Modules(ctx, dir, path)
	if err != nil || len(mods) == 0 || mods[0].Error != nil {
		return "not required"
	}
	m := mods[0]
	switch {
	case m.Main:
		return "workspace module " + m.Dir
	case m.Replace != nil && m.Replace.Version == "":
		return "replaced by " + m.Replace.Path
	default:
		return m.Version
	}
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return s
}

func hashFile(h io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(h, "%s\n", path)
	_, err = io.Copy(h, f)
	return err
}

// Affects reports whether a change to paths may change the binary: one
// of its editable files, or a file that decides the module graph.
func (b *Binary) Affects(paths []string) bool {
	for _, p := range paths {
		switch filepath.Base(p) {
		case "go.mod", "go.sum", "go.work", "go.work.sum":
			return true
		}
		if b.files[p] {
			return true
		}
	}
	return false
}

// keep is how many binaries the cache holds; the least recently used go.
const keep = 4

// Ensure makes sure the binary exists, building it if it does not, and
// reports whether it built it.
func (b *Binary) Ensure(ctx context.Context) (built bool, err error) {
	dir := filepath.Dir(b.Path)
	if _, err := os.Stat(b.Path); err == nil {
		now := time.Now()
		_ = os.Chtimes(dir, now, now) // used: the last to be pruned
		return false, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	// Built beside its final name and renamed into it, so that a session
	// starting meanwhile never runs half a binary.
	tmp, err := os.CreateTemp(dir, "build-*")
	if err != nil {
		return false, err
	}
	tmp.Close()
	defer os.Remove(tmp.Name())
	if err := gotool.Build(ctx, gotool.BuildOptions{Dir: b.Dir, Package: Package, Output: tmp.Name()}); err != nil {
		os.Remove(tmp.Name())
		os.Remove(dir) // if no other session builds there
		if be, ok := errors.AsType[*gotool.BuildError](err); ok {
			return false, fmt.Errorf("building the viewer failed:\n%w", be)
		}
		return false, err
	}
	if err := shared.Rename(tmp.Name(), b.Path); err != nil {
		return false, err
	}
	prune(filepath.Dir(dir), keep)
	return true, nil
}

// prune removes all but the n most recently used binaries in cacheDir.
func prune(cacheDir string, n int) {
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	type used struct {
		path string
		at   time.Time
	}
	var dirs []used
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if fi, err := e.Info(); err == nil {
			dirs = append(dirs, used{filepath.Join(cacheDir, e.Name()), fi.ModTime()})
		}
	}
	slices.SortFunc(dirs, func(a, b used) int { return b.at.Compare(a.at) })
	for _, d := range dirs[min(n, len(dirs)):] {
		os.RemoveAll(d.path)
	}
}

// CacheDir is where the viewer's binaries are kept: GGUI_CACHE_DIR when
// it is set, the user's cache directory's ggui otherwise.
func CacheDir() (string, error) {
	if d := os.Getenv("GGUI_CACHE_DIR"); d != "" {
		return filepath.Join(d, "viewer"), nil
	}
	d, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "ggui", "viewer"), nil
}
