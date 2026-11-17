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
	OutDir  string // where ggui build writes, never watched
	Watch   bool
	Log     io.Writer
	// Tracef, when set, is told each decision the watcher and the loop
	// make.
	Tracef func(format string, args ...any)
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
		ig := watch.NewIgnore(w.OutDir)
		list := func(ctx context.Context) ([]string, error) {
			return WatchSet(ctx, lo, w.Package, w.Extra, ig)
		}
		set, err := list(ctx)
		if err != nil {
			hs.Close()
			return err
		}
		wt := watch.New(set, watch.Options{
			List: list, Ignore: ig,
			Settle: settleTime, Poll: pollInterval, SafetyNet: safetyNetInterval,
			Logf: w.logf, Tracef: w.Tracef,
		})
		changes := make(chan watch.Change)
		go wt.Run(ctx, changes)
		w.logf("watching %d files; the page reloads itself after each build", wt.Len())
		go w.loop(ctx, changes, srv)
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

// loop rebuilds on each real change, after the same wait for quiet as
// the desktop loop. The server cancels a build still running when the
// next starts, and the page loads only the build that finished last; the
// syntax is left to the build, whose errors the page shows.
func (w *Web) loop(ctx context.Context, changes <-chan watch.Change, srv *web.Server) {
	var p plan
	due := time.NewTimer(time.Hour)
	defer due.Stop()
	for {
		var dueC <-chan time.Time
		if t, ok := p.due(); ok {
			due.Reset(time.Until(t))
			dueC = due.C
		}
		select {
		case <-ctx.Done():
			return
		case c := <-changes:
			if len(c.Real) > 0 {
				p.change(c.At, c.Real)
			} else if len(c.Cosmetic) > 0 && p.idle() {
				w.logf("%s: comments only; not rebuilt — line numbers in traces now lag the source", describe(w.Dir, c.Cosmetic))
			}
		case <-dueC:
			id, batch, _ := p.start()
			p.done(id)
			w.logf("%s changed, rebuilding", describe(w.Dir, batch))
			srv.Rebuild()
		}
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
