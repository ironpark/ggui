package router

import (
	"context"
	"testing"
	"time"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/ui"
)

func TestLeavingAPageCancelsItsResource(t *testing.T) {
	started := make(chan context.Context, 4)
	release := make(chan struct{})
	r := New(
		Page("/", text("home")),
		Page("/load/:id", func(c *Context) ggui.Widget {
			res := ggui.Resource(func() string { return c.Param("id") }, func(ctx context.Context, id string) (string, error) {
				started <- ctx
				select {
				case <-release:
				case <-ctx.Done():
				}
				return id, ctx.Err()
			})
			return ggui.Await(res).Then(func(v ggui.Readable[string]) ggui.Widget { return ggui.TextOf(v) })
		}),
	)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	if err := r.Navigate("/load/1"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	first := receive(t, started)
	if err := r.Navigate("/load/2"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	second := receive(t, started)
	waitDone(t, first, "a parameter change")
	if err := r.Navigate("/"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	waitDone(t, second, "leaving the page")
	close(release)
}

func TestRapidNavigationBuildsOnlyTheLastPage(t *testing.T) {
	built := map[string]int{}
	page := func(name string) PageFunc {
		return func(*Context) ggui.Widget { built[name]++; return ggui.Text(name) }
	}
	r := New(Page("/", page("home")), Page("/a", page("a")), Page("/b", page("b")), Page("/c", page("c")))
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	for _, u := range []string{"/a", "/b", "/c"} {
		if err := r.Navigate(u); err != nil {
			t.Fatal(err)
		}
	}
	p.Frame()
	if built["a"] != 0 || built["b"] != 0 || built["c"] != 1 {
		t.Fatalf("built %v; only the final page should mount", built)
	}
}

func TestDisposingViewCleansUpOnce(t *testing.T) {
	cleanups := map[string]int{}
	track := func(name string) func() {
		return func() { cleanups[name]++ }
	}
	r := New(Layout("", func(c *Context, page ggui.Widget) ggui.Widget {
		ggui.OnCleanup(track("layout"))
		return page
	}, Page("/", func(*Context) ggui.Widget {
		ggui.OnCleanup(track("home"))
		return ggui.Text("home")
	}), Page("/other", func(*Context) ggui.Widget {
		ggui.OnCleanup(track("other"))
		return ggui.Text("other")
	})))
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	p.Frame()
	if err := r.Navigate("/other"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	if cleanups["home"] != 1 || cleanups["layout"] != 0 {
		t.Fatalf("after navigation: %v", cleanups)
	}
	p.Close()
	if cleanups["home"] != 1 || cleanups["other"] != 1 || cleanups["layout"] != 1 {
		t.Fatalf("after disposal: %v", cleanups)
	}
}

func TestDepartedFocusClearsAndLayoutFocusSurvives(t *testing.T) {
	r := New(Layout("", func(c *Context, page ggui.Widget) ggui.Widget {
		return ggui.Column(ui.Button("chrome", func() {}), page)
	}, Page("/", func(*Context) ggui.Widget { return ui.Button("home", func() {}) }),
		Page("/other", func(*Context) ggui.Widget { return ui.Button("other", func() {}) })))
	p := ggui.NewProbe(r.View(), ggui.Sz(200, 200))
	defer p.Close()
	p.Frame()

	p.Tap("home")
	if !p.Focused() {
		t.Fatal("tapping home did not focus it")
	}
	if err := r.Navigate("/other"); err != nil {
		t.Fatal(err)
	}
	// Focus is revalidated when input is dispatched, not on a bare Frame.
	p.Move(ggui.Pt(150, 190))
	if p.Focused() {
		t.Fatal("focus moved to the new page's control in the departed one's place")
	}

	p.Tap("chrome")
	if err := r.Navigate("/"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	p.Frame()
	if !p.Focused() {
		t.Fatal("layout focus lost across navigation")
	}
}

func receive(t *testing.T, ch <-chan context.Context) context.Context {
	t.Helper()
	select {
	case ctx := <-ch:
		return ctx
	case <-time.After(5 * time.Second):
		t.Fatal("resource did not start")
		return nil
	}
}

func waitDone(t *testing.T, ctx context.Context, why string) {
	t.Helper()
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatalf("resource not cancelled after %s", why)
	}
}
