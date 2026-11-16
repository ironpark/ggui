package dev

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/watch"
	"github.com/ironpark/ggui/cmd/ggui/internal/web"
)

// Web serves an app under development to a browser. The browser is the
// runner here: the server builds, and the page reloads itself onto each
// new build.
type Web struct {
	Dir     string // where the go command runs
	Package string
	PkgDir  string // the package's directory, whose files are served too
	Target  string // shown on the loading page
	Tags    []string
	LDFlags string
	WasmOpt string
	Title   string
	Base    string
	Addr    string // host:port to listen on
	Extra   []string
	Watch   bool
	Log     io.Writer
}

func (w *Web) logf(format string, args ...any) {
	fmt.Fprintf(w.Log, "ggui: "+format+"\n", args...)
}

// Run serves until ctx is done.
func (w *Web) Run(ctx context.Context) error {
	srv, err := web.NewServer(ctx, web.ServerOptions{
		Build: web.BuildOptions{
			Dir: w.Dir, Package: w.Package, Tags: w.Tags,
			LDFlags: w.LDFlags, WasmOpt: w.WasmOpt,
		},
		PkgDir: w.PkgDir,
		Base:   w.Base,
		Title:  w.Title,
		Target: w.Target,
		Live:   w.Watch,
		Logf:   w.logf,
	})
	if err != nil {
		return err
	}
	defer srv.Close()

	ln, err := net.Listen("tcp", w.Addr)
	if err != nil {
		return fmt.Errorf("%w; is another server on that port? Choose another with --port", err)
	}
	// Compile now, so the first page load usually finds it done.
	srv.Rebuild()
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	served := make(chan error, 1)
	go func() { served <- hs.Serve(ln) }()
	w.logf("serving %s at %s", w.Target, url(ln.Addr(), w.Addr, srv.Base()))

	if w.Watch {
		lo := gotool.ListOptions{Dir: w.Dir, Tags: w.Tags, GOOS: "js", GOARCH: "wasm"}
		set, err := WatchSet(ctx, lo, w.Package, w.Extra)
		if err != nil {
			hs.Close()
			return err
		}
		wt := watch.New(set)
		changes := make(chan []string)
		go wt.Run(ctx, changes)
		w.logf("watching %d files; the page reloads itself after each build", wt.Len())
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case batch := <-changes:
					w.logf("%s changed, rebuilding", describe(w.Dir, batch))
					srv.Rebuild()
					if set, err := WatchSet(ctx, lo, w.Package, w.Extra); err == nil {
						wt.Set(set)
					}
				}
			}
		}()
	} else {
		w.logf("not watching: request %s_rebuild, or reload the page after a failed build, to build again", srv.Base())
	}

	select {
	case <-ctx.Done():
		hs.Close()
		return nil
	case err := <-served:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// url is the address to open, with localhost for an unspecified host.
func url(addr net.Addr, asked, base string) string {
	host := addr.String()
	if h, port, err := net.SplitHostPort(host); err == nil {
		if ip := net.ParseIP(h); ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
			host = net.JoinHostPort("localhost", port)
		}
	}
	if strings.HasPrefix(asked, ":") || strings.HasPrefix(asked, "0.0.0.0") {
		return "http://" + host + base + " (and on this machine's other addresses)"
	}
	return "http://" + host + base
}
