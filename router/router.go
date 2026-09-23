// Package router maps application URLs to pages rendered inside persistent
// layouts. Describe the whole route tree with Page, Layout, Group, NotFound
// and Redirect, pass it to New, and mount the Router's View once.
package router

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/reactive"
)

// ErrNotMounted is returned by navigation before the router's View mounts.
var ErrNotMounted = errors.New("router: View is not mounted")

const maxChain = 8

// ErrRedirectLoop is returned when navigation keeps triggering navigation
// without an intervening frame.
var ErrRedirectLoop = fmt.Errorf("router: more than %d chained navigations without a frame", maxChain)

// Router matches locations, reconciles the mounted branch and drives history.
type Router struct {
	leaves    []*route
	notFounds []*route
	builtin   *route // the screen used when no NotFound applies

	history  History
	viewMade bool
	mounted  bool
	post     func(func())

	loc  *ggui.StateValue[Location]
	path *ggui.StateValue[string]
	leaf *ggui.StateValue[*route]
	pos  *ggui.StateValue[[2]int]
	root *ggui.StateValue[*entry]

	current []*entry
	chain   int
}

// entry is one mounted level: an instance and, for a layout, the state its
// outlet follows.
type entry struct {
	inst  instance
	child *ggui.StateValue[*entry]
}

// New validates routes and returns their router. It panics on programmer
// errors such as ambiguous patterns, naming each effective path.
func New(routes ...Route) *Router {
	r := &Router{
		loc:  ggui.State(Location{Path: "/"}),
		path: ggui.State("/"),
		leaf: ggui.State[*route](nil),
		pos:  ggui.State([2]int{0, 1}),
		root: ggui.State[*entry](nil),
	}
	compile(r, routes)
	return r
}

// History replaces the default: memory history starting at "/" on native
// builds, Hash on js/wasm. Call it before View.
func (r *Router) History(h History) *Router {
	if r.viewMade {
		panic("router: History configured after View")
	}
	r.history = h
	return r
}

// View returns the widget that renders the matched branch. Mount it once;
// disposing it unsubscribes history and disposes every route.
func (r *Router) View() ggui.Widget {
	r.viewMade = true
	if r.history == nil {
		r.history = defaultHistory()
	}
	return ggui.Component(func() ggui.Widget {
		if r.mounted {
			panic("router: View is already mounted")
		}
		r.mounted = true
		r.post = ggui.UIThread()
		if d, ok := r.history.(dispatched); ok {
			d.setDispatch(r.post)
		}
		stop := r.history.Subscribe(r.traverse)
		ggui.OnCleanup(func() {
			stop()
			r.mounted = false
			r.current = nil
		})
		r.traverse(r.history.Current())
		return ggui.Key(r.root, r.build)
	})
}

// dispatched is implemented by adapters whose events arrive off the UI
// thread, such as browser callbacks; the router hands them its dispatcher.
type dispatched interface{ setDispatch(post func(func())) }

// match resolves static redirects and falls back to the NotFound of the
// most specific enclosing section.
func (r *Router) match(u parsedURL) (parsedURL, *route, map[string]string, bool) {
	redirected := false
	for {
		leaf, params := r.matchLeaf(u.decoded)
		if leaf == nil {
			break
		}
		if leaf.kind != kindRedirect {
			return u, leaf, params, redirected
		}
		u, redirected = leaf.to, true
	}
	if nf, params := r.matchNotFound(u.decoded); nf != nil {
		return u, nf, params, redirected
	}
	return u, r.builtin, nil, redirected
}

// traverse commits a history entry the router did not create itself: the
// initial entry or a Back/Forward event.
func (r *Router) traverse(e Entry) {
	u, err := parseURL(e.URL)
	if err != nil {
		loc := Location{Path: e.URL, Entry: e.ID}
		r.commit(loc, r.builtin, nil)
		return
	}
	u, leaf, params, redirected := r.match(u)
	loc := u.location()
	if redirected {
		if next, err := r.history.Replace(loc.String()); err == nil {
			e = next
		}
	}
	loc.Entry = e.ID
	r.commit(loc, leaf, params)
}

// Navigate pushes an application-absolute URL such as /projects/1?tab=a.
// Unknown routes are valid and show a not-found screen.
func (r *Router) Navigate(url string) error {
	reactive.CheckUIThread("router.Navigate")
	return r.navigate(url, false)
}

// Replace is Navigate without a new history entry.
func (r *Router) Replace(url string) error {
	reactive.CheckUIThread("router.Replace")
	return r.navigate(url, true)
}

