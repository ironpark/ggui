package router

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ironpark/ggui"
)

// PageFunc builds a page or not-found screen once per mount.
type PageFunc func(*Context) ggui.Widget

// LayoutFunc builds persistent chrome around page, the layout's outlet.
// Place page exactly once in the returned tree.
type LayoutFunc func(c *Context, page ggui.Widget) ggui.Widget

// Params supplies path parameter values by name.
type Params map[string]string

// Route is a node of a route tree. Only this package implements it.
type Route interface{ route() }

type layoutNode struct {
	path     string
	build    LayoutFunc
	children []Route
}

type groupNode struct {
	path     string
	children []Route
}

type notFoundNode struct{ build PageFunc }

type redirectNode struct{ path, to string }

func (*layoutNode) route()   {}
func (*groupNode) route()    {}
func (*notFoundNode) route() {}
func (*redirectNode) route() {}
func (*Target) route()       {}

// Page shows build at path, relative to the enclosing section. "/" is the
// section's own URL. The returned target is the tree node itself.
func Page(path string, build PageFunc) *Target { return &Target{path: path, build: build} }

// Layout places children under path and renders them inside build. An
// empty path adds no URL segment.
func Layout(path string, build LayoutFunc, children ...Route) Route {
	return &layoutNode{path, build, children}
}

// Group places children under path without adding UI.
func Group(path string, children ...Route) Route { return &groupNode{path, children} }

// NotFound shows build for unknown URLs inside the enclosing section.
func NotFound(build PageFunc) Route { return &notFoundNode{build} }

// Redirect replaces a visit to path with the literal application-absolute
// URL to before anything mounts.
func Redirect(path, to string) Route { return &redirectNode{path, to} }

type routeKind uint8

const (
	kindLayout routeKind = iota
	kindPage
	kindNotFound
	kindRedirect
)

// route is one compiled node that can appear in a mounted branch.
type route struct {
	id      int
	kind    routeKind
	pattern []seg    // full effective pattern; for NotFound, its section
	layouts []*route // enclosing layouts, outermost first
	layout  LayoutFunc
	page    PageFunc
	target  *Target
	to      parsedURL // redirect destination
}

type compiler struct {
	r         *Router
	seen      map[any]bool
	leaves    map[string]*route // page and redirect signatures
	notFounds map[string]*route // section signatures
	count     int
}

func (c *compiler) fail(format string, args ...any) {
	panic(fmt.Sprintf("router: "+format, args...))
}

func (c *compiler) add(r *route) *route {
	r.id = c.count
	c.count++
	return r
}

func (c *compiler) mark(n Route) {
	if c.seen[n] {
		c.fail("a route node is placed twice in the tree")
	}
	c.seen[n] = true
}

// join appends a registration path to a section pattern and checks the
// section rules: unique parameter names and a catch-all only in a leaf.
func (c *compiler) join(section []seg, path string, leaf bool) []seg {
	own, err := parsePattern(path)
	if err != nil {
		c.fail("%v", err)
	}
	out := append(append([]seg(nil), section...), own...)
	for i, s := range out {
		if s.kind == segStatic {
			continue
		}
		if s.kind == segCatchAll && (!leaf || i != len(out)-1) {
			c.fail("catch-all *%s must be the final segment of a page or redirect in %s", s.text, patternString(out))
		}
		for _, t := range out[:i] {
			if t.kind != segStatic && t.text == s.text {
				c.fail("parameter %q declared twice in %s", s.text, patternString(out))
			}
		}
	}
	return out
}

