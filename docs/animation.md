# Animation

[Documentation](README.md) · [Project README](../README.md)

Animate values and transitions while keeping motion tied to component lifetimes.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

`Tween(v, d)` and `Spring(v)` move a value toward a target over time. Both
implement `Binding`, so controls can read and set them. A `Reactive` callback
that reads one reruns as the animated value changes:

```go
width := ggui.Tween(0.0, 200*time.Millisecond).Easing(ggui.EaseOut)
bar := ggui.Reactive(func() ggui.Widget {
	return ggui.Box().Size(width.Get(), 4).Fill(accent)
})
width.Set(120) // slides there over 200ms; Jump(v) skips the motion
```

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
