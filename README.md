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
of them settles within a single frame.

**Ownership** — an `Effect` or `Derived` created while another effect runs
belongs to it and is disposed when the owner re-runs or is disposed, so
nothing piles up across rebuilds. Subscriptions are collected afresh on every
run, so an effect follows only what it read last time. `OnCleanup(fn)` runs
before the enclosing effect re-runs and when it is disposed; `Untrack(fn)` and
`Peek()` read without subscribing.

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

**Builders and components** — a `Builder` is a `func() ggui.Widget`. It runs
inside an effect, so the tree it returns is rebuilt when the signals it read
change. That is the whole of it for a small app. To rebuild less, give a
subtree its own boundary:

```go
func button(label string, onTap func()) ggui.Widget {
	return ggui.Component(func() ggui.Builder {
		hovered := ggui.State(false) // setup: runs once, owns local state
		return func() ggui.Widget {  // build: re-runs when hovered changes
			return ggui.Pointer(ggui.Text(label)).OnTap(onTap).OnHover(hovered.Set)
		}
	})
}
```

**Keyed lists** — `For(items, key, build)` watches a `Reader[[]T]` and keeps
one child per key, so reordering or editing the list reuses the children and
whatever state they hold. Each child receives its item as a `*Signal[T]` that
`For` writes on every change; read it reactively:

```go
ggui.Scroll(ggui.For(todos, func(t Todo) int { return t.ID },
	func(t *ggui.Signal[Todo]) ggui.Widget {
		return ggui.Reactive(func() ggui.Widget { return ggui.Text(t.Get().Title) })
	}).Gap(4))
```

`List(items, build)` is the plain version for a slice you have in hand: one
child per item, rebuilt with the parent. `Root(fn)` is what `For` uses per key,
an owner that never re-runs, for containers of your own that keep children
alive across their own updates.

`Component(setup)` runs setup once, untracked, and the `Builder` it returns in
an effect of its own. `Reactive(build)` is the same without setup: an island
that rebuilds when its signals change while the parent stays put. A component
lives as long as its parent's tree keeps it, so keep a parent's `Builder` free
of signal reads and put the parts that change in islands; then the components
it holds, and their state, survive.

**Widgets** — a `Widget` is asked for a size under `Constraints` and an `Env`
of inherited values, then asked to paint into the `Rect` its parent assigned
on a `Canvas`. Constraints flow down, sizes flow up.
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
`Stack`, `Align`, `Center`, `Scroll`, `Pointer`/`Tap`, `Focus`, `For`, and `List` —
a column built from your own slice:

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
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Style(ts)`, `.Align(0.5)` and
`.NoWrap()` adjust it; what is not set is inherited (see Styling). The
built-in font is Go Regular; `LoadFont(ttf)` or `LoadFontFile(path)` load your
own, and `SetDefaultFont` makes one the default.

## Styling

Three layers, each a plain value.

**Styles are values.** `TextStyle{Font, Size, Color, LineHeight}` is what
`Text`'s setters write into; a zero field means "inherit". `a.Merge(b)` lays
the set fields of `b` over `a`, so a heading is `Text(s).Style(t.Title)` and a
one-off tweak is `Text(s).Style(t.Title).Color(red)`.

**Styles inherit through the tree.** Every widget lays out under an `Env`
that flows down from the root, like CSS inheritance. `Styled(child)` sets the
text style everything below starts from, and a `Text`'s own setters still win:

```go
ggui.Styled(ggui.Column(ggui.Text("a"), ggui.Text("b").Size(18))).Color(t.Muted).Size(12)
```

The root `Env` starts from the theme's `Text`, so a bare `Text(s)` already
looks right. Inheritance happens at layout time, so it works with the eager
construction of Go: no closures around subtrees. Your own inherited values go
the same way: `Provide(key, v, child)` stores `v` under a `Key[T]` from
`NewKey`, and a widget reads it back with `env.Get(key)` in `Layout`.

**Tokens live in a theme.** `Theme` holds colors (`Fg`, `Bg`, `Surface`,
`Accent`, `AccentHover`, `Muted`), named text styles (`Text`, `Title`) and
sizes (`Radius`, `Space`). `UseTheme()` reads it at build time and subscribes
the enclosing Builder; `SetTheme(t)` swaps it and rebuilds only what read it.
`DefaultTheme()` is light, `DarkTheme()` dark, and a window with no
`Background` follows the theme's `Bg`. State-dependent looks are a signal in a
`Component`: the button in `examples/counter` picks `Accent` or `AccentHover`
from a `hovered` signal. `Box` decorates with `.Fill`, `.Radius(r)` and
`.Border(w, c)`.

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

`Scroll(child)` gives its child `Unbounded` height (or width, with
`.Horizontal()`), shows a window onto it, moves that window with the wheel and
clips both drawing and hit regions to the window. `.Offset(sig)` binds the
position to a `Signal[float64]` for programmatic scrolling or to keep it across
rebuilds; `.Speed(px)` and `.Bar(color)` tune it. Widgets that fill their space
fall back to their content size on an unbounded axis, so `Center`, `Expanded`
and `.Justify` inside a `Scroll` do not blow up.

`Pointer` has `OnTap`, `OnDown`, `OnUp`, `OnMove`, `OnEnter`, `OnExit`,
`OnHover` and `OnScroll`; `Tap(child, fn)` is the one-callback shortcut.
A tap is a press and a release inside the same region, matched by `Rect`, so
a tree rebuilt in between still completes it. `Focus(child)` takes keyboard
focus when clicked and delivers `OnKey`, `OnText` and `OnFocus`. Global
shortcuts still go in `App.OnFrame`.

To make your own widget interactive, implement `PointerHandler` or
`KeyHandler` and call `dst.HitPointer(r, w)` or `dst.HitKey(r, w)` from
`Paint`. `dst.Clip(r)` returns a Canvas that draws and registers regions only
inside `r`.

## HiDPI

Widgets work in logical pixels; the screen is allocated at the monitor's
device scale factor so a Retina display gets a sharp image. `dst.Scale()`
returns the factor, and drawing goes through it: `dst.FillRect(r, c)` fills a
logical `Rect`, `dst.Px(v)` converts a length, `dst.Geo(at)` is the transform
for `DrawImageOptions`, and text rasterizes its face at the scaled size rather
than scaling the pixels. A custom widget that draws with Ebitengine directly
should do the same.

**Custom widgets** — implement `Layout` and `Paint`. `Layout` receives the
`Env` to pass on to children unchanged; `Paint` receives the `Rect` to draw
in, so a leaf widget stores nothing between the two calls and a container
remembers only where its children go. `FromFuncs` wraps two closures
when a named type is overkill:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ggui.Canvas, r ggui.Rect) {
		vector.DrawFilledCircle(dst.Image, dst.Px(r.Origin.X+4), dst.Px(r.Origin.Y+4), dst.Px(4), fg, true)
	},
)
```

## Layout

```
├── app.go        App runtime: window setup, frame loop, ebiten.Game
├── signal.go     Reactivity: Signal, Memo, Effect, dependency tracking
├── widget.go     Widget interface, Builder, Component/Reactive, Children
├── widgets.go    Built-in widgets
├── style.go      TextStyle, Env, Key, Theme
├── for.go        For: keyed, reactive list
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
