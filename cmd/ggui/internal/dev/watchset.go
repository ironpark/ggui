package dev

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/watch"
)

// WatchSet returns the paths a change to which should rebuild the app: the files of
// every package it is built from that lives in a module being edited (the
// main module, a workspace module or one replaced by a directory, never
// the read-only module cache or the standard library), each such
// package's directory, so that a file added to it is seen, the go.mod and
// go.sum of those modules, the go.work in effect, and extra, which is
// ggui.toml and the paths dev.watch names, a directory standing for every
// file below it. Below a directory dev.watch names, what ig ignores is
// left out; the directory itself, named on purpose, is kept.
//
// The set follows the build: GOOS, GOARCH and the tags decide which files
// a package is made of, so the listing is made with the dev build's.
func WatchSet(ctx context.Context, lo gotool.ListOptions, pkg string, extra []string, ig *watch.Ignore) ([]string, error) {
	pkgs, err := gotool.Deps(ctx, lo, pkg)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for i := range pkgs {
		p := &pkgs[i]
		if p.Standard || !p.Module.Local() || p.Dir == "" {
			continue
		}
		add(p.Dir)
		for _, f := range p.Files() {
			add(f)
		}
		if mod := p.Module.Dir; mod != "" {
			add(filepath.Join(mod, "go.mod"))
			add(filepath.Join(mod, "go.sum"))
		}
	}
	if env, err := gotool.Env(ctx, lo.Dir, nil, "GOWORK"); err == nil {
		if w := env["GOWORK"]; w != "" && w != "off" {
			add(w)
			add(w + ".sum")
		}
	}
	for _, e := range extra {
		for _, p := range expand(e, ig) {
			add(p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// expand returns path and, when it is a directory, every directory and
// file below it, skipping hidden ones such as .git and those ig ignores.
func expand(path string, ig *watch.Ignore) []string {
	path = filepath.Clean(path)
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return []string{path}
	}
	var all []string
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable entry is skipped; the walk goes on
		}
		if p != path {
			if len(d.Name()) > 0 && d.Name()[0] == '.' {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() && ig != nil && ig.Ignored(p) {
				return filepath.SkipDir
			}
		}
		all = append(all, p)
		return nil
	})
	if len(all) <= 1 {
		return all
	}
	// Asked all at once: git answers for a whole tree in one run.
	ignored := ig.Filter(all[1:])
	out := all[:1]
	skip := make(map[string]bool)
	for i, p := range all[1:] {
		if ignored[i] {
			skip[p] = true
			continue
		}
		skipped := false
		for dir := filepath.Dir(p); dir != path && dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if skip[dir] {
				skipped = true
				break
			}
		}
		if !skipped {
			out = append(out, p)
		}
	}
	return out
}
