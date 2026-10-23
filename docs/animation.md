# Animation

[Documentation](README.md) · [Project README](../README.md)

Animate values and transitions while keeping motion tied to component lifetimes.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

`Tween(v, d)` and `Spring(v)` move a value toward a target over time. Both
implement `Binding`, so controls can read and set them. Bind a property to animate it without rebuilding the widget:

```go
width := ggui.Tween(0.0, 200*time.Millisecond).Easing(ggui.EaseOut)
bar := ggui.Box().BindWidth(width).Height(4).Fill(accent)
width.Set(120) // slides there over 200ms; Jump(v) skips the motion
```

`TweenOf(source, d)` and `SpringOf(source)` follow a `Readable` instead of
being set by hand: a progress bar bound to `TweenOf(fraction, d)` eases to
each new fraction. They watch the source, so like `Watch` they need an owner.

Animations created inside an owner stop when it is disposed and keep their
last value; `Set` and `Jump` on those disposed values do nothing. Create them
in component setup or `App.Setup` to keep them across builder reruns. An
animation created without an owner runs until it settles or `Jump` stops it.

A tween restarts from wherever it is when retargeted; a spring keeps its
momentum, overshoots a little and settles (`.Stiffness`, `.Damping`). Easings:
`EaseLinear`, `EaseIn`, `EaseOut`, `EaseInOut`. The runtime steps running
animations once per frame, before effects are flushed. For a look that moves
inside one widget, `Motion` is the same tween driven from `Paint` with the
current time and no signal; `ui.Switch` slides its knob with one.

## Colors, points and other values

`TweenWith(v, d, lerp)` tweens any type with a lerp that returns `a` at zero
and `b` at one. `LerpColor` blends in premultiplied sRGB, as CSS does, so a fade
to transparent has no gray fringe; `LerpFloat`, `LerpPoint`, `LerpSize`,
`LerpRect` and `LerpInsets` cover geometry. `TweenColor(c, d)` is the common
case. A spring needs numbers it can move one by one, so `SpringWith(v, vec)`
takes a `Vector` that splits the value into at most four: `ColorVector`,
`PointVector`, `SizeVector`, `RectVector`, `InsetsVector` or your own.
`TweenOfWith` and `SpringOfWith` follow a source. Every tween lands on its
target value itself, so a theme token tweened to stays a token.

```go
fill := ggui.TweenColor(idle, 150*time.Millisecond)
at := ggui.SpringWith(ggui.Pt(0, 0), ggui.PointVector).Stiffness(260)
fill.Set(hover)
at.Set(ggui.Pt(120, 40))
```

`Tweened.Delay(d)` holds each move at its start for `d`, the simplest way to
stagger a few independent values.

## Curves

Beside the cubic easings: `EaseInOutCubic`, `EaseInBack`, `EaseOutBack`,
`EaseInOutBack`, `EaseInElastic`, `EaseOutElastic`, `EaseInBounce`,
`EaseOutBounce`, and the Material 3 curves `EaseStandard`, `EaseEmphasized`
and their `Accelerate` (exit) and `Decelerate` (entrance) variants.
`CubicBezier(x1, y1, x2, y2)` is CSS's `cubic-bezier`. `Interval(begin, end, e)`
runs `e` within a stretch of `[0, 1]`, zero before and one after, and
`Reverse(e)` mirrors a curve end for end, turning an ease-in into an ease-out.

## Timelines and stagger

`Timeline(d)` is one progress from zero to one that several values move by:
`Play`, `Reverse`, `Stop` and `Seek`, with `.Repeat(n)` (negative repeats until
`Stop`), `.Alternate()` to ping-pong and `.OnDone(fn)`. It is a `Readable`, and
so are the values derived from it: `tl.Interval(begin, end, e)`,
`tl.Stagger(i, n, span, e)` for item `i` of `n` each playing for `span` of the
timeline, and `Interpolate(p, from, to, lerp)` to turn a progress into a value
to bind:

```go
tl := ggui.Timeline(500 * time.Millisecond) // in setup
for i, row := range rows {
	p := tl.Stagger(i, len(rows), 0.4, ggui.EaseOutBack)
	ggui.Box(row).BindWidth(ggui.Interpolate(p, 0.0, 240.0, ggui.LerpFloat))
}
tl.Play()
```

Like a tween, a timeline made under an owner stops with it. When the
application's environment asks for reduced motion (`SetEnv` with
`ReducedMotionKey`), `Play` and `Reverse` land on their end at once.

## Enter and leave transitions

`Transition(child).Fade().Slide(dx, dy).Scale(from)` plays
an enter animation when the child first appears, over `.Duration(d)` with
`.Easing(e)`. Whether it is new is judged against the previous frame by the
identity a keyed component gives it, else by Rect, so rebuilding a subtree
with the same identity does not restart it.
`Presence(show, child)` keeps the child on screen when `show` turns false,
inert to input, and runs the same animation backwards before removing it:

```go
ggui.Presence(open, ggui.Transition(panel).Slide(0, -8).Fade())
```

## Implicit animations

`Switcher(key, build)` is `Key` that cross-fades: when the key changes the new
subtree enters while the old one plays the same animation backwards, inert,
and is disposed once gone. The first subtree appears at once. Configure it with
`.Fade()`, `.Slide(dx, dy)`, `.Scale(from)`, `.Duration(d)` and `.Easing(e)`
before mount; subtrees are stacked at the top-left corner.

```go
ggui.Switcher(tab, page).Slide(0, 8).Duration(180 * time.Millisecond)
```

`AnimatedBox(child)` is a Box whose `Fill`, `Border`, `Radius`, `Width`,
`Height` and `Padding` ease to each new value, set or bound (`BindFill`,
`BindBorderColor`, `BindRadius`, `BindWidth`, `BindHeight`, `BindPadding`),
over `.Duration(d)` with `.Easing(e)`. Theme colors resolve before they blend,
so a theme switch fades too. Keep one box and change its values: a box built
anew starts where it is told.

```go
card := ggui.AnimatedBox(content).BindFill(tint).BindPadding(inset).Radius(8)
```

`Switcher`, `AnimatedBox`, `Transition` and `Each` rows land at once under
`ReducedMotionKey`.
