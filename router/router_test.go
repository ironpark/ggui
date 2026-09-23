package router

import (
	"strings"
	"testing"

	"github.com/ironpark/ggui"
)

func text(s string) PageFunc {
	return func(*Context) ggui.Widget { return ggui.Text(s) }
}

func passLayout(c *Context, page ggui.Widget) ggui.Widget { return page }

func mustPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		t.Helper()
		v := recover()
		if v == nil {
			t.Fatalf("no panic; want %q", want)
		}
		if msg, _ := v.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v; want %q", v, want)
		}
	}()
	fn()
}

func TestValidation(t *testing.T) {
	mustPanic(t, "ambiguous routes /a/:x and /a/:y", func() {
		New(Page("/a/:x", text("")), Layout("", passLayout, Page("/a/:y", text(""))))
	})
	mustPanic(t, "declared twice", func() {
		New(Group("/:id", Page("/b/:id", text(""))))
	})
	mustPanic(t, "must be the final segment", func() {
		New(Group("/*rest", Page("/", text(""))))
	})
	mustPanic(t, "two NotFound", func() {
		New(Group("/a", Layout("", passLayout, NotFound(text(""))), NotFound(text(""))))
	})
	mustPanic(t, "matches no page", func() { New(Redirect("/a", "/b")) })
	mustPanic(t, "redirect loop", func() { New(Redirect("/a", "/b"), Redirect("/b", "/a")) })
	mustPanic(t, "nil page", func() { New(Page("/", nil)) })
	mustPanic(t, "placed twice", func() {
		p := Page("/", text(""))
		g := Group("/a", p)
		New(g, Group("/b", g))
	})
	p := Page("/", text(""))
	New(p)
	mustPanic(t, "already belongs", func() { New(p) })
}

func TestMatchingPrecedence(t *testing.T) {
	r := New(
		Page("/users/new", text("new")),
		Page("/users/:id", text("user")),
		Page("/docs/*rest", text("docs")),
		Page("/docs/:slug/edit", text("edit")),
		Group("/a", NotFound(text("a-missing"))),
	)
	cases := map[string]string{
		"/users/new":        "/users/new",
		"/users/42":         "/users/:id",
		"/docs/x/y":         "/docs/*rest",
		"/docs/x/edit":      "/docs/:slug/edit",
		"/users/a%2Fb":      "not found /", // an encoded slash is not a parameter
		"/a/unknown/deeper": "not found /a",
	}
	for url, want := range cases {
		u, err := parseURL(url)
		if err != nil {
			t.Fatal(err)
		}
		leaf, _ := r.matchLeaf(u.decoded)
		got := ""
		if leaf != nil {
			got = patternString(leaf.pattern)
		} else if nf, _ := r.matchNotFound(u.decoded); nf != nil {
			got = "not found " + patternString(nf.pattern)
		} else {
			got = "not found /"
		}
		if got != want {
			t.Errorf("%s matched %s; want %s", url, got, want)
		}
	}
	for _, bad := range []string{"relative", "https://x.test/", "//host/a", "/a//b", "/a/%zz"} {
		if _, err := parseURL(bad); err == nil {
			t.Errorf("parseURL(%q) succeeded", bad)
		}
	}
}

func TestTargetURL(t *testing.T) {
	user := Page("/users/:id", text(""))
	doc := Page("/docs/*rest", text(""))
	loose := Page("/loose", text(""))
	New(Group("/org/:org", user), doc)

	if u, err := user.URL(Params{"org": "a b", "id": "42"}); err != nil || u != "/org/a%20b/users/42" {
		t.Fatalf("URL = %q, %v", u, err)
	}
	if u, err := doc.URL(Params{"rest": "guide/intro page"}); err != nil || u != "/docs/guide/intro%20page" {
		t.Fatalf("catch-all URL = %q, %v", u, err)
	}
	for _, p := range []Params{{"id": "1"}, {"org": "o", "id": "1", "x": "y"}, {"org": "o", "id": "a/b"}, {"org": "o", "id": ""}} {
		if _, err := user.URL(p); err == nil {
			t.Errorf("URL(%v) succeeded", p)
		}
	}
	if _, err := loose.URL(nil); err == nil {
		t.Error("unattached target built a URL")
	}
}

type harness struct {
	t     *testing.T
	r     *Router
	p     *ggui.Probe
	count map[string]int
}

