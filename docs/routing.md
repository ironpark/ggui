# Routing

[Documentation](README.md) · [Project README](../README.md) · [Router example](examples/router)

Package `router` (`github.com/ironpark/ggui/router`) maps application URLs to
pages rendered inside persistent layouts. Describe the whole route tree with
`Page`, `Layout`, `Group`, `Redirect`, and `NotFound`, pass it to `router.New`,
and mount `r.View()` once.

Package `router` depends only on `ggui` and `widget` (plus internal packages);
neither `ggui` nor `ui` imports `router`, so routing stays opt-in and works with
any widgets or themed controls.

## On this page

- [Quick start](#quick-start)
- [Defining routes](#defining-routes)
- [Pages, layouts, and lifecycle](#pages-layouts-and-lifecycle)
- [Navigating and active state](#navigating-and-active-state)
- [Reading params, query, and location](#reading-params-query-and-location)
- [Not-found screens](#not-found-screens)
- [History and web deployment](#history-and-web-deployment)
- [API reference and limitations](#api-reference-and-limitations)

## Quick start

Define the route tree, mount `r.View` as the window's root builder, and wire
any keyboard shortcuts you want for history traversal:

```go
package main

import (
	"fmt"
	"log"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/router"
	"github.com/ironpark/ggui/ui"
	"github.com/ironpark/ggui/widget"
)

func routes() *router.Router {
	var r *router.Router
	r = router.New(
		router.Layout("", func(c *router.Context, page ggui.Widget) ggui.Widget {
			return appLayout(r, page)
		},
			router.Page("/", homePage),
			router.Redirect("/home", "/"),
			router.Layout("/projects/:id", projectLayout,
				router.Page("/", overviewPage),
				router.Page("/tasks", tasksPage),
			),
			router.Page("/settings", settingsPage),
			router.NotFound(notFoundPage),
		),
	)
	return r
}

func appLayout(r *router.Router, page ggui.Widget) ggui.Widget {
	back := ui.Button("‹ Back", r.Back).Ghost().BindDisabled(ggui.Not(r.CanBack()))
	forward := ui.Button("Forward ›", r.Forward).Ghost().BindDisabled(ggui.Not(r.CanForward()))
	path := ggui.Derived(func() string { return r.Location().String() })
	toolbar := widget.Row(back, forward, widget.TextOf(path)).Space(1).Align(widget.AlignCenter)

	sidebar := widget.Column(
		ui.Title("Router"),
		navLink(r, "Home", "/", ggui.Derived(func() bool { return r.Location().Path == "/" })),
		navLink(r, "Project 1", "/projects/1", r.Active("/projects/1")),
		navLink(r, "Settings", "/settings", r.Active("/settings")),
	).Space(0.5)

	return widget.Box(widget.Row(
		widget.Box(sidebar).Width(200),
		widget.Expanded(widget.Column(toolbar, ui.Card(page).Pad(24)).Space(1)),
	).Space(2)).Pad(16)
}

func navLink(r *router.Router, label, url string, active ggui.Readable[bool]) ggui.Widget {
	go_ := func() { _ = r.Navigate(url) }
	return ggui.If(active, func() ggui.Widget { return ui.Button(label, go_).Secondary() }).
		Else(func() ggui.Widget { return ui.Button(label, go_).Ghost() })
}

func homePage(c *router.Context) ggui.Widget {
	return widget.Text("Home")
}

func projectLayout(c *router.Context, page ggui.Widget) ggui.Widget {
	return widget.Column(ui.Title("Project "+c.Param("id")), page).Space(1)
}

func overviewPage(c *router.Context) ggui.Widget {
	return widget.Text(fmt.Sprintf("Overview of project %s.", c.Param("id")))
}

func tasksPage(c *router.Context) ggui.Widget {
	filter := ggui.Derived(func() string {
		if f := c.Query("filter"); f != "" {
			return "Tasks filtered by " + f + "."
		}
		return "All tasks."
	})
	return widget.TextOf(filter)
}

func settingsPage(c *router.Context) ggui.Widget {
	return widget.Text("Settings")
}

func notFoundPage(c *router.Context) ggui.Widget {
	return widget.Column(
		ui.Title("Not found"),
		widget.TextOf(ggui.Map(c.LocationValue(), func(l router.Location) string {
			return "No page at " + l.Path + "."
		})),
		ui.Button("Go home", func() { _ = c.Navigate("/") }),
	).Space(1)
}

func main() {
	r := routes()
	app := ggui.New(ggui.Config{
		Title:     "ggui · router",
		Width:     820,
		Height:    520,
		Resizable: true,
	}, r.View)
	app.Shortcut("alt+left", r.Back)
	app.Shortcut("alt+right", r.Forward)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

See [docs/examples/router/main.go](examples/router/main.go) for the full
runnable example (`go run ./docs/examples/router`).

## Defining routes

Five constructors build a route tree:

| Constructor | Role |
| --- | --- |
| `router.Page(path, build)` | Leaf screen at `path`, relative to its enclosing section. Returns `*router.Target`, which is both a `Route` node and a typed navigation handle. |
| `router.Layout(path, build, children...)` | Enclosing section under `path` that renders its children inside persistent chrome. An empty path `""` adds no URL segment. |
| `router.Group(path, children...)` | Enclosing section under `path` that shares a URL prefix (and optional `NotFound`) without adding UI. |
| `router.Redirect(path, to)` | Static redirect from `path` to the literal application-absolute URL `to` before anything mounts. |
| `router.NotFound(build)` | Fallback screen for unmatched URLs inside the enclosing section. |

### Path syntax and matching

Registration paths are relative to their enclosing `Layout` or `Group`. Leading
and trailing slashes are normalized (`""` and `"/"` contribute no segments, so
`Page("/", fn)` matches the section's own URL and `/users/new/` matches
`/users/new`).

Three segment kinds are supported:

- **Static** (`/users/new`): matches a literal decoded segment.
- **Parameter** (`:id`): matches one non-empty decoded segment. An encoded slash
  (`%2F`) does not match a parameter.
- **Catch-all** (`*rest`): matches one or more remaining decoded segments,
  joined with `"/"`. Must be the final segment of a `Page` or `Redirect` (never
  a `Layout` or `Group`). Segments containing an encoded slash do not match.

Matching precedence is independent of registration order. Comparing segment
kinds left to right, **static beats `:param`, and `:param` beats `*catchAll`**.
Where one pattern ends, the longer pattern wins. For example, `/users/new`
always wins over `/users/:id`, and `/docs/:slug/edit` always wins over
`/docs/*rest`, regardless of the order they appear in `router.New`.

### Validation in `router.New`

`router.New` compiles and validates the whole tree up front and panics on
programmer errors, naming the offending effective path:

- ambiguous leaf patterns with the same segment-kind signature (such as `/a/:x`
  and `/a/:y`, even inside different layouts or groups);
- duplicate parameter names along one path (such as `/:id/items/:id`);
- a catch-all outside the final segment of a `Page` or `Redirect`;
- empty segments (`//`), unnamed parameters (`/:`), or unnamed catch-alls (`/*`);
- two `NotFound` nodes in the same section;
- a `nil` route, `nil` builder function, or unknown `Route` implementation;
- placing the same `Route` node twice or passing a `*Target` to a second router;
- a `Redirect` whose destination is not a valid application-absolute URL,
  matches no page, or forms a redirect loop.

## Pages, layouts, and lifecycle

`PageFunc` (`func(*Context) ggui.Widget`) and `LayoutFunc`
(`func(c *Context, page ggui.Widget) ggui.Widget`) run **once per mount**, just
like `ggui.Component`. Place `page` once in the widget tree returned by a
`LayoutFunc` where the nested layout or leaf page should appear.

### Instance identity and persistence

Every mounted node in a matched branch has an identity composed of its compiled
**route ID plus the path parameters matched through that node**. On navigation,
the router keeps the longest matching prefix of mounted instances and replaces
only the outlet at the first diverging depth:

- Navigating between `/projects/1` and `/projects/1/tasks`, or changing query
  parameters (`/projects/1/tasks?filter=open`), keeps both the root layout and
  `projectLayout` mounted — preserving local signals, text inputs, scroll
  positions, effects, and focus inside those layouts.
- Navigating from `/projects/1/tasks` to `/projects/2/tasks` changes `:id`, so
  the root layout stays mounted while `projectLayout` and `tasksPage` unmount
  and remount with a new `*Context` and reactive owner.
- Navigating to a sibling outside a layout (such as `/login` outside `appLayout`)
  unmounts that layout and everything below it.

### Async data and cleanup on unmount

Because each mounted page and layout runs under its own reactive owner, any
`ggui.OnCleanup` callback runs when that instance unmounts or remounts on a
parameter change, and any `ggui.Resource` started inside it cancels its
`context.Context` automatically:

```go
func projectPage(c *router.Context) ggui.Widget {
	id := c.Param("id")
	res := ggui.Resource(
		func() string { return id },
		func(ctx context.Context, id string) (Project, error) {
			return fetchProject(ctx, id) // cancelled when leaving or switching :id
		},
	)
	return ggui.Await(res).
		Pending(func() ggui.Widget { return widget.Text("Loading…") }).
		Error(func(err ggui.Readable[error]) ggui.Widget { return widget.Textf("Error: %v", err) }).
		Then(func(p ggui.Readable[Project]) ggui.Widget { return projectDetail(p) })
}
```

### Setup-time `Replace` and redirect loops

A page or layout may redirect conditionally during setup by calling
`c.Replace(...)` and returning `nil`:

```go
func adminPage(c *router.Context) ggui.Widget {
	if !isSignedIn() {
		_ = c.Replace("/login")
		return nil
	}
	return adminDashboard()
}
```

Multiple `Navigate`/`Replace` calls before the next frame build only the final
matched page. To stop accidental ping-pong redirects, the router caps chained
navigations without an intervening frame at **8 steps**; exceeding that limit
returns `router.ErrRedirectLoop` until the next frame runs.

## Navigating and active state

Drive the router from UI callbacks on the UI thread with `Router` (or the
forwarding methods on `*Context`):

- `r.Navigate(url)` pushes a new history entry and commits the match.
- `r.Replace(url)` replaces the current history entry without adding one.
- `r.Back()` and `r.Forward()` traverse history (no-ops at either end or while
  unmounted).
- `r.CanBack()` and `r.CanForward()` return `ggui.Readable[bool]` values for
  enabling or disabling navigation controls.

URLs passed to `Navigate` and `Replace` must be application-absolute (starting
with `/`, with no scheme or host, no `//`, and valid percent escapes). Unknown
paths are valid and render the matching not-found screen. Navigating to the
exact current normalized URL (same path, query, and fragment) is a no-op.
Calling `Navigate` or `Replace` before `View` mounts or after it disposes
returns `router.ErrNotMounted`.

### Active paths and `ui.Sidebar`

`r.Active(path)` returns a `ggui.Readable[bool]` that is true when the current
path equals `path` or lies below it, matching **whole decoded segments** and
ignoring query and fragment:

- At `/admin/users`, `r.Active("/admin")` and `r.Active("/admin/users")` are
  true, while `r.Active("/adm")` is false.
- Percent-encoding differences are normalized (`/docs/hello%20world` matches
  `r.Active("/docs/hello world")`).
- `r.Active` panics if `path` is not a valid application-absolute URL.
- `r.Active("/")` is always true because every path lies below `"/"`. For a
  home link, compare the path directly or use a typed target:

```go
homeActive := ggui.Derived(func() bool { return r.Location().Path == "/" })
```

To drive a [`ui.Sidebar`](data-and-navigation.md#sidebar-navigation) directly from the
router without maintaining a second piece of state, adapt it with `ggui.Bind`:

```go
selected := ggui.Bind(
	func() string { return r.Location().Path },
	func(path string) { _ = r.Navigate(path) },
)

sidebar := ui.Sidebar(selected,
	ui.SidebarSection("Workspace"),
	ui.SidebarItem("/", "Home"),
	ui.SidebarItem("/projects/1", "Project 1"),
	ui.SidebarItem("/settings", "Settings"),
)
```

### Typed targets

`router.Page` returns a `*router.Target` that records its full effective pattern
once passed to `router.New`:

```go
var (
	homePageTarget    = router.Page("/", homePage)
	projectTasksTarget = router.Page("/tasks", tasksPage)
)

r := router.New(
	homePageTarget,
	router.Layout("/projects/:id", projectLayout,
		projectTasksTarget,
	),
)
```

- `target.URL(params)` builds the escaped application-absolute URL (pass `nil`
  when the route has no parameters). Every parameter in the effective pattern
  must be supplied and non-empty; unknown parameters, slashes in a `:param`, or
  empty catch-all segments return an error, as does calling `URL` before passing
  the target to `router.New`.
- `target.Navigate(params)` builds the URL and calls `r.Navigate`.
- `target.Active()` returns a `ggui.Readable[bool]` that is true when this exact
  page is the matched leaf, regardless of parameter values or query string.
- `r.Current()` returns `ggui.Readable[*router.Target]` for the matched leaf
  page, or `nil` when a not-found screen is showing.

### Redirect history behavior

Static `router.Redirect(from, to)` routes follow chains to a literal destination
URL before mounting anything:

- On `Navigate` or `Replace`, the router resolves the redirect chain *before*
  writing history, so the redirect's own URL never enters the history stack and
  `Back` returns directly to the page before it.
- On an initial history entry or history traversal (such as loading `/old` at
  startup or navigating browser history to it), the router resolves the redirect
  and calls `history.Replace` with the destination URL.
- Redirects go to the literal `to` URL; query parameters and fragments from the
  matched source URL are not forwarded.

## Reading params, query, and location

`*router.Context` separates values that stay constant for a route instance from
values that can change while the instance stays mounted:

| Method | Behavior | When to use |
| --- | --- | --- |
| `c.Param(name)` | Plain string, constant for the instance's lifetime (a parameter change remounts the route). | Directly in page or layout setup. |
| `c.Query(name)` | Tracked read returning the first value of `name` (or `""`). Only notifies when `name`'s value changes, ignoring other query keys. | Inside `ggui.Derived`, `ggui.Effect`, `ggui.Resource`, or `ggui.Reactive`. |
| `c.Location()` / `r.Location()` | Tracked read of the current `router.Location`. | Inside `ggui.Derived`, `ggui.Effect`, `ggui.Resource`, or `ggui.Reactive`. |
| `c.QueryValue(name)` | `ggui.Readable[string]` backed by the same per-key tracking as `c.Query`. | Passing directly to bindable widgets (`widget.TextOf`, `ggui.Map`, `Bind*`) during one-time setup. |
| `c.LocationValue()` | `ggui.Readable[router.Location]` tracking the current location. | Passing directly to bindable widgets during one-time setup. |

Because a `PageFunc` runs once per mount, calling `c.Query("filter")` at the top
level of `PageFunc` reads only the initial value and never updates. Either read
it inside a reactive computation or bind `c.QueryValue("filter")`:

```go
func searchPage(c *router.Context) ggui.Widget {
	// Bind directly in one-time setup:
	heading := widget.TextOf(c.QueryValue("q"))

	// Or read inside a Derived / Effect / Resource / Reactive:
	summary := ggui.Derived(func() string {
		if q := c.Query("q"); q != "" {
			return "Results for " + q
		}
		return "Type a query to search."
	})

	return widget.Column(heading, widget.TextOf(summary)).Space(1)
}
```

`router.Location` is an immutable, comparable snapshot with fields `Path`
(normalized and escaped), `RawQuery`, `Fragment`, and `Entry` (the history entry
ID), plus helper methods:

- `loc.Query(name) string` — first value for `name`, or `""`.
- `loc.HasQuery(name) bool` — true if `name` appears in the query string, even
  with an empty value (`?draft` or `?draft=`).
- `loc.QueryAll(name) []string` — fresh copy of all values for `name`.
- `loc.String() string` — application-absolute URL (`Path`, optional `?RawQuery`,
  optional `#` with escaped `Fragment`).

## Not-found screens

When no `Page` or `Redirect` matches a URL, the router selects the `NotFound`
registered in the most specific enclosing `Layout` or `Group` prefix that
matches the path, and renders it inside that section's layouts:

```go
r := router.New(
	router.Layout("", appLayout,
		router.Page("/", homePage),
		router.Layout("/admin", adminLayout,
			router.Page("/users", adminUsersPage),
			router.NotFound(adminNotFoundPage), // /admin/* renders inside appLayout + adminLayout
		),
		router.NotFound(notFoundPage), // everything else renders inside appLayout
	),
)
```

A `NotFound` screen receives a `*router.Context` with any parameters matched by
its enclosing section (`c.Param`) and can read `c.LocationValue()` to display
the requested path. Moving between two unmatched URLs in the same `NotFound`
section keeps the same `NotFound` instance mounted and updates its location.

If no enclosing `NotFound` applies (or if a history adapter supplies an
unparsable URL), the router falls back to its built-in `widget.Text("Not found")`
screen outside any layout. Whenever any not-found screen is active,
`r.Current().Get()` is `nil`.

## History and web deployment

### Defaults and `Keep`

Without an explicit `r.History(...)` call:

- **Native builds** default to `router.Memory("/")` (`router/history_default.go`).
- **Browser builds (`js/wasm`)** default to `router.Hash()` (`router/history_js.go`),
  storing the application URL in `location.hash` (for example `/#/settings`) so
  static file hosts need no server-side rewrite rules.

Call `r.Keep("router")` before `r.View()` to keep the current URL across
[`ggui dev`](cli.md#keeping-state-across-a-restart) rebuilds and restarts:

```go
r := router.New(routes...).Keep("router")
```

When `ggui dev` restarts the app, `View` replaces the history's first entry with
the saved URL so the first frame opens on the page you were editing (the
back/forward stack behind it is not persisted). Outside `ggui dev`, `Keep` does
nothing; in a browser, reloading already preserves the URL. Both `History` and
`Keep` panic if called after `View`.

### Browser history (`js/wasm` only)

`Hash()`, `HashHistory`, `Browser(basePath)`, and `BrowserHistory` are compiled
**only under `//go:build js && wasm`** (`router/history_js.go`), while
`Memory`, `MemoryHistory`, `History`, and `Entry` are available on all targets.

If a cross-platform app opts into clean URLs with `router.Browser` on the web,
put that call in a small `js/wasm`-tagged file:

```go
// history_js.go
//go:build js && wasm

package main

import "github.com/ironpark/ggui/router"

func configureHistory(r *router.Router) {
	r.History(router.Browser("/app"))
}
```

```go
// history_other.go
//go:build !(js && wasm)

package main

import "github.com/ironpark/ggui/router"

func configureHistory(*router.Router) {}
```

### Deploying with `Browser(basePath)`

`router.Browser(basePath)` maps application URLs under `basePath` (for example
`"/"` or `"/app"` — passing a relative path or one containing `?` or `#`
panics): `/settings` becomes `/app/settings` in the address bar. Paths outside
`basePath` are kept whole and typically hit a not-found screen.

Because a page reload or pasted link asks the web host for `/app/settings`:

1. Configure your web host with an **SPA fallback** that serves `index.html` for
   every route under `basePath` that does not match a static file.
2. Ensure the entry page loads its assets relative to `basePath`, such as with
   `<base href="/app/">`.
3. Match the base path in [`ggui dev`](cli.md#in-a-browser) and
   [`ggui build --target web`](cli.md#the-web-bundle):

```sh
ggui dev --web --base /app/ ./docs/examples/router
```

Or persist it in `ggui.toml`:

```toml
[web]
base = "/app/"
```

Both `ggui dev --web` and `ggui build --target web` write `<base href="...">`
from `web.base`, and `ggui dev --web` answers HTML route requests with the entry
page automatically.

### Headless testing

Use `router.Memory(initialURL)` with [`ggui.NewProbe`](testing.md) to test full
navigation flows, layouts, redirects, and history without opening a window:

```go
func TestRouter(t *testing.T) {
	t.Parallel()
	r := routes().History(router.Memory("/"))
	p := ggui.NewProbe(r.View(), ggui.Sz(820, 520))
	defer p.Close()
	p.Frame()

	p.Tap("Project 1")
	p.Frame()
	if got := ggui.Untrack(r.Location).String(); got != "/projects/1" {
		t.Fatalf("got %s, want /projects/1", got)
	}

	r.Back()
	p.Frame()
	if got := ggui.Untrack(r.Location).Path; got != "/" {
		t.Fatalf("after Back: %s, want /", got)
	}
}
```

See [docs/examples/router/main_test.go](examples/router/main_test.go) for a
complete headless test of the router example.

## API reference and limitations

### Exported API

```go
// Errors
var ErrNotMounted   error // Navigate/Replace before View mounts or after disposal
var ErrRedirectLoop error // >8 chained navigations without an intervening frame

// Route tree constructors
type PageFunc   func(*Context) ggui.Widget
type LayoutFunc func(c *Context, page ggui.Widget) ggui.Widget
type Params     map[string]string
type Route      interface{ /* unexported */ }

func Page(path string, build PageFunc) *Target
func Layout(path string, build LayoutFunc, children ...Route) Route
func Group(path string, children ...Route) Route
func NotFound(build PageFunc) Route
func Redirect(path, to string) Route

// Router
type Router struct{ /* unexported */ }
func New(routes ...Route) *Router
func (r *Router) History(h History) *Router
func (r *Router) Keep(key string) *Router
func (r *Router) View() ggui.Widget
func (r *Router) Navigate(url string) error
func (r *Router) Replace(url string) error
func (r *Router) Back()
func (r *Router) Forward()
func (r *Router) Location() Location
func (r *Router) CanBack() ggui.Readable[bool]
func (r *Router) CanForward() ggui.Readable[bool]
func (r *Router) Active(path string) ggui.Readable[bool]
func (r *Router) Current() ggui.Readable[*Target]

// Route context
type Context struct{ /* unexported */ }
func (c *Context) Param(name string) string
func (c *Context) Query(name string) string
func (c *Context) Location() Location
func (c *Context) QueryValue(name string) ggui.Readable[string]
func (c *Context) LocationValue() ggui.Readable[Location]
func (c *Context) Navigate(url string) error
func (c *Context) Replace(url string) error
func (c *Context) Back()
func (c *Context) Router() *Router

// Typed page target (also implements Route)
type Target struct{ /* unexported */ }
func (t *Target) URL(params Params) (string, error)
func (t *Target) Navigate(params Params) error
func (t *Target) Active() ggui.Readable[bool]

// Location snapshot
type Location struct {
	Path     string
	RawQuery string
	Fragment string
	Entry    uint64
}
func (l Location) Query(name string) string
func (l Location) HasQuery(name string) bool
func (l Location) QueryAll(name string) []string
func (l Location) String() string

// History adapters (all targets)
type Entry struct {
	URL string
	ID  uint64
}
type History interface {
	Current() Entry
	Push(url string) (Entry, error)
	Replace(url string) (Entry, error)
	Go(delta int)
	Position() (index, length int)
	Subscribe(func(Entry)) (stop func())
}
type MemoryHistory struct{ /* unexported */ }
func Memory(initialURL string) *MemoryHistory

// Browser history adapters (js/wasm only: //go:build js && wasm)
type HashHistory struct{ /* unexported */ }
func Hash() *HashHistory
type BrowserHistory struct{ /* unexported */ }
func Browser(basePath string) *BrowserHistory
```

### Panics

- `router.New` panics on malformed, duplicate, or ambiguous route trees and
  invalid static redirects.
- `r.History` and `r.Keep` panic if called after `r.View`.
- `r.View` panics if mounted in two places at once.
- `r.Active` panics if `path` is not a valid application-absolute URL.
- `router.Browser` (`js/wasm`) panics if `basePath` is not a valid path without
  query or fragment.

### Intentionally omitted

The router stays small and explicit. The following are intentionally left out:

- **Navigation guards and leave blockers** — check conditions in your navigation
  handlers, page setup (`c.Replace`), or dialogs instead.
- **Built-in route transitions** — wrap content inside your layout or page with
  [`widget.Transition`](animation.md) if you want animated transitions.
- **Keep-alive caching of unmounted pages** — unmounted pages dispose cleanly so
  resources and subscriptions stop; hoist state into a parent layout or shared
  signal when it should outlive a page.
- **Automatic scroll restoration** — manage scroll positions with a
  `widget.ScrollController` stored in a layout or signal if needed.
- **File-system routing** — routes are declared explicitly in Go.
