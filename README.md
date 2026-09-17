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

app := ggui.New(ggui.Config{Title: "counter", Width: 480, Height: 320}, func() ggui.Widget {
	return ggui.Center(ggui.Textf("count: %d", count))
})

app.OnKey(func(ev ggui.KeyEvent) bool {
	if ev.Key == ebiten.KeySpace {
		ggui.Add(count, 1)
		return true
	}
	return false
})

app.Run()
```

Run it: `make run` (or `go run ./examples/counter`). `make run-todo` runs a
fuller app: a text field with IME input, a keyed list, controls bound to
signals, a tweened progress bar and a theme switch. `make run-gallery` shows
every layout widget and control on one page.

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

`Toggle`, `Add`, `Append` and `Remove` are the one-line updates that come up
constantly; `Remove(items, func(t T) bool)` drops what matches into a new
slice and notifies only if something went. `BindTheme(dark, on, off)`
follows a boolean signal with the theme.
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

A child is built the first time it is laid out. `.ItemExtent(h)` fixes every
child's height (width, with `.Horizontal()`), and then inside a `Scroll` only
the rows in view are built, laid out and painted, so a list of tens of
thousands of items costs what the visible ones do. `List(items, build)` is
the plain version for a slice you have in hand: one child per item, rebuilt
with the parent. `Root(fn)` is what `For` uses per key,
an owner that never re-runs, for containers of your own that keep children
alive across their own updates.

`Component(setup)` runs setup once, at the component's first layout, and
the `Builder` it returns in an effect of its own. A parent rebuild makes a
new one; `Keyed(key, setup)` survives that: the enclosing builder keeps one
instance per key across its runs, and `Mount(key, props, setup)` hands the
props to setup as a `Signal` it writes on every rebuild. `Reactive(build)` is the same without setup: an island
that rebuilds when its signals change while the parent stays put.
`View(reader, build)` is the island for one value, with the widget type
inferred from the constructor (`ggui.View(name, ggui.Text)`), and
`When(cond, then, else)` picks between two widgets built once. For text,
`TextOf(reader)` and `Textf("%d left", count)` are real `Text` widgets that
follow their signals, so every setter still chains; `Sprintf` is the `Memo`
underneath. A component
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
`Wrap`, `Grid`, `Stack`, `Align`, `Center`, `Scroll`, `Image`, `TextInput`,
`Tooltip`, `Popup`, `Transition`/`Presence`, `Cached`, `Pointer`/`Tap`,
`Focus`, `For` and `List`; the themed controls live in the `ui` package (see
Controls).
`List` is a column built from your own slice:

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
children across the axis (`AlignStart`, `AlignCenter`, `AlignEnd`,
`AlignStretch`); a Row centers by default, a Column starts at the left.

```go
ggui.Row(ggui.Text("Title"), ggui.Spacer(), ggui.Text("3 items")) // centered on its height
ggui.Column(ggui.Text("a"), ggui.Text("b")).Align(ggui.AlignStretch)
```

**Wrap and Grid** cover the two other common arrangements. `Wrap` flows
children left to right and starts a new line where the next one would not
fit, for tags and toolbars; `.Gap(v)` spaces both axes, `.RunGap(v)` the
lines alone, `.Align(...)` places children within their line. `Grid(cols,
...)` deals children into equal-width columns, each given its cell width
tight so columns line up, with rows as tall as their tallest cell.

**Tooltip(child, text)** shows text below the child once the cursor has
rested on it for half a second (`.Delay(d)`). It registers no hit region, so
the child gets every event, and it paints through `Canvas.Overlay`, above
everything else.

**Popup(anchor, content)** floats content below its anchor (above it when
there is no room), painted through `Canvas.Overlay` over a scrim: a press
anywhere outside closes it and reaches nothing underneath. `Show`, `Hide`,
`Toggle` and `IsOpen` drive it, or `.Bind(sig)` keeps the state in a
`Signal[bool]`; `.Keys(h)` keeps keyboard focus on the widget that opened it
while the pointer is in the content, and a widget inside can find its popup
with `PopupOf(env)` to close it after acting. `ui.Select` and `ui.Menu` are
built on it.

**Image(img)** draws an `*ebiten.Image` at its natural size, shrinking to
the room it gets with its aspect ratio kept; `.Size`, `.Width` or `.Height`
fix it, and `.Fit(FitContain | FitCover | FitFill | FitNone)` says how it
sits in a box of another shape. `DecodeImage(bytes)` and
`LoadImageFile(path)` read PNG, JPEG and GIF.

**Text** wraps at spaces to the width it is given, and between runes when a
word is wider than the line, so scripts without spaces wrap too. `.Size(px)`,
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Style(ts)`, `.Align(0.5)` and
`.NoWrap()` adjust it; what is not set is inherited (see Styling). The
built-in font is Go Regular; `LoadFont(ttf)` or `LoadFontFile(path)` load your
own, and `SetDefaultFont` makes one the default.

