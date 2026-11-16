package dev

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
)

// WatchSet returns the paths a change to which should rebuild the app: the files of
// every package it is built from that lives in a module being edited (the
// main module, a workspace module or one replaced by a directory, never
// the read-only module cache or the standard library), each such
// package's directory, so that a file added to it is seen, the go.mod and
// go.sum of those modules, the go.work in effect, and extra, which is
// ggui.toml and the paths dev.watch names, a directory standing for every
// file below it.
//
// The set follows the build: GOOS, GOARCH and the tags decide which files
// a package is made of, so the listing is made with the dev build's.
func WatchSet(ctx context.Context, lo gotool.ListOptions, pkg string, extra []string) ([]string, error) {
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
		for _, p := range expand(e) {
			add(p)
		}
	}
	slices.Sort(out)
	return out, nil
}

// expand returns path and, when it is a directory, every directory and
// file below it, skipping hidden ones such as .git.
func expand(path string) []string {
	fi, err := os.Stat(path)
	if err != nil || !fi.IsDir() {
		return []string{path}
	}
	var out []string
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p != path && len(d.Name()) > 0 && d.Name()[0] == '.' {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out = append(out, p)
		return nil
	})
	return out
}