// mount renders a small admin application and counts factory runs.
func mount(t *testing.T, h History) *harness {
	x := &harness{t: t, count: map[string]int{}}
	page := func(name string) PageFunc {
		return func(c *Context) ggui.Widget {
			x.count[name+":"+c.Param("id")]++
			return ggui.Text(name)
		}
	}
	layout := func(name string) LayoutFunc {
		return func(c *Context, outlet ggui.Widget) ggui.Widget {
			x.count[name+":"+c.Param("id")]++
			return ggui.Column(ggui.Text(name), outlet)
		}
	}
	x.r = New(
		Page("/login", page("login")),
		Layout("", layout("app"),
			Page("/", page("home")),
			Redirect("/home", "/"),
			Layout("/projects/:id", layout("project"),
				Page("/", page("overview")),
				Page("/tasks", page("tasks")),
			),
			Layout("/admin", layout("admin"),
				Page("/users", page("users")),
				NotFound(page("admin-missing")),
			),
			NotFound(page("missing")),
		),
	)
	if h != nil {
		x.r.History(h)
	}
	x.p = ggui.NewProbe(x.r.View(), ggui.Sz(400, 300))
	t.Cleanup(x.p.Close)
	x.p.Frame()
	return x
}

func (x *harness) go_(url string) {
	x.t.Helper()
	if err := x.r.Navigate(url); err != nil {
		x.t.Fatalf("Navigate(%s): %v", url, err)
	}
	x.p.Frame()
}

func (x *harness) want(counts map[string]int) {
	x.t.Helper()
	for k, v := range counts {
		if x.count[k] != v {
			x.t.Errorf("%s ran %d times; want %d (all: %v)", k, x.count[k], v, x.count)
		}
	}
}

func TestLayoutsSurviveAndParametersRemount(t *testing.T) {
	x := mount(t, nil)
	x.want(map[string]int{"app:": 1, "home:": 1})

	x.go_("/projects/1")
	x.go_("/projects/1/tasks")
	x.want(map[string]int{"app:": 1, "project:1": 1, "overview:1": 1, "tasks:1": 1})

	x.go_("/projects/2/tasks")
	x.want(map[string]int{"app:": 1, "project:2": 1, "tasks:2": 1})

	x.go_("/projects/2/tasks?filter=open")
	x.want(map[string]int{"project:2": 1, "tasks:2": 1})
	if got := x.r.Location().Query("filter"); got != "open" {
		t.Fatalf("query = %q", got)
	}

	x.go_("/login")
	x.go_("/")
	x.want(map[string]int{"app:": 2, "home:": 2, "login:": 1})
}

func TestNotFoundScopeAndRedirect(t *testing.T) {
	x := mount(t, nil)
	x.go_("/admin/nope")
	x.want(map[string]int{"admin:": 1, "admin-missing:": 1})
	x.go_("/admin/other")
	x.want(map[string]int{"admin-missing:": 1}) // same instance, new location
	x.go_("/elsewhere")
	x.want(map[string]int{"missing:": 1})
	if x.r.Current().Get() != nil {
		t.Fatal("Current should be nil on a not-found screen")
	}

	x.go_("/login")
	x.go_("/home")
	if got := x.r.Location().Path; got != "/" {
		t.Fatalf("redirect landed on %s", got)
	}
	x.r.Back()
	x.p.Frame()
	if got := x.r.Location().Path; got != "/login" {
		t.Fatalf("Back from a redirect landed on %s; want /login", got)
	}
}

func TestHistoryTraversalAndReaders(t *testing.T) {
	h := Memory("/admin/users")
	x := mount(t, h)
	users := x.r.Active("/admin/users")
	admin := x.r.Active("/admin")
	adm := x.r.Active("/adm")
	if !users.Get() || !admin.Get() || adm.Get() {
		t.Fatalf("Active: users=%v admin=%v adm=%v", users.Get(), admin.Get(), adm.Get())
	}
	if x.r.CanBack().Get() {
		t.Fatal("CanBack at the first entry")
	}
	x.go_("/projects/7")
	x.go_("/projects/7/tasks")
	x.r.Back()
	x.p.Frame()
	if got := x.r.Location().Path; got != "/projects/7" {
		t.Fatalf("Back landed on %s", got)
	}
	if !x.r.CanBack().Get() || !x.r.CanForward().Get() {
		t.Fatal("expected both directions")
	}
	x.want(map[string]int{"project:7": 1, "overview:7": 2})
	x.r.Forward()
	x.p.Frame()
	if got := x.r.Location().Path; got != "/projects/7/tasks" || x.r.CanForward().Get() {
		t.Fatalf("Forward landed on %s", got)
	}

	if err := x.r.Navigate("/projects/7/tasks"); err != nil {
		t.Fatal(err)
	}
	if _, n := h.Position(); n != 3 {
		t.Fatalf("same-URL navigation pushed: %d entries", n)
	}
}