## Controls

The ready-made interactive widgets live in `github.com/ironpark/ggui/ui`,
apart from the core the way Flutter's `material` sits on `widgets`: the core
is the layout, reactivity and input machinery, `ui` is one opinionated set
built on its public API, and a set of your own can be built the same way.
Each control binds to a signal the way Svelte's `bind:` does: the control
writes it, and writing it moves the control.

```go
name := ggui.State("")
agree := ggui.State(false)
size := ggui.State(0.5)
plan := ggui.State("free")

ggui.Column(
	ui.TextField(name).Placeholder("Your name").OnSubmit(save),
	ui.Checkbox(agree, "I agree"),
	ui.Switch(dark, "Dark mode"),
	ui.Slider(size, 0, 1).Step(0.1),
	ui.Radios(plan, []string{"free", "pro"}).Label(strings.ToTitle), // or one ui.Radio(plan, value, label) at a time
	ggui.Row(ui.Button("Save", save), ui.Button("Cancel", cancel).Secondary()).Gap(8),
	ui.Divider(),
).Gap(12)
```

`ui.Button(label, onTap)` is the primary button; `.Secondary()` quiets it,
`.Disabled(v)` greys it out, `.Pad(...)` overrides the theme's padding,
`ui.ButtonOf(child, onTap)` wraps any content. Every control has
`.Disabled(v)`, and every control that changes a signal has `.OnChange(fn)`
for the value the user picked. They take their colors and padding from the
theme in their `Env` at layout time, keep hover and press state in the
widget itself, and read their signal in `Paint`, so nothing rebuilds for a
hover or a tick. Buttons and text fields set the mouse cursor. Anything
that eases, such as a switch knob or a tab underline, keeps its `Motion` in
the Canvas with `Retain`, so it keeps moving through a rebuild; animations
read the clock through `ggui.Now()`, which `ggui.SetClock` replaces in
tests.

