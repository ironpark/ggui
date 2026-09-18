// Command serve builds a Go package for js/wasm and serves it over HTTP for
// development, in the shape of hajimehoshi/wasmserve with two differences:
// the build is cached, so reloading the page serves the binary already built
// instead of compiling again, and the page shows a loading screen while the
// build runs and the binary downloads.
//
// Usage:
//
//	go run ./internal/serve -http :3000 ./examples/gallery
//
// Each build is linked without its symbol table and DWARF, run through
// binaryen's wasm-opt, and gzipped once, so a browser on a phone or at the
// end of a tunnel fetches about half as many bytes; the page is unaware,
// because the browser decodes Content-Encoding before the wasm is
// instantiated. Builds without binaryen installed are served as linked.
//
// Most of what reaches the wire is gzip's doing: on a 35 MB binary, -O2
// takes 2% off the binary the browser must compile but only 0.4% off the
// bytes actually sent, for about seven seconds. Pass -wasm-opt="" to skip it
// and keep the build loop short.
//
// A build is kept for the life of the process; fetching /_rebuild drops it
// so the next request compiles again, and so does loading the page when the
// build that is kept failed. Reloading is therefore the way to try again
// after fixing the code, while the status the loading screen polls is only
// read.
package main

import (
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const mainWasm = "main.wasm"

// indexHTML is the page: a loading screen that follows the build, then the
// binary, and gets out of the way once the app has painted. Errors from the
// toolchain are shown where they happened rather than only in the terminal.
//
//go:embed index.html
var indexHTML string

var (
	flagHTTP    = flag.String("http", ":8080", "HTTP bind address to serve")
	flagTags    = flag.String("tags", "", "build tags, passed to go build")
	flagLDFlags = flag.String("ldflags", "-s -w", "linker flags; the default drops the symbol table and DWARF")
	flagWasmOpt = flag.String("wasm-opt", "-O2", `binaryen wasm-opt flags to run after building, e.g. "-O2" or "-Oz"; empty to skip`)
)

// build is one compilation of the target: in flight until done is closed,
// then either a path to the binary or the error that came out of the
// toolchain.
type build struct {
	done    chan struct{}
	path    string
	gzPath  string // the gzipped copy, empty when compressing failed
	err     error
	size    int64
	gzSize  int64
	elapsed time.Duration
}

// builder compiles the target once and keeps the result, success or
// failure, until invalidate drops it.
type builder struct {
	target, tags, ldflags, wasmOpt, dir string

	mu  sync.Mutex
	cur *build
}

// get returns the current build, starting one if there is none.
func (b *builder) get() *build {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur == nil {
		b.cur = &build{done: make(chan struct{})}
		go b.run(b.cur)
	}
	return b.cur
}

func (b *builder) invalidate() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cur = nil
}

// retryIfFailed drops a finished build that failed, so the next get starts a
// fresh one. Only a page load calls it: were every status poll to retry, a
// build that cannot succeed would start the compiler four times a second.
func (b *builder) retryIfFailed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur != nil && b.cur.ready() && b.cur.err != nil {
		b.cur = nil
	}
}