func TestTargetsAndQueryTracking(t *testing.T) {
	var runs, filterRuns int
	var filter string
	list := Page("/tasks", func(c *Context) ggui.Widget {
		runs++
		return ggui.Reactive(func() ggui.Widget {
			filterRuns++
			filter = c.Query("filter")
			return ggui.Text(filter)
		})
	})
	other := Page("/other", text("other"))
	r := New(Layout("/p/:id", passLayout, list), other)
	r.History(Memory("/p/1/tasks"))
	p := ggui.NewProbe(r.View(), ggui.Sz(200, 100))
	defer p.Close()
	p.Frame()

	if !list.Active().Get() || other.Active().Get() || r.Current().Get() != list {
		t.Fatal("target activity")
	}
	if err := r.Navigate("/p/1/tasks?filter=open"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	if err := r.Navigate("/p/1/tasks?filter=open&page=2"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	if runs != 1 || filterRuns != 2 || filter != "open" {
		t.Fatalf("runs=%d filterRuns=%d filter=%q", runs, filterRuns, filter)
	}
	if err := other.Navigate(nil); err != nil || !other.Active().Get() {
		t.Fatalf("target navigation: %v", err)
	}
}

func TestSetupReplaceAndLoopReporting(t *testing.T) {
	var r *Router
	r = New(
		Page("/", text("home")),
		Page("/old", func(c *Context) ggui.Widget {
			if err := c.Replace("/"); err != nil {
				t.Errorf("setup Replace: %v", err)
			}
			return nil
		}),
		Page("/ping", text("ping")),
		Page("/pong", text("pong")),
	)
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()
	if err := r.Navigate("/old"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	p.Frame()
	if got := r.Location().Path; got != "/" {
		t.Fatalf("setup Replace landed on %s", got)
	}

	p.Frame()
	var err error
	for i := 0; i < 20 && err == nil; i++ {
		err = r.Navigate([]string{"/ping", "/pong"}[i%2])
	}
	if err != ErrRedirectLoop {
		t.Fatalf("chained navigation error = %v", err)
	}
	p.Frame()
	if err := r.Navigate("/"); err != nil {
		t.Fatalf("navigation after a frame: %v", err)
	}
}

func TestViewLifecycle(t *testing.T) {
	r := New(Page("/", text("home")))
	if err := r.Navigate("/"); err != ErrNotMounted {
		t.Fatalf("unmounted Navigate = %v", err)
	}
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	p.Frame()
	mustPanic(t, "after View", func() { r.History(Memory("/")) })
	p.Close()
	if err := r.Navigate("/"); err != ErrNotMounted {
		t.Fatalf("Navigate after disposal = %v", err)
	}
}

func TestBrowserBasePath(t *testing.T) {
	for in, want := range map[string]string{"": "", "/": "", "/app": "/app", "/app/": "/app", "/a%20b/c": "/a%20b/c"} {
		if got := cleanBase(in); got != want {
			t.Errorf("cleanBase(%q) = %q; want %q", in, got, want)
		}
	}
	for _, bad := range []string{"app", "/app?x", "/app#x", "//host", "/a//b"} {
		mustPanic(t, "must be a path", func() { cleanBase(bad) })
	}
	cases := []struct{ base, pathname, want string }{
		{"", "/settings", "/settings"},
		{"/app", "/app", "/"},
		{"/app", "/app/", "/"},
		{"/app", "/app/projects/1", "/projects/1"},
		{"/app", "/apple", "/apple"},
		{"/app", "/", "/"},
	}
	for _, c := range cases {
		if got := appURL(c.base, c.pathname); got != c.want {
			t.Errorf("appURL(%q, %q) = %q; want %q", c.base, c.pathname, got, c.want)
		}
		// Writing what was read shows the same page.
		if got := appURL(c.base, c.base+c.want); got != c.want {
			t.Errorf("round trip of %q under %q gave %q", c.want, c.base, got)
		}
	}
}