// navigate commits at once. A call from a page's setup runs during the
// frame's flush, after the commit that mounted the page, so it needs no
// queue; chain stops a page that keeps navigating.
func (r *Router) navigate(url string, replace bool) error {
	if !r.mounted {
		return ErrNotMounted
	}
	u, err := parseURL(url)
	if err != nil {
		return err
	}
	// A redirect's own URL never enters history: the destination is pushed
	// or replaced in its place.
	u, leaf, params, _ := r.match(u)
	loc := u.location()
	s := loc.String()
	if s == ggui.Untrack(r.loc.Get).String() {
		return nil
	}
	r.chain++
	if r.chain == 1 {
		r.post(func() { r.chain = 0 })
	}
	if r.chain > maxChain {
		return ErrRedirectLoop
	}
	var e Entry
	if replace {
		e, err = r.history.Replace(s)
	} else {
		e, err = r.history.Push(s)
	}
	if err != nil {
		return err
	}
	loc.Entry = e.ID
	r.commit(loc, leaf, params)
	return nil
}

// commit publishes the location and match together, then reconciles.
func (r *Router) commit(loc Location, leaf *route, params map[string]string) {
	index, length := r.history.Position()
	r.loc.Set(loc)
	r.path.Set(loc.Path)
	r.leaf.Set(leaf)
	r.pos.Set([2]int{index, length})
	r.reconcile(branch(leaf, params))
}

// reconcile keeps the longest common instance prefix and writes only the
// outlet at the first diverging depth. Outlets below it are created fresh
// with their initial entry, so they never observe a location their
// ancestors are about to discard.
func (r *Router) reconcile(next []instance) {
	old := r.current
	d := 0
	for d < len(old) && d < len(next) && old[d].inst.key == next[d].key {
		d++
	}
	if d == len(old) && d == len(next) {
		return
	}
	fresh := make([]*entry, len(next))
	copy(fresh, old[:d])
	var first *entry
	for i := len(next) - 1; i >= d; i-- {
		e := &entry{inst: next[i]}
		if next[i].route.kind == kindLayout {
			e.child = ggui.State(first)
		}
		fresh[i], first = e, e
	}
	r.current = fresh
	if d == 0 {
		r.root.Set(first)
	} else {
		old[d-1].child.Set(first)
	}
}

// build mounts one entry under the owner its outlet's Key provides.
func (r *Router) build(e *entry) ggui.Widget {
	if e == nil {
		return nil
	}
	c := &Context{r: r, inst: e.inst, owner: reactive.CurrentOwner()}
	rt := e.inst.route
	if rt.kind == kindLayout {
		return rt.layout(c, ggui.Key(e.child, r.build))
	}
	return rt.page(c)
}

// Back moves one entry back; it is a no-op at the first entry.
func (r *Router) Back() {
	reactive.CheckUIThread("router.Back")
	r.traversal(-1)
}

// Forward moves one entry forward; it is a no-op at the last entry.
func (r *Router) Forward() {
	reactive.CheckUIThread("router.Forward")
	r.traversal(1)
}

func (r *Router) traversal(delta int) {
	if r.mounted {
		r.history.Go(delta)
	}
}

// Location reads the current location; it is a tracked read.
func (r *Router) Location() Location { return r.loc.Get() }

// CanBack reports whether Back has an entry to move to.
func (r *Router) CanBack() ggui.Readable[bool] {
	return reader[bool](func() bool { return r.pos.Get()[0] > 0 })
}

// CanForward reports whether Forward has an entry to move to.
func (r *Router) CanForward() ggui.Readable[bool] {
	return reader[bool](func() bool { p := r.pos.Get(); return p[0] < p[1]-1 })
}

// Active reports whether the current path is path or lies below it, by
// whole segments. It ignores query and fragment.
func (r *Router) Active(path string) ggui.Readable[bool] {
	u, err := parseURL(path)
	if err != nil {
		panic(err)
	}
	prefix := u.path()
	below := prefix + "/"
	return reader[bool](func() bool {
		p := r.path.Get()
		return prefix == "/" || p == prefix || strings.HasPrefix(p, below)
	})
}

// Current is the target of the matched page, or nil for a not-found screen.
func (r *Router) Current() ggui.Readable[*Target] {
	return reader[*Target](func() *Target {
		if l := r.leaf.Get(); l != nil {
			return l.target
		}
		return nil
	})
}

// reader reads router state inside Get, so it needs no owner.
type reader[T any] func() T

func (f reader[T]) Get() T { return f() }
