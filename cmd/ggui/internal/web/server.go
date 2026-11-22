package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Server serves the app to a browser during development. It keeps the
// latest build, success or failure, and serves every request from it;
// Rebuild replaces it, and a page that is open hears about the new build
// over server-sent events and reloads itself onto it.
//
// The page is also served for any other URL that asks for HTML and names
// no file in the package's directory, so an app using the router's
// Browser history can be reloaded on a route; Base mounts everything below
// a path, as a host serving the app from a subdirectory would.
type Server struct {
	build BuildOptions // Out is set per build
	// pkgDir is the package's directory, whose files are served as they
	// are, so a package can fetch an asset at run time.
	pkgDir string
	base   string
	index  []byte
	target string
	logf   func(string, ...any)

	ctx    context.Context
	tmp    string
	mu     sync.Mutex
	cur    *build
	seq    int
	subs   map[chan event]struct{}
	cancel context.CancelFunc // the current build's
	closed bool
	wg     sync.WaitGroup // active build goroutines
}

// build is one compilation: in flight until done is closed, then either a
// result or the error that came out of the toolchain.
type build struct {
	id   string
	done chan struct{}
	res  *Result
	err  error
}

func (b *build) ready() bool {
	select {
	case <-b.done:
		return true
	default:
		return false
	}
}

// event is what the page is told about a build.
type event struct {
	ID    string `json:"id"`
	State string `json:"state"` // building, ready or error
	Error string `json:"error,omitempty"`
}

func (b *build) event() event {
	switch {
	case !b.ready():
		return event{ID: b.id, State: "building"}
	case b.err != nil:
		return event{ID: b.id, State: "error", Error: b.err.Error()}
	default:
		return event{ID: b.id, State: "ready"}
	}
}

// ServerOptions configures a Server.
type ServerOptions struct {
	Build  BuildOptions // Out is ignored: builds go to a directory of the server's own
	PkgDir string
	Base   string
	Title  string
	Target string
	// Live has the page follow rebuilds. Without it the page loads once,
	// as the serve tool's did, and a reload after a failed build retries.
	Live bool
	Logf func(string, ...any)
}

// NewServer returns a server whose builds live until ctx is done and
// Close is called. It does not build until asked.
func NewServer(ctx context.Context, o ServerOptions) (*Server, error) {
	tmp, err := os.MkdirTemp("", "ggui-dev-web-")
	if err != nil {
		return nil, err
	}
	logf := o.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	s := &Server{
		build:  o.Build,
		pkgDir: o.PkgDir,
		base:   CleanBase(o.Base),
		target: o.Target,
		logf:   logf,
		ctx:    ctx,
		tmp:    tmp,
		subs:   map[chan event]struct{}{},
	}
	s.index = Page(PageOptions{Title: o.Title, Base: s.base, Dev: true, Target: o.Target})
	if !o.Live {
		// The page only follows builds over _events; without them there
		// is nothing to follow, and the endpoint answers that.
		s.subs = nil
	}
	return s, nil
}

// Close cancels any in-flight build, waits for it to exit, and removes the builds.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return os.RemoveAll(s.tmp)
}

// Base returns the path the app is served below.
func (s *Server) Base() string { return s.base }

// Rebuild starts a build of the code as it is now. A build still in flight
// is cancelled, since it is building code that has since changed.
func (s *Server) Rebuild() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startLocked()
}