**Text input.** `ui.TextField(value)` is an editor in a themed box;
`ggui.TextInput(value)` is the bare editor for a box of your own, and part
of the core because it is a primitive like `Text`. Either is one line that
scrolls sideways until `.Multiline()` (or `.Lines(n)`, the fewest lines it
is tall) makes it wrap at its width and grow by the line, with Up and Down
between lines, Home and End within one, Enter for a line break and
⌘/Ctrl+Enter for `OnSubmit`. Text arrives
through the platform IME (Ebitengine's `exp/textinput`), so composed scripts
such as Korean and Japanese work, with the composition shown underlined in
place. Arrows move (Shift selects, Alt or Ctrl jumps words, ⌘ on macOS reaches
the ends), Home/End, Backspace/Delete, ⌘/Ctrl+A, C, X and V through the system
clipboard, Enter fires `.OnSubmit`. Click places the caret, drag selects,
double-click selects a word, triple-click everything. `.Placeholder(s)`,
`.Password()`, `.OnChange(fn)` and `.MinWidth(w)` tune it. The editor keeps
its caret and selection across a rebuild of the tree, and `.Input()` on a
`ui.TextField` reaches the editor for `Focused()`.

The built-in font covers Latin, Greek and Cyrillic. Glyphs a font lacks are
drawn from its fallbacks: `SystemFonts()`, the CJK and wide-coverage fonts
found at well-known paths on macOS, Windows and Linux (plus any files in
the `GGUI_FONTS` environment variable), so Korean, Japanese and Chinese
render out of the box on a machine that has such a font. `Fallback(fonts...)`
on a `Font` chooses a chain of your own and `NoFallback()` turns it off.
`LoadFontFile` also reads the first face of a `.ttc`; `LoadFontCollection`
returns them all.

**Tabs, Collapsible, Card, Badge, Progress.** `ui.Tabs(selected,
ui.Tab("One", page), ...)` shows the page whose index the signal holds,
with an animated underline and Left/Right while focused; only that page is
laid out. `ui.Collapsible(open, "Title", content)` folds content under a
clickable header, animating it with `Presence`. `ui.Card(child)` is a
Surface panel with border, radius and padding; `ui.Badge("new")` a small
pill (`.Accent()`); `ui.Progress(value)` a bar that eases to a fraction
read from a `Reader[float64]` every frame.

**Select and Menu.** `ui.Select(value, options)` is a dropdown bound to a
signal, labelled through `fmt.Sprint` or `.Label(fn)`: a click
or Space opens the list in a `Popup`, the arrow keys move through it (or
step the value while it is closed), Enter picks, Escape closes.
`ui.Menu("File", ui.MenuItem("New", fn), ui.MenuDivider(), ...)` is a
secondary button that opens a list of actions the same way; an item runs
its function and closes the menu.

## Animation

`Tween(v, d)` and `Spring(v)` are values that move toward their target over
time instead of jumping, after Svelte's `tweened` and `spring`. Both are
`Reader`s, so an island that reads one rebuilds every frame the value moves:

```go
width := ggui.Tween(0.0, 200*time.Millisecond).Easing(ggui.EaseOut)
bar := ggui.Reactive(func() ggui.Widget { return ggui.Box().Size(width.Get(), 4).Fill(accent) })
width.Set(120) // slides there over 200ms; Jump(v) skips the motion
```

A tween restarts from wherever it is when retargeted; a spring keeps its
momentum, overshoots a little and settles (`.Stiffness`, `.Damping`). Easings:
`EaseLinear`, `EaseIn`, `EaseOut`, `EaseInOut`. The runtime steps running
animations once per frame, before effects are flushed. For a look that moves
inside one widget, `Motion` is the same tween driven from `Paint` with the
current time and no signal; `ui.Switch` slides its knob with one.

**Transitions.** `Transition(child).Fade().Slide(dx, dy).Scale(from)` plays
an enter animation when the child first appears, over `.Duration(d)` with
`.Easing(e)`. Whether it is new is judged against the previous frame by Rect
(or `.Key(k)`), so a Builder that rebuilds every frame does not restart it.
`Presence(show, child)` keeps the child on screen when `show` turns false,
inert to input, and runs the same animation backwards before removing it:

```go
ggui.Presence(open, ggui.Transition(panel).Slide(0, -8).Fade())
```

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
looks right; `Title(s)` and `Caption(s)` take the theme's named styles from
the Env at layout, `.Space(n)` on Column, Row, Wrap, Grid and For is n times
the theme's `Space`, and `Themed(t, child)` gives a subtree its own theme,
so a Builder rarely needs `UseTheme` at all. Inheritance happens at layout time, so it works with the eager
construction of Go: no closures around subtrees. Your own inherited values go
the same way: `Provide(key, v, child)` stores `v` under a `Key[T]` from
`NewKey`, and a widget reads it back with `env.Get(key)` in `Layout`.

**Tokens live in a theme.** `Theme` holds colors (`Fg`, `Bg`, `Surface`,
`Field`, `Border`, `Accent`, `AccentHover`, `OnAccent`, `Selection`, `Muted`),
named text styles (`Text`, `Title`, `Caption`), sizes (`Radius`, `Space`)
and the paddings the controls use (`ButtonPad`, `FieldPad`, `ItemPad`,
`CardPad`, `PanelPad`), so a custom control can match the built-in ones.
`UseTheme()` reads it at build time and subscribes the enclosing Builder;
`SetTheme(t)` swaps it and rebuilds only what read it. `DefaultTheme()` is
light, `DarkTheme()` dark, and a window with no `Background` follows the
theme's `Bg`. The theme also travels in the `Env`, where `env.Theme()` gives a
custom widget the tokens at layout time, the way the built-in controls get
theirs. `Box` decorates with `.Fill`, `.Radius(r)` and `.Border(w, c)`.

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
clips both drawing and hit regions to the window. The offset carries across
a rebuild; `.Offset(sig)` binds it to a `Signal[float64]` for programmatic
scrolling; `.Speed(px)` and `.Bar(color)` tune it. Widgets that fill their space
fall back to their content size on an unbounded axis, so `Center`, `Expanded`
and `.Justify` inside a `Scroll` do not blow up.

`Pointer` has `OnTap`, `OnDown`, `OnUp`, `OnMove`, `OnDrag`, `OnEnter`,
`OnExit`, `OnHover` and `OnScroll`, and `.Cursor(shape)` sets the mouse
cursor over it; `Tap(child, fn)` is the one-callback shortcut. A tap is a
press and a release inside the same region, matched by `Rect`, so a tree
rebuilt in between still completes it. The region that took a press captures
the pointer: it gets `OnDrag` every frame until the release, wherever the
cursor went, which is what a slider or a text selection needs. `Focus(child)`
takes keyboard focus when clicked and delivers `OnKey`, `OnText` and
`OnFocus`; a held key repeats, and `KeyEvent.Mods` carries Shift, Ctrl, Alt
and Meta (`Mods.Cmd()` is ⌘ on macOS and Ctrl elsewhere). Tab and Shift+Tab
move focus through the key regions in paint order; focus that arrived that
way is reported with `Key` set to `KeyTab`, which is when the controls draw
a focus ring. Buttons press on Space or Enter, toggles flip, sliders step
with the arrows. Global shortcuts go in `App.OnKey`, which sees every key
press before the focused widget and keeps the ones it returns true for.

To make your own widget interactive, implement `PointerHandler` or
`KeyHandler` and call `dst.HitPointer(r, w)`, `dst.HitKey(r, w)` or
`dst.HitCursor(r, shape)` from `Paint`; calls for the same `Rect` merge into
one region. A `KeyHandler` that also implements `TickHandler` runs once per
frame while focused, which is how `TextInput` drives the IME. `dst.Clip(r)`
returns a Canvas that draws and registers regions only inside `r`,
`dst.Pointer()` is where the cursor is, and `dst.Overlay(fn)` paints above
the tree once it is done.

A rebuild replaces widgets, and with them the state they hold. A handler that
implements `Adopter` is handed the handler that held the same region in the
previous frame as it registers its own, so it can copy hover, press, a caret
or an animation in flight: the built-in controls, `TextInput`, `Scroll` and
`Popup` all do, which is why flipping a `ui.Switch` that rebuilds its own
subtree still slides the knob. The region is matched by the handler's
identity when it implements `Identified`, else by `Rect`; `.Key(k)` on a
control, `Scroll` or `Popup` sets one, so a widget rebuilt and moved in the
same frame keeps its state. `ggui.Interactive` is the shared body of a
control: embed it, call `Hit` from `Paint` and `Pointer` and `Keyboard` from
the handlers, and hover, press, focus, the focus ring and adoption come with
it; `ui` is built on it.

**Testing** needs no window: `NewProbe(w, size)` runs the runtime's frame
steps headlessly, and `Click`, `Press`, `Move`, `Release`, `Scroll`, `Type`
and `Text` route input through the same hit regions and focus as the app.

```go
p := ggui.NewProbe(ui.Checkbox(on, "x"), ggui.Sz(200, 30))
p.Click(ggui.Pt(5, 5)) // on.Peek() is now true
```

## HiDPI

Widgets work in logical pixels; the screen is allocated at the monitor's
device scale factor so a Retina display gets a sharp image. `dst.Scale()`
returns the factor, and drawing goes through it: `dst.FillRect`,
`dst.FillRoundRect`, `dst.StrokeRoundRect`, `dst.FillCircle` and
`dst.StrokeLine` take logical geometry, `dst.Px(v)` converts a length,
`dst.Geo(at)` is the transform for `DrawImageOptions`, and text rasterizes
its face at the scaled size rather than scaling the pixels. A custom widget
that draws with Ebitengine directly should do the same.

**Custom widgets** — implement `Layout` and `Paint`. `Layout` receives the
`Env` to pass on to children unchanged; `Paint` receives the `Rect` to draw
in, so a leaf widget stores nothing between the two calls and a container
remembers only where its children go. A container paints its children with
`dst.Paint(child, r)` rather than `child.Paint(dst, r)`, so the inspector
sees them. `FromFuncs` wraps two closures when a named type is overkill:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ggui.Canvas, r ggui.Rect) {
		vector.DrawFilledCircle(dst.Image, dst.Px(r.Origin.X+4), dst.Px(r.Origin.Y+4), dst.Px(4), fg, true)
	},
)
```

## Frames

Every frame the runtime routes input, steps animations, flushes effects and
paints. It lays the tree out only when something could have moved: the root
was rebuilt, the window changed size, a `Signal` was written, or
`RequestLayout()` was called. Hover and press live outside signals and only
change how a widget paints, so a still frame costs a paint and nothing
else. A custom widget that keeps size-affecting state outside signals calls
`Invalidate(env)` when that state changes; `Scroll` does for its offset. A
`Scroll` also tells its subtree the window it shows through the `Env`
(`ScrollViewport(env)`), which is how `For` virtualizes.

`Cached(child)` narrows the skip to a subtree: it returns its last size
while the constraints and everything inherited through the `Env` are
unchanged and nothing inside asked for a layout. `Reactive`, `For`, `Scroll`
and `TextInput` ask when they change; a custom widget whose size depends on
state outside a signal calls `Invalidate(env)` with the Env it was laid out
under. Wrap the panels that do not change together in it.

State that has no signal and must outlive a rebuild can be kept on the
Canvas under a typed `Slot`: `dst.Retain(anchor, slot, v)` stores a value
for the next frame and `dst.Retained(anchor, slot)` reads what was stored
last frame, where the `Anchor` is the widget's ID or its Rect. `dst.Ease`
is a `Motion` kept that way. Tooltip keeps its hover timer and Transition
its start time in slots.

## Inspector

`Config{Inspector: ebiten.KeyF1}` binds a key that toggles an overlay
outlining every widget painted through `Canvas.Paint`, colored by depth, and
naming the one under the cursor with its size and position;
`App.Inspector(on)` does the same from code. It is the quickest way to see
why something sits where it does.

## Layout

```
├── app.go        App runtime: window setup, frame loop, ebiten.Game
├── signal.go     Reactivity: Signal, Memo, Effect, dependency tracking
├── widget.go     Widget interface, Builder, Component/Reactive, Children
├── widgets.go    Built-in layout and drawing widgets
├── editor.go     TextInput: the text editor and its IME driver
├── anim.go       Tween, Spring, Motion, easings, the per-frame animator
├── probe.go      Probe: headless frame driver for tests
├── tooltip.go    Tooltip
├── popup.go      Popup: anchored overlay with a closing scrim
├── transition.go Transition and Presence: enter and leave animations
├── cache.go      Cached: per-subtree layout cache
├── image.go      Image widget and image loading
├── inspector.go  The widget inspector overlay
├── style.go      TextStyle, Env, Key, Theme
├── for.go        For: keyed, reactive list
├── canvas.go     Canvas: paint target plus the frame's hit regions
├── input.go      Pointer and keyboard events, Pointer/Tap/Focus widgets
├── clipboard.go  System clipboard for cut, copy and paste
├── font.go       Font loading, default font, text wrapping
├── geometry.go   Point, Size, Rect, Constraints
├── ui/           One file per control: Button, Checkbox, Radio, Switch,
│                 Slider, TextField, Select, Menu, Tabs, Collapsible,
│                 Card, Badge, Progress, Divider
└── examples/     Runnable apps: counter, todo, gallery
```

## Development

```sh
make        # fmt + vet + test
make run    # the counter example
make run-todo
make run-gallery
```