func (b *builder) run(bd *build) {
	defer close(bd.done)
	start := time.Now()
	out := filepath.Join(b.dir, mainWasm)
	args := []string{"build"}
	if b.tags != "" {
		args = append(args, "-tags", b.tags)
	}
	if b.ldflags != "" {
		args = append(args, "-ldflags", b.ldflags)
	}
	args = append(args, "-o", out, b.target)
	log.Printf("GOOS=js GOARCH=wasm go %s", strings.Join(args, " "))

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if msg, err := cmd.CombinedOutput(); err != nil {
		bd.err = fmt.Errorf("%s%w", msg, err)
		log.Printf("build failed: %v", bd.err)
		return
	}
	fi, err := os.Stat(out)
	if err != nil {
		bd.err = err
		return
	}
	bd.path, bd.size = out, fi.Size()

	// Optimizing is worth seconds but never correctness: a binary that was
	// not optimized is still a binary that runs.
	if b.wasmOpt != "" {
		switch err := optimize(out, b.wasmOpt); {
		case errors.Is(err, errNoWasmOpt):
			log.Print("wasm-opt not on PATH, serving the binary as linked (brew install binaryen)")
		case err != nil:
			log.Printf("wasm-opt failed, serving the binary as linked: %v", err)
		default:
			if fi, err := os.Stat(out); err == nil {
				bd.size = fi.Size()
			}
		}
	}

	// Compressing costs a fraction of what linking did, and every later
	// request is served from the result.
	if gz, n, err := compress(out); err != nil {
		log.Printf("gzip: %v (serving uncompressed)", err)
	} else {
		bd.gzPath, bd.gzSize = gz, n
	}

	bd.elapsed = time.Since(start)
	log.Printf("built %s in %s (%.1f MB%s)", b.target, bd.elapsed.Round(time.Millisecond), mb(bd.size), gzNote(bd))
}

func mb(n int64) float64 { return float64(n) / (1 << 20) }

func gzNote(bd *build) string {
	if bd.gzSize == 0 {
		return ""
	}
	return fmt.Sprintf(", %.1f MB gzipped", mb(bd.gzSize))
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
}

// errNoWasmOpt reports that binaryen is not installed, which is a reason to
// serve the binary as linked rather than to fail the build.
var errNoWasmOpt = errors.New("wasm-opt not found")

// optimize rewrites the binary in place with binaryen's wasm-opt.
func optimize(path, flags string) error {
	bin, err := exec.LookPath("wasm-opt")
	if err != nil {
		return errNoWasmOpt
	}
	before, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp := path + ".opt"
	args := append(append([]string{}, wasmFeatures...), strings.Fields(flags)...)
	args = append(args, path, "-o", tmp)

	start := time.Now()
	if msg, err := exec.Command(bin, args...).CombinedOutput(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("%s%w", msg, err)
	}
	after, err := os.Stat(tmp)
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	log.Printf("wasm-opt %s: %.1f MB to %.1f MB in %s", flags, mb(before.Size()), mb(after.Size()),
		time.Since(start).Round(time.Millisecond))
	return nil
}

// compress writes a gzip copy beside the binary and returns its path and
// size. The default level is the one worth having: on a 36 MB binary it
// saves 45%% in a quarter second, where the best level spends seven times
// as long for another percent.
func compress(path string) (string, int64, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer src.Close()

	out := path + ".gz"
	dst, err := os.Create(out)
	if err != nil {
		return "", 0, err
	}
	defer dst.Close()

	zw := gzip.NewWriter(dst)
	if _, err := io.Copy(zw, src); err != nil {
		zw.Close()
		return "", 0, err
	}
	if err := zw.Close(); err != nil {
		return "", 0, err
	}
	fi, err := dst.Stat()
	if err != nil {
		return "", 0, err
	}
	return out, fi.Size(), nil
}

// wait blocks until the build finishes.
func (bd *build) wait() *build {
	<-bd.done
	return bd
}

// ready reports whether the build has finished, without waiting for it.
func (bd *build) ready() bool {
	select {
	case <-bd.done:
		return true
	default:
		return false
	}
}

type server struct {
	b *builder
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch path := strings.TrimPrefix(r.URL.Path, "/"); path {
	case "", "index.html":
		s.serveIndex(w, r)
	case mainWasm:
		s.serveWasm(w, r)
	case "wasm_exec.js":
		serveWasmExec(w, r)
	case "_status":
		s.serveStatus(w, r)
	case "_rebuild":
		s.b.invalidate()
		w.WriteHeader(http.StatusNoContent)
	default:
		// Anything else comes from the target's own directory, so a package
		// can ship an asset it fetches at runtime.
		http.ServeFile(w, r, filepath.Join(s.b.target, filepath.Clean("/"+path)))
	}
}

