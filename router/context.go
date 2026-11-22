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
	r        *Router
	inst     instance
	owner    *reactive.Computation
	disposed bool
	queries  map[string]*ggui.DerivedValue[string]
}

// Param returns a path parameter matched through this route node, or "".
// It is constant for the Context's lifetime: a parameter change remounts
// the instance with a new Context.
func (c *Context) Param(name string) string { return c.inst.params[name] }

// Query returns the first value of a query parameter, or "". It is a
// tracked read that notifies only when this name's value changes.
func (c *Context) Query(name string) string { return c.query(name).Get() }

// query is the derived value behind Query, made on first use while the
// Context is mounted. After disposal it falls back to a stateless reader so
// no derived computation is attached to a dead owner.
func (c *Context) query(name string) ggui.Readable[string] {
	if d := c.queries[name]; d != nil {
		return d
	}
	if c.disposed || c.owner == nil {
		return reader[string](func() string { return c.r.loc.Get().Query(name) })
	}
	var d *ggui.DerivedValue[string]
	reactive.WithOwner(c.owner, func() {
		d = c.r.loc.Map(func(l Location) string { return l.Query(name) })
	})
	if c.queries == nil {
		c.queries = map[string]*ggui.DerivedValue[string]{}
	}
	c.queries[name] = d
	return d
}

// Location reads the current location; it is a tracked read.
func (c *Context) Location() Location { return c.r.loc.Get() }

// QueryValue is Query as a value to bind, for a page that shows it: a page
// factory runs once, so what it reads with Query is never read again.
func (c *Context) QueryValue(name string) ggui.Readable[string] { return c.query(name) }

// LocationValue is Location as a value to bind; see QueryValue.
func (c *Context) LocationValue() ggui.Readable[Location] { return c.r.loc }

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

	parsed *parsedQuery // cached parse of RawQuery; pointer keeps Location comparable
}

type parsedQuery struct {
	raw    string
	values url.Values
}

func (l Location) queryValues() url.Values {
	if l.RawQuery == "" {
		return nil
	}
	if l.parsed != nil && l.parsed.raw == l.RawQuery {
		return l.parsed.values
	}
	v, _ := url.ParseQuery(l.RawQuery)
	return v
}

// Query returns the first value of name, or "".
func (l Location) Query(name string) string {
	return l.queryValues().Get(name)
}

// HasQuery reports whether name is present, even with an empty value.
func (l Location) HasQuery(name string) bool {
	return l.queryValues().Has(name)
}

// QueryAll returns a copy of every value of name.
func (l Location) QueryAll(name string) []string {
	v := l.queryValues()
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
