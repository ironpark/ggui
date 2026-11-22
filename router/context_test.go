package router

import (
	"slices"
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/widget"
)

func TestContextForwardsToItsRouter(t *testing.T) {
	t.Parallel()
	var ctx *Context
	r := New(
		Page("/", text("home")),
		Page("/items/:id", func(c *Context) ggui.Widget {
			ctx = c
			return widget.Text(c.Param("id"))
		}),
		Page("/next", text("next")),
	)
	r.History(Memory("/items/7?sort=asc"))
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	if ctx == nil {
		t.Fatal("the item page was not built for its initial URL")
	}
	if ctx.Router() != r {
		t.Error("Context.Router is not the router that mounted the page")
	}
	if got := ctx.Location(); got.Path != "/items/7" || got.RawQuery != "sort=asc" {
		t.Errorf("Context.Location = %+v, want /items/7?sort=asc", got)
	}
	if got := ctx.Param("missing"); got != "" {
		t.Errorf("Param of an undeclared name = %q, want empty", got)
	}
	if err := ctx.Navigate("/next"); err != nil {
		t.Fatalf("Context.Navigate: %v", err)
	}
	p.Frame()
	if got := r.Location().Path; got != "/next" {
		t.Fatalf("Context.Navigate landed on %s, want /next", got)
	}
	ctx.Back()
	p.Frame()
	if got := r.Location().String(); got != "/items/7?sort=asc" {
		t.Errorf("Context.Back landed on %s, want /items/7?sort=asc", got)
	}
}

func TestLocationQueryHelpers(t *testing.T) {
	t.Parallel()
	l := Location{Path: "/search", RawQuery: "q=go&tag=a&tag=b&flag&empty="}
	cases := []struct {
		name  string
		first string
		has   bool
		all   []string
	}{
		{"q", "go", true, []string{"go"}},
		{"tag", "a", true, []string{"a", "b"}},
		{"flag", "", true, []string{""}},
		{"empty", "", true, []string{""}},
		{"absent", "", false, nil},
	}
	for _, c := range cases {
		if got := l.Query(c.name); got != c.first {
			t.Errorf("Query(%q) = %q, want %q", c.name, got, c.first)
		}
		if got := l.HasQuery(c.name); got != c.has {
			t.Errorf("HasQuery(%q) = %v, want %v", c.name, got, c.has)
		}
		if got := l.QueryAll(c.name); !slices.Equal(got, c.all) {
			t.Errorf("QueryAll(%q) = %q, want %q", c.name, got, c.all)
		}
	}
	all := l.QueryAll("tag")
	all[0] = "changed"
	if l.Query("tag") != "a" {
		t.Error("QueryAll returned storage shared with the location")
	}
}

func TestLocationStringEscapesTheFragment(t *testing.T) {
	t.Parallel()
	cases := []struct {
		l    Location
		want string
	}{
		{Location{Path: "/"}, "/"},
		{Location{Path: "/a", RawQuery: "x=1"}, "/a?x=1"},
		{Location{Path: "/a", Fragment: "part two"}, "/a#part%20two"},
		{Location{Path: "/a", RawQuery: "x=1", Fragment: "top"}, "/a?x=1#top"},
	}
	for _, c := range cases {
		if got := c.l.String(); got != c.want {
			t.Errorf("%+v.String() = %q, want %q", c.l, got, c.want)
		}
	}
}

func TestDisposedContextQueryAndLocationCache(t *testing.T) {
	t.Parallel()
	var oldCtx *Context
	r := New(
		Page("/", func(c *Context) ggui.Widget {
			oldCtx = c
			_ = c.Query("q")
			return widget.Text("home")
		}),
		Page("/other", text("other")),
	)
	r.History(Memory("/?q=one&tag=x"))
	p := ggui.NewProbe(r.View(), ggui.Sz(100, 100))
	defer p.Close()
	p.Frame()

	// Location caches parsed query values and remains comparable.
	loc := r.Location()
	if loc.parsed == nil || loc.Query("q") != "one" || !loc.HasQuery("tag") {
		t.Fatalf("expected cached parsed query on Location: %+v", loc)
	}
	if loc != r.Location() {
		t.Fatal("identical Location values did not compare equal")
	}
	mut := loc
	mut.RawQuery = "q=two"
	if got := mut.Query("q"); got != "two" {
		t.Fatalf("mutated RawQuery returned stale cache %q, want two", got)
	}

	if err := r.Navigate("/other?q=after"); err != nil {
		t.Fatal(err)
	}
	p.Frame()
	if !oldCtx.disposed || oldCtx.queries != nil {
		t.Fatal("departed Context was not marked disposed and cleared")
	}
	if got := oldCtx.Query("q"); got != "after" {
		t.Fatalf("disposed Context.Query = %q, want after", got)
	}
	if got := oldCtx.QueryValue("q").Get(); got != "after" {
		t.Fatalf("disposed Context.QueryValue = %q, want after", got)
	}
	if oldCtx.queries != nil {
		t.Fatal("disposed Context.query attached a derived computation to a dead owner")
	}
}
