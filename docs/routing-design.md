# Explicit page and layout routing (proposal)

[Documentation](README.md) · [State and components](reactivity.md) · [Data and navigation](data-and-navigation.md)

Status: in progress. Stages 1–4 of the [implementation plan](#implementation-plan) are implemented in `router/`, with `examples/router`; the Browser adapter, outlet placement checks and the Tab anchor are not. Names may change.

## On this page

- [Scope](#scope)
- [Concepts](#concepts)
- [Route tree](#route-tree)
- [Rendering pages and layouts](#rendering-pages-and-layouts)
- [Navigating](#navigating)
- [Route context](#route-context)
- [Instance identity and lifetime](#instance-identity-and-lifetime)
- [Not-found screens](#not-found-screens)
- [Data and failures](#data-and-failures)
- [History adapters](#history-adapters)
- [API reference](#api-reference)
- [Implementation plan](#implementation-plan)

## Scope

Add `github.com/ironpark/ggui/router` as an optional package that depends on
`ggui`. Neither the core package nor `ui` imports it. Applications describe
their route tree in Go; there is no file discovery, code generation, server
rendering, or server-only data loader.

The first version proves stable nested layout ownership. It deliberately
leaves out navigation guards, dynamic redirects, transitions, prefetching,
route resource caches, keep-alive, scroll restoration and file routing.

## Concepts

| Term | Meaning |
| --- | --- |
| Page | Owns one screen. A leaf of the tree. |
| Layout | Owns persistent chrome around a child outlet. Survives while its section stays matched. |
| Group | Adds a URL section without UI. |
| NotFound | The screen for unknown URLs inside its section. |
| Redirect | A leaf that sends one URL to another before anything mounts. |
| Router | Matches locations, reconciles the mounted branch, and drives history. |
| Context | A route-scoped handle passed to factories: parameters, query, navigation. |
| Target | The value `Page` returns: a tree node that is also a typed navigation handle. |

UI controls navigate through ordinary callbacks. The router does not depend
on a theme or control library, and no control depends on the router.

## Route tree

The tree reads like a site map. Each nesting level adds a URL section, a
layout, or both, and every `Page` line shows the path next to the screen it
renders. `New` receives the complete tree as one nested value, in the same
style as ggui widget trees.

```go
r := router.New(
    router.Page("/login", loginPage),                 // no app chrome

    router.Layout("", appLayout,                      // chrome only
        router.Page("/", homePage),                   // /
        router.Redirect("/home", "/"),                // /home → /

        router.Layout("/admin", adminLayout,          // section + chrome
            router.Page("/", dashboardPage),          // /admin
            router.Page("/users", usersPage),         // /admin/users
            router.Page("/users/:id", userPage),      // /admin/users/42

            router.Group("/settings",                 // section only
                router.Page("/profile", profilePage), // /admin/settings/profile
            ),
            router.NotFound(adminNotFoundPage),       // unknown /admin/...
        ),
        router.NotFound(notFoundPage),                // any other unknown URL
    ),
)

return r.View()
```

| Constructor | Meaning |
| --- | --- |
| `Page(path, page)` | This URL shows this screen. Returns a `*Target`. |
| `Layout(path, layout, children...)` | Children live under `path` and render inside `layout`. `""` adds no URL segment. |
| `Group(path, children...)` | Children live under `path`; no extra UI. |
| `NotFound(page)` | Screen for unknown URLs in the enclosing section. |
| `Redirect(path, to)` | Visiting `path` replaces the location with the literal URL `to`. |

A section is an ordinary value, so large trees split into functions:

```go
func adminRoutes() router.Route {
    return router.Layout("/admin", adminLayout,
        router.Page("/", dashboardPage),
        router.Page("/users", usersPage),
    )
}
```

### Path syntax

Paths compose the way route groups do in chi or gin: a child's path is
appended to its parents', and `"/"` is the section's own URL. The leading
slash is notation only; it never escapes the section.

| Segment | Matches | Example |
| --- | --- | --- |
| `users` | Exactly that text, case-sensitively. | `/users` |
| `:id` | One non-empty segment; `Param("id")` returns it decoded. | `/users/42` |
| `*rest` | The remainder of the path, including slashes. Must be last. | `/docs/guide/intro` |

Matching compares segment by segment from the left. At each position a
static segment beats a parameter, and a parameter beats a catch-all.
Patterns that are equally specific for some URL are rejected by `New`, so
route order never matters.

Pages require full-path matches. A trailing slash is removed from every
path except `/` before matching and before the location is stored, so
`/admin/` and `/admin` are one location. Internal duplicate slashes, bad
escapes and encoded slashes inside a single-segment parameter are matching
failures, not silent corrections.

### Validation

`New` validates the whole tree immediately and panics with effective paths
on programmer errors:

- duplicate or ambiguous effective patterns, including the same URL under
  different layouts;
- a parameter name declared twice on one branch;
- a catch-all that is not the final segment;
- more than one `"/"` page for one section URL;
- two `NotFound` nodes in one section;
- a `Redirect` whose target matches no page, or a chain of redirects that
  loops;
- a nil factory;
- a node placed twice in one tree or in two routers.

Because the tree is complete when `New` runs, there is no registration
phase, no configuration freeze and no ordering rule. Navigation errors are
runtime errors returned from `Navigate`.

## Rendering pages and layouts

```go
func homePage(c *router.Context) ggui.Widget {
    return ggui.Text("Home")
}

func appLayout(c *router.Context, page ggui.Widget) ggui.Widget {
    return ggui.Column(header(), page)
}
```

Factories run once per mount with untracked setup reads, exactly like
`Component`. Use bindings, `Derived`, `Effect` and `Resource` for updates.

The layout's `page` argument is its outlet. Place it exactly once in the
returned tree. The router switches its content; applications never build
outlets, keys or ownership scopes for routes themselves. Layouts wrap in
tree order, so `/admin/users` renders appLayout → adminLayout → usersPage.
A page outside every `Layout`, like `/login`, has no chrome; there is no
layout-removal option.

The outlet and the page inside it belong to the layout instance, not to the
widget that happens to contain the outlet. Placing the outlet inside an `If`
branch, a `Tabs` page or a collapsed container hides the page without
disposing it; only navigation or disposing the router's `View` disposes
pages. Laying the outlet out from a second parent panics. An outlet that is
never laid out after the first frame is reported in `ggui_debug` builds,
because its page runs setup and resources without being visible.

## Navigating

Navigate with the URL you would type. URLs are application-absolute, such as
`/projects/1?tab=activity`; external and relative URLs return an error.
Navigating to the same normalized URL is a no-op. Unknown routes are valid
navigation that shows a not-found screen, not a `Navigate` error.

```go
err := r.Navigate("/admin/users/42")
err = r.Replace("/admin/users/42?tab=security")
r.Back()
```

`Router` and `Context` expose the same actions. Pass `c.Navigate`, a
callback, or the `Context` to a child that needs navigation. There is no
global active router and no implicit lookup.

### Active sections

`Active` answers "is the user inside this section?" for menus and sidebars.
It matches whole segments, so `/admin/users/42` activates `"/admin/users"`
while `/administrator` does not activate `"/admin"`. It ignores query and
fragment, so query updates do not change it. `Active("/")` is true on every
page; use a target for the home page.

```go
navItem("Users", r.Active("/admin/users"), func() { _ = r.Navigate("/admin/users") })
```

Readers returned by `Active`, `Current`, `CanBack` and `CanForward` read the
router's state inside `Get`, so they need no owner and are never disposed;
they are valid as long as the router is. Creating them in a builder is free.

`ui.Sidebar` takes a `Binding[string]`; with URL keys, `ggui.Bind` adapts
the router without new API:

```go
page := ggui.Bind(
    func() string { return r.Location().Path },
    func(path string) { _ = r.Navigate(path) },
)
ui.Sidebar(page,
    ui.SidebarItem("/admin", "Dashboard"),
    ui.SidebarItem("/admin/users", "Users"),
)
```

The sidebar highlights exact keys only, so `/admin/users/42` highlights
nothing. Custom menus should use `Active` instead.

### Typed targets

URL strings are enough for most applications. Keep the value `Page` returns
when links should survive path changes or parameters should be checked. A
`*Target` is both the tree node and a navigation handle; its full path comes
from where it is placed.

```go
user := router.Page("/users/:id", userPage)

r := router.New(
    router.Layout("/admin", adminLayout,
        user,                                         // /admin/users/:id
    ),
)

err := user.Navigate(router.Params{"id": "42"})
url, err := user.URL(router.Params{"id": "42"})       // /admin/users/42
exact := user.Active()                                // this page, any id
```

`URL` and `Navigate` take one `Params` map; pass nil for a parameterless
page. Every parameter, including those declared by enclosing sections, must
be supplied; targets do not infer values from the mounted route. Missing,
extra or invalid parameters return errors, as does a target not yet passed
to `New`. `URL` escapes parameter values. A single-segment value containing
a slash is rejected, consistent with matching; a catch-all value is split
and escaped segment by segment. Compose query and fragment with `net/url`.

`URL` works without mounting. `Navigate` follows the same UI-thread and
mounted-router rules as `Router.Navigate`. Targets store route identity,
not widget instances. Declare them in setup code or an application struct
passed to pages, not as package-level variables: a package-level target
whose page function refers back to it is a Go initialization cycle.

`Target.Active` reports whether the target's route is the matched leaf,
ignoring parameters and query. `Router.Current` exposes the matched leaf
target, or nil for a not-found screen.

### Redirects

`Redirect(path, to)` is resolved during matching, before anything mounts.
`to` is a literal application-absolute URL; parameters are not forwarded in
v1. `New` verifies that `to` matches a page or another redirect and that no
chain loops, so redirects need no runtime loop counter. A redirect replaces
the history entry: Back from `/` after visiting `/home` does not return to
`/home`.

Dynamic redirects, such as sending signed-out users to `/login`, are
navigation guards and are deferred. Until then, a page may call `Replace`
during setup; see [Queued navigation](#queued-navigation) for its cost.

## Route context

Factories receive an explicit, route-scoped `Context`:

```go
id := c.Param("projectID")           // string, constant for this instance
query := c.Query("q")                // string; "" when absent
loc := c.Location()                  // immutable snapshot
err := c.Navigate("/settings/profile")
```

`Param` is constant for a `Context`'s lifetime. Path parameters are part of
the instance key, so a change remounts the instance with a new `Context`
instead of updating the old one. Reading `Param` into a local variable
during setup is correct and needs no tracking. `Param` sees parameters
matched through its own node; a parent never depends on parameters that a
child introduces.

`Query` and `Location` change while the instance stays mounted. They are
tracked reads: inside `Effect`, `Derived`, `Reactive` or a `Resource` input,
updates subscribe automatically. In one-time page or layout setup they are
snapshots, exactly like `State.Get`. Each `Query(name)` tracks only that
name: it is backed by a per-name derived string owned by the route
instance, and a derived value notifies only when its result changes, so an
unrelated query update reruns nothing.

```go
func projectPage(c *router.Context) ggui.Widget {
    id := c.Param("projectID")
    tasks := ggui.Resource(func() taskQuery {
        return taskQuery{Project: id, Filter: c.Query("filter")}
    }, loadTasks)
    return tasksView(tasks) // Await renders pending, ready and failed
}
```

`Location` is an immutable value: the normalized path, the raw query, the
fragment and the history entry ID. Its query accessors return copies; no
shared map or `url.Values` is exposed.

A `Context` is valid only while its route instance is mounted. Workers use
`Resource` or a captured `UIThread()` dispatcher rather than reading a
`Context` off-thread.

## Instance identity and lifetime

`New` assigns every node a route ID. An instance key is the route ID plus
the decoded path parameters matched through that node. Query, fragment and
history entry ID do not participate in identity.

| Navigation | Preserved | Recreated |
| --- | --- | --- |
| settings/profile → settings/appearance | App and settings layouts | Page |
| projects/1 → projects/1/tasks/2 | App and project layouts | Project page replaced by task page |
| projects/1/tasks/2 → projects/1/tasks/3 | App and project layouts | Task page |
| projects/1 → projects/2 | App layout | Project layout and descendants |
| Same path, different query or fragment | All mounted instances | Nothing; location readers update |
| Leave a page, then go Back | Common currently mounted ancestors | Revisited page starts fresh |

This deliberately resets entity-specific state when path parameters change.
History stores locations, not live widget trees. Shared state belongs above
the switching boundary; automatic keep-alive is outside the first version.

A catch-all value is a path parameter too: `/docs/*rest` remounts when
`rest` changes. Put a parameterless layout above it and read `Location`
when a document viewer should survive between documents.

### Commit sequence

Every navigation, programmatic or from a history event, runs one commit on
the UI thread:

1. Parse and match the next location into a complete branch of instance
   keys, following static redirects. A failure here leaves the current
   screen and history untouched.
2. Reserved for navigation guards. Browser traversal has already happened
   by the time the router sees it, so a guard can only undo it with another
   navigation; nothing may assume every navigation is cancellable.
3. For programmatic navigation, mutate the history backend; a failure
   returns an error and stops.
4. Publish location and match as one snapshot, so no reader observes a new
   location with an old match.
5. Reconcile outlets: keep the longest common instance prefix, dispose the
   departed suffix through normal ownership cleanup, and mount the new
   suffix.

Outlets are driven by the commit, not by tracked reads of the match. Each
outlet holds its child in its own state, and step 5 writes only the outlet
at the first diverging depth; outlets below it are created fresh and read
their initial instance without tracking. Deeper outlets therefore never
observe a location their ancestors are about to discard. Nested `ggui.Key`
boundaries keyed on a derived match would currently work only because
effects flush in creation order; do not depend on that, and do not rebuild
the whole route tree inside one `Reactive` callback.

### Queued navigation

`Navigate` or `Replace` called while a commit is running, for example from
a page's setup or a synchronous effect, is queued and runs after the current
commit; only the last queued navigation runs. A setup-time `Replace`
therefore works as a redirect, at the cost of briefly mounting the
redirecting page and running its resources. Prefer a `Redirect` node when
the target is static. More than eight chained commits without an
intervening frame are reported as a redirect loop and stop at the current
screen.

### Focus after navigation

If the focused control belongs to the departed suffix, focus moves to no
control. Controls with an identity, such as every `ui` control, get a fresh
one under the new outlet, and the input system drops the stale focus at the
next input dispatch. A bare `ggui.Focus` without an identity is matched by
its rectangle, so a new page's control in the same place inherits focus;
give such controls a key when that matters. The goal is that the next Tab starts at the new page's outlet
rather than the window's first control. That needs a Tab anchor in the
existing focus scope machinery; until one exists, Tab restarting at the
window is the documented v1 behavior. Focus inside retained layouts is
untouched. Moving focus to a page heading and announcing the page to
assistive technologies are deferred, but the commit reserves a post-mount
hook for them.

## Not-found screens

A `NotFound` node's position states both its scope and its chrome. Its
section is the nearest enclosing `Group` or `Layout` with a non-empty path,
or the root when there is none. It renders inside exactly the layouts that
enclose the node.

Matching tries full page matches and redirects first, anywhere in the tree.
If none exists, the router selects the most specific matching section by
segment boundaries with the usual static-over-parameter precedence, walks
from that section toward the root for the nearest `NotFound`, and renders it
inside its enclosing layouts; layouts below them are disposed. In the tree
above, `/admin/unknown` renders adminNotFoundPage inside appLayout and
adminLayout, `/unknown` renders notFoundPage inside appLayout, and `/login`
is never shadowed by either. Without any `NotFound` node, a built-in minimal
screen renders with no layouts.

A section holds at most one `NotFound`, even across sibling pathless
layouts; `New` rejects a second one instead of choosing by order.

`NotFound` receives a normal `Context` with the full unmatched `Location`
and the parameters matched through its section. The unknown URL is
committed to history. The node is a synthetic leaf with the same lifecycle
as a page; layouts shared with the previous screen stay mounted.

## Data and failures

Use `ggui.Resource` and `ggui.Await` inside page or layout setup. This keeps
loading typed and avoids a `map[string]any` loader contract. Layouts share
data with pages through application closures or a struct passed to
factories; a typed ownership context can be considered later if explicit
sharing becomes cumbersome.

Route exit cancels owned resources. A query change reloads only a resource
whose input reads that query. Existing request IDs discard stale results.
Navigation commits immediately; pending and failure UI is rendered by
`Await` inside the appropriate layout or page. The router never waits for
route resources before updating the location.

## History adapters

Configure history before mounting `View`; configuring afterwards panics,
following the convention for fluent blocks. Native builds default to memory
history starting at `/`; js/wasm builds default to hash history starting at
the current browser URL.

```go
r := router.New(routes...).History(router.Memory("/admin"))
```

| Adapter | Use |
| --- | --- |
| `Memory(initialURL)` | Native applications and deterministic tests. |
| `Hash()` | Browser URLs such as `/#/settings/profile`; works on static hosting without server fallback. |
| `Browser(basePath)` | Clean browser paths through pushState, replaceState and popstate. The host must serve the app entry for every route URL, and asset URLs must resolve against the application base, not the active route. |

Adapters implement one small interface so tests can substitute a fake:

```go
type Entry struct {
    URL string
    ID  uint64 // unique per entry, stable across traversal
}

type History interface {
    Current() Entry
    Push(url string) (Entry, error)
    Replace(url string) (Entry, error)
    Go(delta int)                           // traversal; may complete asynchronously
    Position() (index, length int)          // entries this adapter knows about
    Subscribe(func(Entry)) (stop func())    // traversal events, delivered on the UI thread
}
```

`CanBack` and `CanForward` derive from `Position` and drive native toolbar
buttons and shortcuts. Memory history knows its position exactly. A browser
cannot reveal entries outside the application, so `Hash` and `Browser`
count only entries this router pushed in the current document; Back beyond
them still delegates to the browser. `Go` is a no-op at a memory boundary.

Browser code stays behind js/wasm build constraints. Programmatic navigation
publishes only after the backend mutation succeeds; browser traversal
publishes from its event. Adapters preserve unrelated browser history state
and tag their own entries to prevent duplicate commits. `Memory` and `Hash`
ship first; `Browser` follows once base-path and host fallback behavior are
tested.

Fragment changes update state only; anchor scrolling is not part of v1. New
pages get fresh control identities, so page scroll positions are not
restored; layout-owned scroll state survives while the layout is mounted.

## API reference

```go
type PageFunc func(*Context) ggui.Widget
type LayoutFunc func(*Context, ggui.Widget) ggui.Widget

type Params map[string]string

// Route is a sealed tree node; only this package implements it.
type Route interface{ route() }

func Page(path string, build PageFunc) *Target // *Target implements Route
func Layout(path string, build LayoutFunc, children ...Route) Route
func Group(path string, children ...Route) Route
func NotFound(build PageFunc) Route
func Redirect(path, to string) Route

func New(routes ...Route) *Router            // validates; panics on conflicts
func (*Router) History(History) *Router     // before View; panics afterwards
func (*Router) View() ggui.Widget           // mount once

func (*Router) Navigate(url string) error
func (*Router) Replace(url string) error
func (*Router) Back()
func (*Router) Forward()
func (*Router) CanBack() ggui.Readable[bool]
func (*Router) CanForward() ggui.Readable[bool]
func (*Router) Location() Location          // tracked read
func (*Router) Active(path string) ggui.Readable[bool] // whole-segment prefix
func (*Router) Current() ggui.Readable[*Target]

func (*Context) Param(name string) string   // constant per instance
func (*Context) Query(name string) string   // tracked, per-name
func (*Context) Location() Location         // tracked read
func (*Context) Navigate(url string) error
func (*Context) Replace(url string) error
func (*Context) Back()
func (*Context) Router() *Router

func (*Target) URL(params Params) (string, error)
func (*Target) Navigate(params Params) error
func (*Target) Active() ggui.Readable[bool]

type Location struct {
    Path     string // normalized, escaped
    RawQuery string
    Fragment string
    Entry    uint64
}
func (Location) Query(name string) string
func (Location) HasQuery(name string) bool
func (Location) QueryAll(name string) []string // copy
func (Location) String() string                // application-absolute URL
```

Create the router once during app setup and mount its `View` once. `View`
provides the component owner for route state and history subscriptions.
Navigation and reads follow ggui's UI-thread rules; history events dispatch
to that thread. Disposing `View` unsubscribes history and disposes all
routes. A second concurrent `View` for one router is an error. Nodes are
immutable descriptions: the router assigns IDs and owns mounted instances,
so one tree value cannot be shared between routers.

## Implementation plan

1. Tree construction and `New`-time validation: section and layout nesting,
   redirect checks, `Router.Active` segment matching, target URL escaping,
   not-found scoping, and memory history tests. Verify that full page
   matches beat `NotFound` in every scope.
2. Mounted `View` with commit-driven nested outlets. Probe tests for factory
   counts, retained layout state, parameter remounts, query updates,
   per-name `Query` tracking, `Active` and `Current`, static redirects, and
   hidden or doubly placed outlets.
3. Resource cancellation, stale-result rejection, cleanup-once, rapid
   repeated navigation, navigation queued during commit, redirect-loop
   reporting, whole-router disposal and departed-control focus tests.
4. `Hash` adapter with real-browser initial URL, reload, Back/Forward and
   event teardown checks, plus a small navigation example.
5. `Browser` adapter with deployment and base-path documentation.

Deferred: navigation guards, dynamic redirects, transitions, prefetching,
route resource caches, keep-alive, scroll restoration and file routing.