func (s *Server) startLocked() *build {
	if s.closed {
		if s.cur != nil {
			return s.cur
		}
		done := make(chan struct{})
		close(done)
		return &build{done: done, err: context.Canceled}
	}
	if s.cancel != nil {
		s.cancel()
	}
	s.seq++
	b := &build{
		id:   strconv.FormatInt(time.Now().UnixNano(), 36),
		done: make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.cur, s.cancel = b, cancel
	dir := filepath.Join(s.tmp, strconv.Itoa(s.seq))
	prev := filepath.Join(s.tmp, strconv.Itoa(s.seq-2))
	s.broadcastLocked(b.event())
	s.wg.Go(func() {
		defer cancel()
		o := s.build
		o.Out = filepath.Join(dir, WasmName)
		o.Logf = s.logf
		// The go command's work directory, which a cancelled build, killed,
		// leaves behind.
		o.TmpDir = filepath.Join(dir, "work")
		if err := os.MkdirAll(o.TmpDir, 0o755); err != nil {
			b.err = err
		} else {
			b.res, b.err = Build(ctx, o)
			os.RemoveAll(o.TmpDir)
		}
		close(b.done)

		s.mu.Lock()
		defer s.mu.Unlock()
		if s.cur != b {
			return // superseded; its successor reports instead
		}
		if b.err != nil {
			s.logf("build failed:\n%v", b.err)
		} else {
			s.logf("built %s in %s (%s, %s gzipped)", s.target, b.res.Elapsed.Round(time.Millisecond), MB(b.res.Size), MB(b.res.GzSize))
			// The build before the last may still be downloading, but
			// not the one before that.
			os.RemoveAll(prev)
		}
		s.broadcastLocked(b.event())
	})
	return b
}

// current returns the current build, starting one if there is none.
func (s *Server) current() *build {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return s.startLocked()
	}
	return s.cur
}

// settled waits for the current build to finish, following it to its
// successor if it is replaced while being waited for.
func (s *Server) settled(ctx context.Context) (*build, error) {
	for {
		b := s.current()
		select {
		case <-b.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		s.mu.Lock()
		same := s.cur == b
		s.mu.Unlock()
		if same {
			return b, nil
		}
	}
}

// retryIfFailed starts another build when the current one failed. Only a
// page load calls it: were every status poll to retry, a build that cannot
// succeed would start the compiler four times a second.
func (s *Server) retryIfFailed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.cur.ready() && s.cur.err != nil {
		s.startLocked()
	}
}