func (c *compiler) walk(nodes []Route, section []seg, layouts []*route) {
	for _, n := range nodes {
		if n == nil {
			c.fail("nil route in %s", patternString(section))
		}
		c.mark(n)
		switch n := n.(type) {
		case *layoutNode:
			if n.build == nil {
				c.fail("nil layout at %s", patternString(section))
			}
			p := c.join(section, n.path, false)
			l := c.add(&route{kind: kindLayout, pattern: p, layouts: layouts, layout: n.build})
			c.walk(n.children, p, append(layouts[:len(layouts):len(layouts)], l))
		case *groupNode:
			c.walk(n.children, c.join(section, n.path, false), layouts)
		case *Target:
			if n.build == nil {
				c.fail("nil page at %s", n.path)
			}
			if n.r != nil {
				c.fail("page %s already belongs to a router", n.path)
			}
			p := c.join(section, n.path, true)
			n.r = c.r
			n.rt = c.leaf(&route{kind: kindPage, pattern: p, layouts: layouts, page: n.build, target: n})
		case *redirectNode:
			p := c.join(section, n.path, true)
			to, err := parseURL(n.to)
			if err != nil {
				c.fail("redirect %s → %q: %v", patternString(p), n.to, err)
			}
			c.leaf(&route{kind: kindRedirect, pattern: p, layouts: layouts, to: to})
		case *notFoundNode:
			if n.build == nil {
				c.fail("nil not-found page at %s", patternString(section))
			}
			sig := signature(section)
			if c.notFounds[sig] != nil {
				c.fail("two NotFound nodes for section %s", patternString(section))
			}
			c.notFounds[sig] = c.add(&route{kind: kindNotFound, pattern: section, layouts: layouts, page: n.build})
			c.r.notFounds = append(c.r.notFounds, c.notFounds[sig])
		default:
			c.fail("unknown route node %T", n)
		}
	}
}

func (c *compiler) leaf(r *route) *route {
	sig := signature(r.pattern)
	if prev := c.leaves[sig]; prev != nil {
		c.fail("ambiguous routes %s and %s", patternString(prev.pattern), patternString(r.pattern))
	}
	c.leaves[sig] = r
	c.r.leaves = append(c.r.leaves, r)
	return c.add(r)
}

// compile validates the tree and fills r's route tables.
func compile(r *Router, routes []Route) {
	c := &compiler{r: r, seen: map[any]bool{}, leaves: map[string]*route{}, notFounds: map[string]*route{}}
	c.walk(routes, nil, nil)
	// The screen used when no NotFound applies.
	r.builtin = c.add(&route{kind: kindNotFound, page: func(*Context) ggui.Widget { return ggui.Text("Not found") }})
	for _, rt := range r.leaves {
		if rt.kind != kindRedirect {
			continue
		}
		visited := map[*route]bool{rt: true}
		cur := rt
		for cur.kind == kindRedirect {
			next, _ := r.matchLeaf(cur.to.decoded)
			if next == nil {
				c.fail("redirect %s → %q matches no page", patternString(cur.pattern), cur.to.location())
			}
			if visited[next] {
				c.fail("redirect loop through %s", patternString(rt.pattern))
			}
			visited[next] = true
			cur = next
		}
	}
}

// matchLeaf finds the most specific page or redirect for path.
func (r *Router) matchLeaf(path []string) (*route, map[string]string) {
	return bestMatch(r.leaves, path, false)
}

// matchNotFound finds the NotFound of the most specific section containing
// path, or nil for the built-in screen.
func (r *Router) matchNotFound(path []string) (*route, map[string]string) {
	return bestMatch(r.notFounds, path, true)
}

func bestMatch(routes []*route, path []string, prefix bool) (*route, map[string]string) {
	var best *route
	var bestParams map[string]string
	for _, rt := range routes {
		params, ok := matchSegs(rt.pattern, path, prefix)
		if ok && (best == nil || moreSpecific(rt.pattern, best.pattern)) {
			best, bestParams = rt, params
		}
	}
	return best, bestParams
}

// instance is one level of a matched branch: a layout, page or not-found
// screen with the parameters matched through it.
type instance struct {
	route  *route
	params map[string]string
	key    string
}

func newInstance(rt *route, all map[string]string) instance {
	var own map[string]string
	var b strings.Builder
	b.WriteString(strconv.Itoa(rt.id))
	for _, s := range rt.pattern {
		if s.kind == segStatic {
			continue
		}
		if own == nil {
			own = map[string]string{}
		}
		own[s.text] = all[s.text]
		b.WriteByte(0)
		b.WriteString(all[s.text])
	}
	return instance{rt, own, b.String()}
}

// branch is the chain of instances a location mounts, outermost first.
func branch(leaf *route, params map[string]string) []instance {
	out := make([]instance, 0, len(leaf.layouts)+1)
	for _, l := range leaf.layouts {
		out = append(out, newInstance(l, params))
	}
	return append(out, newInstance(leaf, params))
}
