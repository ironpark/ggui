package web

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// BundleOptions says what to write.
type BundleOptions struct {
	BuildOptions
	// OutDir is the directory the bundle is written to. Files a previous
	// bundle wrote there are replaced; nothing else in it is touched.
	OutDir string
	Title  string
	Base   string
}

// Files are what a bundle holds, each with a gzipped copy beside it.
var Files = []string{"index.html", WasmName, "wasm_exec.js"}

// WriteBundle builds the app and writes a directory a static host can
// serve: the page, the binary and the loader, each with a .gz copy that a
// host able to serve precompressed files (nginx's gzip_static, Caddy's
// precompressed, most CDNs) sends to browsers that take gzip, which is
// all of them, for a little over half the bytes.
func WriteBundle(ctx context.Context, o BundleOptions) (*Result, error) {
	if err := os.MkdirAll(o.OutDir, 0o755); err != nil {
		return nil, err
	}
	o.Out = filepath.Join(o.OutDir, WasmName)
	r, err := Build(ctx, o.BuildOptions)
	if err != nil {
		return nil, err
	}
	if r.Gzip == "" {
		return nil, errors.New("could not gzip the binary; see above")
	}

	loader, err := WasmExec(ctx, o.Dir)
	if err != nil {
		return nil, err
	}
	js, err := os.ReadFile(loader)
	if err != nil {
		return nil, err
	}
	page := Page(PageOptions{Title: o.Title, Base: o.Base, Size: r.Size})
	for name, data := range map[string][]byte{"index.html": page, "wasm_exec.js": js} {
		p := filepath.Join(o.OutDir, name)
		if err := os.WriteFile(p, data, 0o644); err != nil {
			return nil, err
		}
		if _, err := Gzip(p); err != nil {
			return nil, fmt.Errorf("gzip %s: %w", p, err)
		}
	}
	return r, nil
}
