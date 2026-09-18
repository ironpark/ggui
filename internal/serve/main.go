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
// A successful build is kept for the life of the process; fetching /_rebuild
// drops it so the next request compiles again. A failed build is not cached:
// fix the code and reload.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

var (
	flagHTTP = flag.String("http", ":8080", "HTTP bind address to serve")
	flagTags = flag.String("tags", "", "build tags, passed to go build")
)

// build is one compilation of the target: in flight until done is closed,
// then either a path to the binary or the error that came out of the
// toolchain.
type build struct {
	done    chan struct{}
	path    string
	err     error
	size    int64
	elapsed time.Duration
}

// builder compiles the target once and keeps the result. A build that
// failed is not kept, so the next request tries again; a build that
// succeeded is served until invalidate drops it.
type builder struct {
	target, tags, dir string

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

// forget drops bd if it is still the current build, so a failure is retried
// rather than served again.
func (b *builder) forget(bd *build) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cur == bd {
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
	args = append(args, "-o", out, b.target)
	log.Printf("GOOS=js GOARCH=wasm go %s", strings.Join(args, " "))

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if msg, err := cmd.CombinedOutput(); err != nil {
		bd.err = fmt.Errorf("%s%w", msg, err)
		log.Printf("build failed: %v", bd.err)
		b.forget(bd)
		return
	}
	fi, err := os.Stat(out)
	if err != nil {
		bd.err = err
		b.forget(bd)
		return
	}
	bd.path, bd.size, bd.elapsed = out, fi.Size(), time.Since(start)
	log.Printf("built %s in %s (%.1f MB)", b.target, bd.elapsed.Round(time.Millisecond), float64(bd.size)/(1<<20))
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
	s.b.get() // start compiling while the page loads
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
	f, err := os.Open(bd.path)
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

// serveStatus answers what the build is doing, for the loading screen.
func (s *server) serveStatus(w http.ResponseWriter, r *http.Request) {
	bd := s.b.get()
	status := struct {
		State   string `json:"state"`
		Error   string `json:"error,omitempty"`
		Size    int64  `json:"size,omitempty"`
		Elapsed string `json:"elapsed,omitempty"`
	}{State: "building"}
	if bd.ready() {
		switch {
		case bd.err != nil:
			status.State, status.Error = "error", bd.err.Error()
		default:
			status.State, status.Size = "ready", bd.size
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

	s := &server{b: &builder{target: target, tags: *flagTags, dir: dir}}
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

// indexHTML is the page: a loading screen that follows the build, then the
// binary, and gets out of the way once the app has painted. Errors from the
// toolchain are shown where they happened rather than only in the terminal.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>ggui</title>
<style>
  :root {
    color-scheme: light dark;
    --bg: #fafafa; --fg: #18181b; --dim: #71717a; --line: #e4e4e7; --accent: #2563eb;
  }
  @media (prefers-color-scheme: dark) {
    :root { --bg: #09090b; --fg: #fafafa; --dim: #a1a1aa; --line: #27272a; --accent: #60a5fa; }
  }
  html, body { margin: 0; height: 100%; background: var(--bg); color: var(--fg); }
  body { font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, "Helvetica Neue", sans-serif; }
  canvas { display: block; }
  #loader {
    position: fixed; inset: 0; display: grid; place-content: center; justify-items: center;
    gap: 14px; background: var(--bg); transition: opacity .25s ease; z-index: 10;
  }
  #loader.done { opacity: 0; pointer-events: none; }
  .spinner {
    width: 26px; height: 26px; border-radius: 50%;
    border: 2px solid var(--line); border-top-color: var(--accent);
    animation: spin .7s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  #label { color: var(--dim); font-variant-numeric: tabular-nums; }
  #track { width: 220px; height: 3px; border-radius: 2px; background: var(--line); overflow: hidden; }
  #bar { width: 0; height: 100%; background: var(--accent); transition: width .15s linear; }
  #track.indeterminate #bar { width: 40%; animation: sweep 1.1s ease-in-out infinite; }
  @keyframes sweep { 0% { margin-left: -40%; } 100% { margin-left: 100%; } }
  #error {
    display: none; margin: 0; padding: 20px; max-width: min(90vw, 900px); max-height: 70vh;
    overflow: auto; border: 1px solid var(--line); border-radius: 8px; background: var(--bg);
    font: 12px/1.6 ui-monospace, SFMono-Regular, Menlo, monospace; white-space: pre-wrap;
  }
  #error:not(:empty) { display: block; }
</style>
<div id="loader">
  <div class="spinner" id="spinner"></div>
  <div id="track" class="indeterminate"><div id="bar"></div></div>
  <div id="label">Building…</div>
  <pre id="error"></pre>
</div>
<script src="wasm_exec.js"></script>
<script>
const loader = document.getElementById('loader');
const label = document.getElementById('label');
const track = document.getElementById('track');
const bar = document.getElementById('bar');
const spinner = document.getElementById('spinner');

const fail = (text) => {
  spinner.style.display = 'none';
  track.style.display = 'none';
  label.textContent = 'Build failed';
  document.getElementById('error').textContent = text;
};

const hide = () => {
  loader.classList.add('done');
  setTimeout(() => loader.remove(), 300);
};

// The loader goes once the app has painted, not the moment Go starts, so
// there is no blank frame between the two.
const hideWhenPainted = () => {
  const canvas = () => document.querySelector('canvas');
  const paint = () => requestAnimationFrame(() => requestAnimationFrame(hide));
  if (canvas()) { paint(); return; }
  const obs = new MutationObserver(() => {
    if (canvas()) { obs.disconnect(); paint(); }
  });
  obs.observe(document.body, { childList: true, subtree: true });
  setTimeout(() => { obs.disconnect(); hide(); }, 8000);
};

// Phase one: wait for the build the server started when this page was asked
// for, reporting how long it is taking.
const waitForBuild = async () => {
  const started = Date.now();
  for (;;) {
    const status = await (await fetch('_status', { cache: 'no-store' })).json();
    if (status.state === 'ready') return status;
    if (status.state === 'error') throw new Error(status.error);
    const secs = Math.round((Date.now() - started) / 1000);
    label.textContent = secs < 1 ? 'Building…' : 'Building… ' + secs + 's';
    await new Promise((r) => setTimeout(r, 250));
  }
};

// Phase two: download the binary, which the server has cached, so a reload
// is usually a 304 and this bar fills at once.
const download = async (expected) => {
  const resp = await fetch(mainWasmURL, { cache: 'default' });
  if (!resp.ok) throw new Error(await resp.text());
  const total = Number(resp.headers.get('Content-Length')) || expected || 0;
  if (!resp.body || !total) return resp;
  track.classList.remove('indeterminate');
  let loaded = 0;
  const stream = new ReadableStream({
    start(controller) {
      const reader = resp.body.getReader();
      const pump = () => reader.read().then(({ done, value }) => {
        if (done) { controller.close(); return; }
        loaded += value.byteLength;
        const pct = Math.min(100, Math.round((loaded / total) * 100));
        bar.style.width = pct + '%';
        label.textContent = 'Loading… ' + pct + '%';
        controller.enqueue(value);
        pump();
      }).catch((e) => controller.error(e));
      pump();
    },
  });
  return new Response(stream, { headers: { 'Content-Type': 'application/wasm' } });
};

const mainWasmURL = 'main.wasm';

(async () => {
  try {
    const status = await waitForBuild();
    const resp = await download(status.size);
    label.textContent = 'Starting…';
    track.classList.add('indeterminate');
    const go = new Go();
    const { instance } = await WebAssembly.instantiateStreaming(resp, go.importObject);
    hideWhenPainted();
    go.run(instance).catch((e) => fail(String(e && e.stack || e)));
  } catch (e) {
    fail(String(e && e.message || e));
  }
})();
</script>
</html>
`