func (s *Server) broadcastLocked(e event) {
	for ch := range s.subs {
		select {
		case ch <- e:
		default:
			// A page that has stopped reading misses this event, not
			// the next: the channel holds the latest one waiting.
			select {
			case <-ch:
			default:
			}
			ch <- e
		}
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path+"/" == s.base {
		http.Redirect(w, r, s.base, http.StatusMovedPermanently)
		return
	}
	rest, ok := strings.CutPrefix(r.URL.Path, s.base)
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch rest {
	case "", "index.html":
		s.serveIndex(w)
	case WasmName:
		s.serveWasm(w, r)
	case "wasm_exec.js":
		s.serveWasmExec(w, r)
	case "_status":
		s.serveStatus(w)
	case "_events":
		s.serveEvents(w, r)
	case "_rebuild":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !sameOrigin(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		s.Rebuild()
		w.WriteHeader(http.StatusNoContent)
	default:
		// Anything else comes from the package's own directory, so it can
		// ship an asset it fetches at run time. A page request naming no
		// file is a route the app shows itself.
		clean := pathClean(rest)
		if blockedStaticPath(clean) {
			http.NotFound(w, r)
			return
		}
		file := filepath.Join(s.pkgDir, filepath.FromSlash(clean))
		if _, err := os.Stat(file); errors.Is(err, fs.ErrNotExist) {
			// A dangling symlink is not a route.
			if _, lerr := os.Lstat(file); lerr == nil {
				http.NotFound(w, r)
				return
			}
			if wantsPage(r) {
				s.serveIndex(w)
				return
			}
			http.NotFound(w, r)
			return
		}
		if !s.containedInPkgDir(file) {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, file)
	}
}

// sameOrigin rejects cross-origin browser requests while permitting same-origin
// requests and non-browser CLI requests without Origin/Referer headers.
func sameOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		if origin == "null" {
			return false
		}
		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	if ref := r.Header.Get("Referer"); ref != "" {
		u, err := url.Parse(ref)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
	if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" && sfs != "none" {
		return false
	}
	return true
}

// blockedStaticPath reports whether clean (slash-separated, relative) refers
// to a Go source file or a dotfile/dotdir such as .env or .git.
func blockedStaticPath(clean string) bool {
	if clean == "" {
		return false
	}
	if strings.EqualFold(filepath.Ext(clean), ".go") {
		return true
	}
	for part := range strings.SplitSeq(clean, "/") {
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

// containedInPkgDir verifies that file, after resolving symlinks, remains
// inside s.pkgDir and does not resolve to a blocked path.
func (s *Server) containedInPkgDir(file string) bool {
	root, err := filepath.EvalSymlinks(s.pkgDir)
	if err != nil {
		return false
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return false
	}
	resolved, err := filepath.EvalSymlinks(file)
	if err != nil {
		return false
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	if rel != "." && blockedStaticPath(filepath.ToSlash(rel)) {
		return false
	}
	return true
}

// pathClean cleans a URL path below the base so it cannot climb out of
// the package directory.
func pathClean(p string) string {
	return strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+p)), "/")
}

// wantsPage reports whether r is a navigation, as opposed to a fetch for
// an asset that happens to be missing.
func wantsPage(r *http.Request) bool {
	return (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
		strings.Contains(r.Header.Get("Accept"), "text/html")
}

func (s *Server) serveIndex(w http.ResponseWriter) {
	s.retryIfFailed() // a reload is how you ask for another go at it
	s.current()       // start compiling while the page loads
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(s.index)
}

func (s *Server) serveWasm(w http.ResponseWriter, r *http.Request) {
	b, err := s.settled(r.Context())
	if err != nil {
		return // the client went away
	}
	if b.err != nil {
		http.Error(w, b.err.Error(), http.StatusInternalServerError)
		return
	}
	// A client that takes gzip gets the copy compressed at build time. The
	// browser decodes it before instantiate sees the bytes, so the page
	// needs to know nothing about this; Vary keeps any cache in between
	// from handing the encoded bytes to a client that cannot read them.
	path, etag := b.res.Wasm, b.id
	w.Header().Set("Vary", "Accept-Encoding")
	if b.res.Gzip != "" && acceptsGzip(r) {
		path, etag = b.res.Gzip, b.id+"-gz"
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
	// out of the browser's cache instead of over the wire, but only after
	// asking: without a validator to check, a browser may guess the binary
	// is still fresh and run the one from before a rebuild or a restart.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", strconv.Quote(etag))
	http.ServeContent(w, r, WasmName, fi.ModTime(), f)
}

// acceptsGzip reports whether the client asked for gzip and did not then
// refuse it with q=0.
func acceptsGzip(r *http.Request) bool {
	for enc := range strings.SplitSeq(r.Header.Get("Accept-Encoding"), ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(enc), ";")
		if name != "gzip" {
			continue
		}
		return strings.ReplaceAll(strings.TrimSpace(params), " ", "") != "q=0"
	}
	return false
}

// serveStatus answers what the build is doing, for the loading screen.
func (s *Server) serveStatus(w http.ResponseWriter) {
	b := s.current()
	// Size is the binary as the page will see it, decoded, which is what its
	// progress bar counts; Compressed is only for reporting.
	status := struct {
		ID         string `json:"id"`
		Target     string `json:"target"`
		State      string `json:"state"`
		Error      string `json:"error,omitempty"`
		Size       int64  `json:"size,omitempty"`
		Compressed int64  `json:"compressed,omitempty"`
		Elapsed    string `json:"elapsed,omitempty"`
	}{ID: b.id, Target: s.target, State: "building"}
	if b.ready() {
		switch {
		case b.err != nil:
			status.State, status.Error = "error", b.err.Error()
		default:
			status.State, status.Size = "ready", b.res.Size
			status.Compressed = b.res.GzSize
			status.Elapsed = b.res.Elapsed.Round(time.Millisecond).String()
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(status)
}

// serveEvents streams build events to the page: the current build's state
// at once, then each change.
func (s *Server) serveEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ch := make(chan event, 1)
	s.mu.Lock()
	if s.subs == nil {
		s.mu.Unlock()
		// 204 tells EventSource to stop reconnecting.
		w.WriteHeader(http.StatusNoContent)
		return
	}
	b := s.cur
	if b == nil {
		b = s.startLocked()
	}
	ch <- b.event()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	// A comment now and then keeps a proxy in between from closing a
	// stream it thinks idle.
	keepalive := time.NewTicker(20 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.ctx.Done():
			return
		case e := <-ch:
			data, _ := json.Marshal(e)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// serveWasmExec hands over the toolchain's JavaScript loader.
func (s *Server) serveWasmExec(w http.ResponseWriter, r *http.Request) {
	p, err := WasmExec(r.Context(), s.build.Dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Revalidated, so a toolchain upgrade does not pair a new binary with
	// the loader the browser kept from the old one.
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, p)
}
