package router

import (
	"errors"
	"net/url"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/reactive"
)

// Context is the route-scoped handle a page, layout or not-found factory
// receives. It is valid only while its route instance is mounted.
type Context struct {
	r       *Router
	inst    instance
	owner   *reactive.Computation
	queries map[string]*ggui.DerivedValue[string]
}

// Param returns a path parameter matched through this route node, or "".
// It is constant for the Context's lifetime: a parameter change remounts
// the instance with a new Context.
func (c *Context) Param(name string) string { return c.inst.params[name] }

// Query returns the first value of a query parameter, or "". It is a
// tracked read that notifies only when this name's value changes.
func (c *Context) Query(name string) string {
	d := c.queries[name]
	if d == nil {
		reactive.WithOwner(c.owner, func() {
			d = c.r.loc.Map(func(l Location) string { return l.Query(name) })
		})
		if c.queries == nil {
			c.queries = map[string]*ggui.DerivedValue[string]{}
		}
		c.queries[name] = d
	}
	return d.Get()
}

// Location reads the current location; it is a tracked read.
func (c *Context) Location() Location { return c.r.loc.Get() }

// Navigate is Router.Navigate.
func (c *Context) Navigate(url string) error { return c.r.Navigate(url) }

// Replace is Router.Replace.
func (c *Context) Replace(url string) error { return c.r.Replace(url) }

// Back is Router.Back.
func (c *Context) Back() { c.r.Back() }

// Router returns the router that mounted this route.
func (c *Context) Router() *Router { return c.r }

// Target is the node Page returns: part of the route tree and a typed
// navigation handle. Its full path comes from where it is placed.
type Target struct {
	path  string
	build PageFunc
	r     *Router
	rt    *route
}

var errUnattached = errors.New("router: target has not been passed to New")

// URL fills the target's effective path with params. Pass nil for a page
// without parameters. Every parameter must be supplied.
func (t *Target) URL(params Params) (string, error) {
	if t.rt == nil {
		return "", errUnattached
	}
	return buildURL(t.rt.pattern, params)
}

// Navigate pushes the target's URL.
func (t *Target) Navigate(params Params) error {
	u, err := t.URL(params)
	if err != nil {
		return err
	}
	return t.r.Navigate(u)
}

// Active reports whether this target's page is the matched leaf, for any
// parameter values and query.
func (t *Target) Active() ggui.Readable[bool] {
	return reader[bool](func() bool { return t.rt != nil && t.r.leaf.Get() == t.rt })
}

// Location is an immutable snapshot of an application URL.
type Location struct {
	Path     string // normalized, escaped
	RawQuery string
	Fragment string
	Entry    uint64 // history entry ID
}

// Query returns the first value of name, or "".
func (l Location) Query(name string) string {
	v, _ := url.ParseQuery(l.RawQuery)
	return v.Get(name)
}

// HasQuery reports whether name is present, even with an empty value.
func (l Location) HasQuery(name string) bool {
	v, _ := url.ParseQuery(l.RawQuery)
	return v.Has(name)
}

// QueryAll returns a copy of every value of name.
func (l Location) QueryAll(name string) []string {
	v, _ := url.ParseQuery(l.RawQuery)
	return append([]string(nil), v[name]...)
}

// String returns the application-absolute URL.
func (l Location) String() string {
	s := l.Path
	if l.RawQuery != "" {
		s += "?" + l.RawQuery
	}
	if l.Fragment != "" {
		s += "#" + (&url.URL{Fragment: l.Fragment}).EscapedFragment()
	}
	return s
}
