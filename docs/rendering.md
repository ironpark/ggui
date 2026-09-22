# Custom widgets and rendering

[Documentation](README.md) · [Project README](../README.md)

Implement custom widgets, draw at the correct display scale, and understand frame and cache behavior.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [HiDPI](#hidpi)
- [Custom widgets](#custom-widgets)
- [Frame lifecycle](#frame-lifecycle)
- [Layout caching](#layout-caching)
- [Retained paint state](#retained-paint-state)
- [Overlays](#overlays)

## HiDPI

Widgets work in logical pixels; the screen is allocated at the monitor's
device scale factor so a Retina display gets a sharp image. `dst.Scale()`
returns the factor, and drawing goes through it: `dst.FillRect`,
`dst.FillRoundRect`, `dst.StrokeRoundRect`, `dst.FillCircle` and
`dst.StrokeLine` take logical geometry, `dst.Px(v)` converts a length,
`dst.Geo(at)` is the transform for `DrawImageOptions`, and text rasterizes
its face at the scaled size rather than scaling the pixels. A custom widget
that draws with Ebitengine directly should do the same.

## Custom widgets

Implement `Layout` and `Paint`. `Layout` receives constraints and an inherited
`Env`; return a size within those constraints. Store any resolved theme, font,
geometry, or child positions that `Paint` needs, because `Paint` receives only
the canvas and assigned rectangle. Pass the `Env` to children, deriving a new
value only when you intend to override inherited settings.

A widget that shows text, or wraps one that does, may also implement
`Baseliner`: `Baseline()` returns how far below the widget's top its first
text baseline sits, valid after `Layout`. `Row.Align(AlignBaseline)` reads it.
`Text` and `TextInput` report their font's ascent; `Box`, `Column`, `Align`
and the wrappers report their first child's, moved by where the child is
painted. A widget that does not implement it is aligned by its bottom edge.

Paint children through `dst.Paint(child, r)` so the inspector can see them.
The [custom example](../examples/custom) builds a dial on `Interactive`, a
disclosure that calls `Invalidate`, and a virtualized list. For small
widgets, `FromFuncs` wraps the two methods as closures:

```go
dot := ggui.FromFuncs(
	func(c ggui.Constraints, _ ggui.Env) ggui.Size { return c.Constrain(ggui.Sz(8, 8)) },
	func(dst *ggui.Canvas, r ggui.Rect) {
		dst.FillCircle(ggui.Pt(r.Origin.X+4, r.Origin.Y+4), 4, fg)
	},
)
```

## Frame lifecycle

Every frame the runtime routes input and steps animations, settles internal
bindings and mounts, stabilizes layout, runs user effects, and then paints.
Effect writes trigger another stabilization before paint. Layout runs when
the tree is dirty, the window size changes, state invalidates layout, or
`Invalidate` is called. Hover and press normally affect paint without requiring
layout. Input, animation, and other frame bookkeeping still run. See
[Layout caching](#layout-caching) for how unchanged subtrees skip measurement.

A custom widget that keeps size-affecting state outside signals calls
`Invalidate(env)` when that state changes; `Scroll` does for its offset. A
`Scroll` also tells its subtree the window it shows through the `Env`
(`ScrollViewport(env)`), which is how `EachKeyed` virtualizes.

Effects are flushed until they are quiet. If writes keep retriggering effects without settling, the frame gives
up, `App.Run` returns `ErrCycle` and `Probe` panics with it. Match it with
`errors.Is(err, ggui.ErrCycle)` and print the error for the detail, which says
how many effects the cycle turns and how many there were in all. Build with
`-tags ggui_debug` and each is named by the line that created it:

```
ggui: effects did not settle after 16 passes; an Effect is writing a StateValue it reads
  2 of 15 effects never settled
    - /src/app/total.go:31 (a derived value)
    - /src/app/cart.go:64
```

Use `Derived` for computed values. If a read should not subscribe, wrap that
read in `Untrack`; also check that writes converge instead of continually
changing another dependency.

## Layout caching

A rebuild boundary is a layout boundary. `Component`, `Reactive`, and `Key` each cache their subtree's size: they return it unchanged while
the constraints and everything inherited through the `Env` are the same as
last time, layout input versions match, and nothing inside asked for a layout.
Measurement dependencies include untracked reads without adding reactive
subscriptions. A signal write in one panel
therefore measures that panel, not the whole window, even though the runtime
lays out from the root whenever anything was written.

`Cached(child)` is the same cache without a rebuild boundary, for a static
subtree that sits under something that does rebuild.

`EachKeyed`, `Scroll` and `TextInput` ask for a layout when they change. A custom
widget whose size depends on state outside a signal **must** call
`Invalidate(env)` with the Env it was laid out under; without it the widget
keeps the size it was last measured at, since the cache above it has no
reason to measure again.

## Retained paint state

State that has no signal and must outlive a rebuild can be kept on the
Canvas under a typed `Slot`: `dst.Retain(anchor, slot, v)` stores a value
for the next frame and `dst.Retained(anchor, slot)` reads what was stored
last frame, where the `Anchor` is the widget's ID or its Rect. `dst.Ease`
is a `Motion` kept that way. `ui.Tooltip` keeps its hover timer and Transition
its start time in slots.

## Overlays

An `Overlay` is drawn over the whole window after the widget tree, and is
offered each frame's input before any widget: a development panel, a frame
counter, a recording banner. `App.SetOverlay(o)` installs one and
`SetOverlay(nil)` removes it; the widget inspector installs itself there
while it is on.

```go
type Overlay interface {
	Paint(dst *Canvas)
	Input(in OverlayInput) bool
	Cursor(p Point) (CursorShape, bool)
}
```

`Paint` draws imperatively with the Canvas shape and path methods, and with
`DrawText` and `TextWidth`, which draw and measure one line in a `Font` at
a logical size; a nil font is `DefaultFont()`. `Physical` gives a Rect in
image pixels for an overlay that keeps an image of its own. `Input` returns
true to keep the frame's input from the widgets; it is not offered while a
widget holds a press, so a drag that began in the app finishes there.
`Cursor` chooses the mouse cursor over the overlay. An overlay cannot be a
widget: it paints after the frame's trace is complete and read back, so it
must not paint through `Canvas.Paint`.

