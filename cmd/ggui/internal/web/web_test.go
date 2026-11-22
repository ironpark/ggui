package web

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPage(t *testing.T) {
	page := string(Page(PageOptions{Title: "Tom & Jerry", Base: "app", Size: 1234}))
	for _, want := range []string{
		"<title>Tom &amp; Jerry</title>",
		`<span class="brand">Tom &amp; Jerry</span>`,
		`<base href="/app/">`,
		`window.gguiConfig = {"dev":false,"size":1234};`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	// No base unless asked: a bundle then loads its files from wherever
	// it is put.
	if strings.Contains(string(Page(PageOptions{})), "<base") {
		t.Error("a page without a base has a <base>")
	}
	// The configuration cannot close the script it is in.
	if p := string(Page(PageOptions{Dev: true, Target: "</script><script>alert(1)"})); strings.Contains(p, "</script><script>alert") {
		t.Error("the target name escaped the configuration script")
	}
}

func TestCleanBase(t *testing.T) {
	for in, want := range map[string]string{"": "/", "/": "/", "app": "/app/", "/app": "/app/", "/a/b/": "/a/b/"} {
		if got := CleanBase(in); got != want {
			t.Errorf("CleanBase(%q) = %q, want %q", in, got, want)
		}
	}
}

// tinyApp writes a module whose main package builds for js/wasm in a
// second, standing in for an app.
func tinyApp(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a js/wasm binary")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module tiny\n\ngo 1.24\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { println(\"hi\") }\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("asset"), 0o644)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "")
	return dir
}

func gunzip(t *testing.T, path string) []byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestWriteBundle(t *testing.T) {
	dir := tinyApp(t)
	out := filepath.Join(dir, "build", "web")
	r, err := WriteBundle(t.Context(), BundleOptions{
		Dir: dir, Package: ".", LDFlags: "-s -w", Trimpath: true,
		OutDir: out,
		Title:  "Tiny",
		Base:   "/tiny/",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range Files {
		data, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(gunzip(t, filepath.Join(out, f+".gz")), data) {
			t.Errorf("%s.gz does not hold %s", f, f)
		}
	}
	wasm, _ := os.ReadFile(filepath.Join(out, WasmName))
	if !bytes.HasPrefix(wasm, []byte("\x00asm")) || int64(len(wasm)) != r.Size {
		t.Errorf("main.wasm: %d bytes, magic %q; result says %d", len(wasm), wasm[:4], r.Size)
	}
	page, _ := os.ReadFile(filepath.Join(out, "index.html"))
	for _, want := range []string{"<title>Tiny</title>", `<base href="/tiny/">`, `"dev":false`, `<script src="wasm_exec.js">`} {
		if !bytes.Contains(page, []byte(want)) {
			t.Errorf("index.html lacks %q", want)
		}
	}
	js, _ := os.ReadFile(filepath.Join(out, "wasm_exec.js"))
	if !bytes.Contains(js, []byte("Go = class")) {
		t.Error("wasm_exec.js is not the Go loader")
	}
}

func get(t *testing.T, srv *httptest.Server, path string, header ...string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	for i := 0; i+1 < len(header); i += 2 {
		req.Header.Set(header[i], header[i+1])
	}
	resp, err := srv.Client().Transport.RoundTrip(req) // no transparent gunzip
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func TestServer(t *testing.T) {
	dir := tinyApp(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s, err := NewServer(ctx, ServerOptions{
		Build:  BuildOptions{Dir: dir, Package: ".", LDFlags: "-s -w"},
		PkgDir: dir, Base: "/app/", Title: "Tiny", Target: ".", Live: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := httptest.NewServer(s)
	defer srv.Close()
	// Ending the server's context ends the event stream, which Close
	// would otherwise wait on; deferred last, it runs first.
	defer cancel()

	// The events stream opens with the current build.
	events := get(t, srv, "/app/_events")
	if ct := events.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("_events Content-Type = %q", ct)
	}
	evs := make(chan event, 16)
	go func() {
		sc := bufio.NewScanner(events.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var e event
				json.Unmarshal([]byte(data), &e)
				evs <- e
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			t.Errorf("event stream: %v", err)
		}
	}()
	next := func(state string) event {
		t.Helper()
		for {
			select {
			case e := <-evs:
				if e.State == state {
					return e
				}
				if e.State == "error" {
					t.Fatalf("build failed: %s", e.Error)
				}
			case <-time.After(60 * time.Second):
				t.Fatalf("no %s event", state)
			}
		}
	}
	first := next("ready")

	// The page, under the base, and for a route the app shows itself.
	for _, path := range []string{"/app/", "/app/projects/1"} {
		resp := get(t, srv, path, "Accept", "text/html")
		body, _ := io.ReadAll(resp.Body)
		if !bytes.Contains(body, []byte(`<base href="/app/">`)) || !bytes.Contains(body, []byte(`"dev":true`)) {
			t.Errorf("%s did not serve the dev page", path)
		}
	}
	if resp := get(t, srv, "/app"); resp.StatusCode != http.StatusMovedPermanently {
		t.Errorf("/app = %d, want a redirect to /app/", resp.StatusCode)
	}
	if resp := get(t, srv, "/elsewhere"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("/elsewhere = %d, want 404", resp.StatusCode)
	}
	if body, _ := io.ReadAll(get(t, srv, "/app/asset.txt").Body); string(body) != "asset" {
		t.Errorf("asset = %q", body)
	}

	// The binary, gzipped for a client that takes it, with the build's ETag.
	resp := get(t, srv, "/app/main.wasm", "Accept-Encoding", "gzip")
	if resp.Header.Get("Content-Encoding") != "gzip" || resp.Header.Get("Content-Type") != "application/wasm" {
		t.Errorf("main.wasm headers: %v", resp.Header)
	}
	if etag := resp.Header.Get("ETag"); !strings.Contains(etag, first.ID) {
		t.Errorf("ETag %s does not name build %s", etag, first.ID)
	}
	zr, err := gzip.NewReader(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	wasm, _ := io.ReadAll(zr)
	if !bytes.HasPrefix(wasm, []byte("\x00asm")) {
		t.Error("main.wasm is not WebAssembly")
	}

	var status struct{ ID, State string }
	json.NewDecoder(get(t, srv, "/app/_status").Body).Decode(&status)
	if status.State != "ready" || status.ID != first.ID {
		t.Errorf("_status = %+v", status)
	}

	// A rebuild is announced, then its result, under a new ID: what makes
	// the open page reload.
	s.Rebuild()
	building := next("building")
	ready := next("ready")
	if building.ID == first.ID || ready.ID != building.ID {
		t.Errorf("rebuild ids: first %s, building %s, ready %s", first.ID, building.ID, ready.ID)
	}

	// A broken build is reported with the compiler's message.
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() { undefined() }\n"), 0o644)
	s.Rebuild()
	for {
		e := <-evs
		if e.State == "error" {
			if !strings.Contains(e.Error, "undefined") {
				t.Errorf("error event = %q", e.Error)
			}
			break
		}
		if e.State == "ready" {
			t.Fatal("a broken build was reported ready")
		}
	}
}

func TestServerSecurityAndRebuild(t *testing.T) {
	outside := t.TempDir()
	secretFile := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("top-secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "asset.txt"), []byte("ok"), 0o644)
	_ = os.Symlink(secretFile, filepath.Join(dir, "escape.txt"))
	_ = os.Symlink(filepath.Join(dir, "main.go"), filepath.Join(dir, "sneaky.txt"))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	s, err := NewServer(ctx, ServerOptions{
		Build:  BuildOptions{Dir: dir, Package: "."},
		PkgDir: dir, Base: "/app/", Title: "Sec", Target: ".", Live: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	srv := httptest.NewServer(s)
	defer srv.Close()

	// Blocked static files: *.go, dotfiles/dotdirs, outside symlink, symlink to .go.
	for _, p := range []string{"/app/main.go", "/app/.env", "/app/.git/HEAD", "/app/escape.txt", "/app/sneaky.txt"} {
		resp := get(t, srv, p)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", p, resp.StatusCode)
		}
	}
	// Normal asset is still served.
	if resp := get(t, srv, "/app/asset.txt"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /app/asset.txt = %d, want 200", resp.StatusCode)
	}

	// /_rebuild requires POST and same-origin.
	if resp := get(t, srv, "/app/_rebuild"); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /_rebuild = %d, want 405", resp.StatusCode)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/app/_rebuild", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	if resp, err := srv.Client().Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("cross-origin POST /_rebuild = %d, want 403", resp.StatusCode)
		}
	}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/app/_rebuild", nil)
	req.Header.Set("Origin", srv.URL)
	if resp, err := srv.Client().Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("same-origin POST /_rebuild = %d, want 204", resp.StatusCode)
		}
	}
	// Non-browser CLI POST without Origin/Referer is allowed.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/app/_rebuild", nil)
	if resp, err := srv.Client().Do(req); err != nil {
		t.Fatal(err)
	} else {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("CLI POST /_rebuild = %d, want 204", resp.StatusCode)
		}
	}
}
