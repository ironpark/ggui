// Package web builds a ggui application for the browser: the js/wasm
// binary, shrunk by binaryen's wasm-opt when it is installed and gzipped
// once, the toolchain's JavaScript loader, and a page that loads them
// while showing what it is doing. ggui build --target web writes the
// result to a directory any static host can serve; ggui dev --web serves
// it itself and rebuilds it as the code changes.
package web

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// WasmName is the binary's file name, beside the page.
const WasmName = "main.wasm"

// BuildOptions says what to build.
type BuildOptions struct {
	Dir     string // the directory the go command runs in
	Package string
	Tags    []string
	// LDFlags are the linker flags. The usual -s -w drop the symbol table
	// and DWARF, which a browser never reads and would otherwise download.
	LDFlags  string
	Trimpath bool
	// WasmOpt is wasm-opt's flags, such as -O2; empty skips the pass.
	WasmOpt string
	// Out is the binary to write; its gzipped copy goes beside it.
	Out string
	// Logf reports progress and anything skipped; nil is silent.
	Logf func(format string, args ...any)
	// TmpDir, when set, is where the go command makes its work directory;
	// a build that is cancelled leaves it behind.
	TmpDir string
}

// Result is a finished build.
type Result struct {
	Wasm    string // the binary
	Gzip    string // its gzipped copy, "" when compressing failed
	Size    int64  // the binary's size, what the page counts against
	GzSize  int64
	Elapsed time.Duration
}

// Build compiles the package for js/wasm, optimizes it when it can and
// gzips it. A failure to compile is a *gotool.BuildError; a failure to
// optimize or compress is only logged, since the binary as linked still
// runs and an uncompressed one is still served.
func Build(ctx context.Context, o BuildOptions) (*Result, error) {
	logf := o.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	start := time.Now()
	bo := gotool.BuildOptions{
		Dir: o.Dir, Package: o.Package, Output: o.Out, Tags: o.Tags,
		LDFlags: o.LDFlags, Trimpath: o.Trimpath, GOOS: "js", GOARCH: "wasm",
		TmpDir: o.TmpDir,
	}
	logf("%s", bo)
	if err := gotool.Build(ctx, bo); err != nil {
		return nil, err
	}
	fi, err := os.Stat(o.Out)
	if err != nil {
		return nil, err
	}
	r := &Result{Wasm: o.Out, Size: fi.Size()}

	if o.WasmOpt != "" {
		switch err := Optimize(ctx, o.Out, o.WasmOpt, logf); {
		case errors.Is(err, ErrNoWasmOpt):
			logf("wasm-opt is not on PATH, so the binary ships as linked (install binaryen: brew install binaryen, apt install binaryen)")
		case err != nil:
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			logf("wasm-opt failed, so the binary ships as linked: %v", err)
		default:
			if fi, err := os.Stat(o.Out); err == nil {
				r.Size = fi.Size()
			}
		}
	}

	// Compressing costs a fraction of what linking did, and every later
	// request is served from the result.
	if n, err := Gzip(o.Out); err != nil {
		logf("gzip %s: %v (serving it uncompressed)", o.Out, err)
	} else {
		r.Gzip, r.GzSize = o.Out+".gz", n
	}
	r.Elapsed = time.Since(start)
	return r, nil
}

// wasmFeatures are the WebAssembly extensions the Go toolchain emits, which
// wasm-opt refuses to read unless they are named. Only these: -all would also
// let it write proposals no browser ships yet, and the binary then fails to
// instantiate.
var wasmFeatures = []string{
	"--enable-bulk-memory",
	"--enable-bulk-memory-opt",
	"--enable-nontrapping-float-to-int",
	"--enable-sign-ext",
	"--enable-mutable-globals",
	"--enable-simd", // the runtime's memory routines, from Go 1.27
}

// ErrNoWasmOpt reports that binaryen is not installed, which is a reason to
// ship the binary as linked rather than to fail the build.
var ErrNoWasmOpt = errors.New("wasm-opt not found")

// Optimize rewrites the binary in place with binaryen's wasm-opt.
//
// Most of what reaches the wire is gzip's doing: on a 35 MB binary, -O2
// takes 2% off what the browser must compile but only 0.4% off the bytes
// sent, for about seven seconds. That is worth it for a release and not
// for a development rebuild, which is why ggui dev skips it by default.
func Optimize(ctx context.Context, path, flags string, logf func(string, ...any)) error {
	bin, err := exec.LookPath("wasm-opt")
	if err != nil {
		return ErrNoWasmOpt
	}
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp := path + ".opt"
	args := append(append([]string{}, wasmFeatures...), strings.Fields(flags)...)
	args = append(args, path, "-o", tmp)

	start := time.Now()
	if msg, err := exec.CommandContext(ctx, bin, args...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s%w", msg, err)
	}
	after, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err := shared.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	logf("wasm-opt %s: %.1f MB to %.1f MB in %s", flags, mb(before.Size()), mb(after.Size()),
		time.Since(start).Round(time.Millisecond))
	return nil
}

// Gzip writes path.gz beside path and returns its size. The default level
// is the one worth having: on a 36 MB binary it saves 45% in a quarter
// second, where the best level spends seven times as long for another
// percent.
func Gzip(path string) (size int64, err error) {
	src, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer src.Close()

	out := path + ".gz"
	dst, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := dst.Close(); err == nil {
			err = cerr
		}
	}()

	zw := gzip.NewWriter(dst)
	if _, err := io.Copy(zw, src); err != nil {
		zw.Close()
		return 0, err
	}
	if err := zw.Close(); err != nil {
		return 0, err
	}
	fi, err := dst.Stat()
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}

// WasmExec returns the path of the toolchain's JavaScript loader, which
// must come from the toolchain that built the binary: the two are a pair,
// and a loader from another Go version fails at instantiation. It moved
// from misc/wasm to lib/wasm in Go 1.24.
func WasmExec(ctx context.Context, dir string) (string, error) {
	env, err := gotool.Env(ctx, dir, nil, "GOROOT")
	if err != nil {
		return "", err
	}
	root := env["GOROOT"]
	for _, d := range []string{"lib", "misc"} {
		p := filepath.Join(root, d, "wasm", "wasm_exec.js")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
	}
	return "", fmt.Errorf("wasm_exec.js is not in %s; is the Go installation complete?", root)
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }

// MB formats a byte count for a log line.
func MB(n int64) string { return fmt.Sprintf("%.1f MB", mb(n)) }
