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
count := ggui.NewSignal(0)
label := count.Map(func(n int) string { return fmt.Sprintf("count: %d", n) })

app := ggui.New(ggui.Config{Title: "counter", Width: 480, Height: 320}, func() ggui.Widget {
	return &ggui.Center{Child: &ggui.Text{Value: label.Get()}}
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

**Signals** — `Signal[T]` is a reactive cell. Reads made inside an `Effect` are
tracked automatically, so a `Set` invalidates exactly the computations that
depend on it. No subscriptions to declare, no manual invalidation. Writing a
value equal to the current one notifies no one (`WithEqual` supplies equality
for types `==` cannot handle, like slices).

**Derived values** — `Derive(fn)` caches a computed value and refreshes it when
one of the signals `fn` read changes. `Signal.Map` is the common case, and
`Combine` folds two sources into one:

```go
total := ggui.Combine(price, qty, func(p float64, n int) float64 { return p * float64(n) })
pretty := total.Map(func(v float64) string { return fmt.Sprintf("$%.2f", v) })
```

A `Memo` notifies its readers only when the result actually differs, and a chain
of them settles within a single frame. Create memos once, next to the signals
they derive from — not inside a `Builder`, which runs again on every rebuild.

**Lenses** — `Signal.Lens` projects part of a value into its own read-write
cell, so a single state struct can be handed to components field by field:

```go
state := ggui.NewSignal(model{})
count := state.Lens(
	func(m model) int { return m.Count },
	func(m model, n int) model { m.Count = n; return m },
)
count.Update(func(n int) int { return n + 1 }) // writes back through state
```

`*Signal[T]`, `*Memo[T]` and `Lens[T, U]` all satisfy `Reader[T]`, so `Watch`,
`Combine` and your own helpers accept any of them.

**Builders** — a component is a `func() ggui.Widget`. The runtime runs it inside
an effect, so the tree rebuilds when the signals it read change.

**Widgets** — a `Widget` is asked for a size under `Constraints`, then asked to
paint at a `Point`. Constraints flow down, sizes flow up. Current set: `Box`,
`Text`, `Column`, `Center`, and `List[T]` — a column built from your own slice:

```go
&ggui.List[Row]{Gap: 4, Items: rows, Item: func(r Row) ggui.Widget {
	return &ggui.Text{Value: r.Title}
}}
```

`Children(items, build)` does the same thing for any widget that takes a
`[]Widget`. Geometry constructors are numeric-generic: `ggui.Sz(w, h)` and
`ggui.Pt(x, y)` take ints or floats without a cast.

## Layout

```
├── app.go        App runtime: window setup, frame loop, ebiten.Game
├── signal.go     Reactivity: Signal, Memo, Lens, Effect, dependency tracking
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