func (s *server) serveIndex(w http.ResponseWriter, r *http.Request) {
	s.b.retryIfFailed() // a reload is how you ask for another go at it
	s.b.get()           // start compiling while the page loads
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, indexHTML)
}

func (s *server) serveWasm(w http.ResponseWriter, r *http.Request) {
	bd := s.b.get().wait()
	if bd.err != nil {
		http.Error(w, bd.err.Error(), http.StatusInternalServerError)
		return
	}
	// A client that takes gzip gets the copy compressed at build time. The
	// browser decodes it before instantiateStreaming sees the stream, so the
	// page needs to know nothing about this; Vary keeps any cache in between
	// from handing the encoded bytes to a client that cannot read them.
	path := bd.path
	w.Header().Set("Vary", "Accept-Encoding")
	if bd.gzPath != "" && acceptsGzip(r) {
		path = bd.gzPath
		w.Header().Set("Content-Encoding", "gzip")
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// instantiateStreaming insists on this type, and ServeContent would
	// otherwise sniff the bytes.
	w.Header().Set("Content-Type", "application/wasm")
	// The binary is stable until it is built again, so a reload can come
	// out of the browser's cache instead of over the wire.
	http.ServeContent(w, r, mainWasm, fi.ModTime(), f)
}

// acceptsGzip reports whether the client asked for gzip and did not then
// refuse it with q=0.
func acceptsGzip(r *http.Request) bool {
	for _, enc := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(enc), ";")
		if name != "gzip" {
			continue
		}
		return strings.ReplaceAll(strings.TrimSpace(params), " ", "") != "q=0"
	}
	return false
}

// serveStatus answers what the build is doing, for the loading screen.
func (s *server) serveStatus(w http.ResponseWriter, r *http.Request) {
	bd := s.b.get()
	// Size is the binary as the page will see it, decoded, which is what its
	// progress bar counts; Compressed is only for reporting.
	status := struct {
		State      string `json:"state"`
		Error      string `json:"error,omitempty"`
		Size       int64  `json:"size,omitempty"`
		Compressed int64  `json:"compressed,omitempty"`
		Elapsed    string `json:"elapsed,omitempty"`
	}{State: "building"}
	if bd.ready() {
		switch {
		case bd.err != nil:
			status.State, status.Error = "error", bd.err.Error()
		default:
			status.State, status.Size = "ready", bd.size
			status.Compressed = bd.gzSize
			status.Elapsed = bd.elapsed.Round(time.Millisecond).String()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(status)
}

// serveWasmExec hands over the toolchain's JavaScript loader, which moved
// from misc/wasm to lib/wasm in Go 1.24.
func serveWasmExec(w http.ResponseWriter, r *http.Request) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	root := strings.TrimSpace(string(out))
	for _, dir := range []string{"lib", "misc"} {
		p := filepath.Join(root, dir, "wasm", "wasm_exec.js")
		if _, err := os.Stat(p); err == nil {
			http.ServeFile(w, r, p)
			return
		} else if !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	http.Error(w, "wasm_exec.js not found in "+root, http.StatusInternalServerError)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("serve: ")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: serve [flags] [target]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	target := "."
	if flag.NArg() > 0 {
		target = flag.Arg(0)
	}
	dir, err := os.MkdirTemp("", "ggui-serve-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(dir)

	s := &server{b: &builder{
		target:  target,
		tags:    *flagTags,
		ldflags: *flagLDFlags,
		wasmOpt: *flagWasmOpt,
		dir:     dir,
	}}
	s.b.get() // compile now, so the first page load usually finds it done

	srv := &http.Server{Addr: *flagHTTP, Handler: s}
	addr := *flagHTTP
	if strings.HasPrefix(addr, ":") {
		addr = "localhost" + addr
	}
	log.Printf("serving %s at http://%s/", target, addr)

	// Leave the temp directory behind on Ctrl-C, not on the next boot.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		os.RemoveAll(dir)
		log.Fatal(err)
	}
}
