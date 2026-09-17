# ggui

A cross-platform GUI framework for Go — Flutter's widget tree and layout model,
driven by SvelteKit-style reactivity, rendered with [Ebitengine](https://ebiten.org).

> Early scaffolding. The APIs below work, but everything is subject to change.

## Install

```sh
go get github.com/ironpark/ggui
```

Requires Go 1.27+ (the reactive API uses generic methods). Ebitengine needs the
usual platform toolchains (Xcode command line tools on macOS, X11/ALSA dev
headers on Linux).

## Hello, counter

```go
count := ggui.State(0)
label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })

app := ggui.New(ggui.Config{Title: "counter", Width: 480, Height: 320}, func() ggui.Widget {
	return ggui.Center(ggui.Text(label.Get()))
})

app.OnFrame(func() {
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		count.Update(func(n int) int { return n + 1 })
	}
})

app.Run()
```

Run it: `make run` (or `go run ./examples/counter`).

## Concepts

**Signals** — `State(v)` creates a `Signal[T]`, a reactive cell; `T` is
inferred from `v`. Reads made inside an `Effect` are
tracked automatically, so a `Set` invalidates exactly the computations that
depend on it. No subscriptions to declare, no manual invalidation. Writing a
value equal to the current one notifies no one (`WithEqual` supplies equality
for types `==` cannot handle, like slices).

**Derived values** — `Derived(fn)` caches a computed value and refreshes it when
one of the signals `fn` read changes. `Signal.Map` is the common case, and
`Combine` folds two sources into one:

```go
total := ggui.Combine(price, qty, func(p float64, n int) float64 { return p * float64(n) })
pretty := total.Map(func(v float64) string { return fmt.Sprintf("$%.2f", v) })
```

A `Memo` notifies its readers only when the result actually differs, and a chain
of them settles within a single frame. Create memos once, next to the signals
they derive from — not inside a `Builder`, which runs again on every rebuild.

**Cells and fields** — `Signal.Field` hands one field of a value out as its
own read-write cell, so a single state struct can be passed to components
field by field. Pick the field by pointer; writes copy the struct, assign
through the pointer and store the copy, so watchers of the whole value still
fire:

```go
state := ggui.State(model{})
count := state.Field(func(m *model) *int { return &m.Count })
ggui.Add(count, 1) // writes back through state
```

Fields nest (`state.Field(...).Field(...)`), and `Signal.Lens(get, set)` covers
the rare projection that is computed rather than stored. `Toggle` and `Add`
are the two one-line updates that come up constantly.

`*Signal[T]` and `Lens[T, U]` are both `Cell[T]` (`Get`, `Set`, `Update`), so
a component can take a `Cell[int]` without caring where it came from. All of
them plus `*Memo[T]` satisfy `Reader[T]`, so `Watch`, `Combine` and your own
helpers accept any of them.

**Builders** — a component is a `func() ggui.Widget`. The runtime runs it inside
an effect, so the tree rebuilds when the signals it read change.

**Widgets** — a `Widget` is asked for a size under `Constraints`, then asked to
paint into the `Rect` its parent assigned. Constraints flow down, sizes flow up.
Every built-in follows one shape: a constructor takes what the widget cannot
do without, chainable setters take the rest, so the code reads as the tree it
builds:

```go
ggui.Center(
	ggui.Box(
		ggui.Column(
			ggui.Text(title).Color(fg),
			ggui.Text(body).Color(dim),
		).Gap(8),
	).Pad(24).Fill(panel),
)
```

Current set: `Text`, `Box`, `Padding`, `Column`, `Row`, `Stack`, `Align`,
`Center`, and `List` — a column built from your own slice:

```go
ggui.List(rows, func(r Row) ggui.Widget { return ggui.Text(r.Title) }).Gap(4)
```

`Children(items, build)` does the same thing for any widget that takes
children: `ggui.Row(ggui.Children(rows, build)...)`. Padding takes CSS
shorthand, on its own or on a `Box`: `Padding(w, 8)`, `Padding(w, 4, 12)`,
`Box(w).Pad(1, 2, 3, 4)`. `Stack` layers children at the top-left corner, sized
to the largest one unless `.Expand()` is set. `Align` fills its space and
places the child by fraction, `Align(w).At(0.25, 1)` or with the edge setters,
`Align(w).Bottom().Right()`; `Center` is `Align` at (0.5, 0.5). Geometry constructors
are numeric-generic: `ggui.Sz(w, h)` and `ggui.Pt(x, y)` take ints or floats
without a cast.

**Custom widgets** — implement `Layout` and `Paint`. `Paint` receives the
`Rect` to draw in, so a leaf widget stores nothing between the two calls; a
container remembers only where its children go. `FromFuncs` wraps two closures
when a named type is overkill:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ebiten.Image, r ggui.Rect) {
		vector.DrawFilledCircle(dst, float32(r.Origin.X+4), float32(r.Origin.Y+4), 4, fg, true)
	},
)
```

## Layout

```
├── app.go        App runtime: window setup, frame loop, ebiten.Game
├── signal.go     Reactivity: Signal, Memo, Lens/Field, Effect, dependency tracking
├── widget.go     Widget interface, Builder, Children
├── widgets.go    Built-in widgets
├── geometry.go   Point, Size, Rect, Constraints
└── examples/     Runnable apps
```

## Development

```sh
make        # fmt + vet + test
make run    # the counter example
```
