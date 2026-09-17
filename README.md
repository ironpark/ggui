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

**State as a struct of signals** — keep one signal per piece of state and
group them in a plain struct. Each field is its own reactive cell, so a change
to one re-runs only what read it, and components take exactly the signals they
need:

```go
type model struct {
	Count *ggui.Signal[int]
	Step  *ggui.Signal[int]
}
state := model{Count: ggui.State(0), Step: ggui.State(1)}
ggui.Add(state.Count, state.Step.Get())
```

`Toggle` and `Add` are the two one-line updates that come up constantly.
`*Signal[T]` and `*Memo[T]` both satisfy `Reader[T]`, so `Watch`, `Combine`
and your own helpers accept either.

**Builders** — a component is a `func() ggui.Widget`. The runtime runs it inside
an effect, so the tree rebuilds when the signals it read change.

**Widgets** — a `Widget` is asked for a size under `Constraints`, then asked to
paint into the `Rect` its parent assigned on a `Canvas`. Constraints flow down, sizes flow up.
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

Current set: `Text`, `Box`, `Padding`, `Column`, `Row`, `Flex`/`Expanded`/`Spacer`,
`Stack`, `Align`, `Center`, `Pointer`/`Tap`, `Focus`, and `List` — a column built
from your own slice:

```go
ggui.List(rows, func(r Row) ggui.Widget { return ggui.Text(r.Title) }).Gap(4)
```

`Children(items, build)` does the same thing for any widget that takes
children: `ggui.Row(ggui.Children(rows, build)...)`. Padding takes CSS
shorthand, on its own or on a `Box`: `Padding(w, 8)`, `Padding(w, 4, 12)`,
`Box(w).Pad(1, 2, 3, 4)`. `Stack` layers children at the top-left corner, sized
to the largest one unless `.Expand()` is set. `Align` fills its space and
places the child by fraction, `Align(w).At(0.25, 1)` or with the edge setters,
`Align(w).Bottom().Right()`; `Center` is `Align` at (0.5, 0.5). Geometry
constructors are numeric-generic: `ggui.Sz(w, h)` and `ggui.Pt(x, y)` take
ints or floats without a cast.

**Row and Column** hug their children by default. `Expanded(child)` and
`Flex(child, weight)` share whatever main-axis space the rigid children leave,
`Spacer()` is an empty `Expanded`, and `.Justify(...)` distributes slack
(`JustifyCenter`, `JustifyEnd`, `SpaceBetween`, `SpaceAround`, `SpaceEvenly`).
Any of those makes the widget fill its main axis. `.Align(...)` places
children across the axis (`AlignCenter`, `AlignEnd`, `AlignStretch`).

```go
ggui.Row(ggui.Text("Title"), ggui.Spacer(), ggui.Text("3 items")).Align(ggui.AlignCenter)
```

**Text** wraps at spaces to the width it is given, and between runes when a
word is wider than the line, so scripts without spaces wrap too. `.Size(px)`,
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Align(0.5)` and `.NoWrap()`
adjust it. The built-in font is Go Regular; `LoadFont(ttf)` or
`LoadFontFile(path)` load your own, and `SetDefaultFont` makes one the default.

## Input

Interactive widgets register the `Rect` they painted as a hit region on the
`Canvas`. Each frame the runtime routes the pointer to the topmost region
under it, and a region that does not handle an event lets it fall through to
the one beneath, so a tap target does not block scrolling.

```go
hovered := ggui.State(false)
button := ggui.Pointer(ggui.Box(ggui.Text("+")).Pad(6, 16)).
	OnTap(func() { ggui.Add(count, 1) }).
	OnHover(hovered.Set)
```

`Pointer` has `OnTap`, `OnDown`, `OnUp`, `OnMove`, `OnEnter`, `OnExit`,
`OnHover` and `OnScroll`; `Tap(child, fn)` is the one-callback shortcut.
A tap is a press and a release inside the same region, matched by `Rect`, so
a tree rebuilt in between still completes it. `Focus(child)` takes keyboard
focus when clicked and delivers `OnKey`, `OnText` and `OnFocus`. Global
shortcuts still go in `App.OnFrame`.

To make your own widget interactive, implement `PointerHandler` or
`KeyHandler` and call `dst.Hit(r, w)` from `Paint`.

**Custom widgets** — implement `Layout` and `Paint`. `Paint` receives the
`Rect` to draw in, so a leaf widget stores nothing between the two calls; a
container remembers only where its children go. `FromFuncs` wraps two closures
when a named type is overkill:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ggui.Canvas, r ggui.Rect) {
		vector.DrawFilledCircle(dst.Image, float32(r.Origin.X+4), float32(r.Origin.Y+4), 4, fg, true)
	},
)
```

## Layout

```
├── app.go        App runtime: window setup, frame loop, ebiten.Game
├── signal.go     Reactivity: Signal, Memo, Effect, dependency tracking
├── widget.go     Widget interface, Builder, Children
├── widgets.go    Built-in widgets
├── canvas.go     Canvas: paint target plus the frame's hit regions
├── input.go      Pointer and keyboard events, Pointer/Tap/Focus widgets
├── font.go       Font loading, default font, text wrapping
├── geometry.go   Point, Size, Rect, Constraints
└── examples/     Runnable apps
```

## Development

```sh
make        # fmt + vet + test
make run    # the counter example
```
