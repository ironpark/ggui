# Widgets and layout

[Documentation](README.md) · [Project README](../README.md)

Choose a layout, size its children, and display text, images, and overlays. Dimensions are logical pixels unless stated otherwise.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

A `Widget` is asked for a size under `Constraints` and an `Env`
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

## On this page

- [Choosing a layout](#choosing-a-layout)
- [Rows, columns, and flex](#rows-columns-and-flex)
- [Wrap and grid](#wrap-and-grid)
- [Tooltips and popups](#tooltips-and-popups)
- [Images](#images)
- [Text](#text)

## Choosing a layout

| Need | Widgets |
| --- | --- |
| A line of children | `Row`, `Column` |
| Share remaining space | `Flex`, `Expanded`, `Spacer` |
| Flow onto multiple lines | `Wrap` |
| Equal-width columns | `Grid` |
| Overlap children | `Stack` |
| Position one child | `Align`, `Center` |
| Decorate or inset content | `Box`, `Padding` |
| Show overflowing content | `Scroll` |

Build children from a plain slice with `List` or `Children`:

```go
ggui.List(rows, func(r Row) ggui.Widget {
	return ggui.Text(r.Title)
}).Gap(4)

ggui.Row(ggui.Children(rows, func(r Row) ggui.Widget {
	return ggui.Text(r.Title)
})...)
```

Padding uses CSS order: `Padding(w, 8)` sets every side, `Padding(w, 4, 12)`
sets vertical/horizontal padding, and `Box(w).Pad(1, 2, 3, 4)` sets
top/right/bottom/left.

`Stack` layers children at the top-left and takes its largest child's size
unless `.Expand()` is set. `Align` fills available space and positions its child
by fraction (`.At(0.25, 1)`) or edge (`.Bottom().Right()`). `Center` places it at
`(0.5, 0.5)`.

`ggui.Sz(w, h)` and `ggui.Pt(x, y)` accept ints or floats without casts.

## Rows, columns, and flex

`Row` and `Column` hug their children by default. `Expanded(child)` and
`Flex(child, weight)` share whatever main-axis space the rigid children leave,
`Spacer()` is an empty `Expanded`, and `.Justify(...)` distributes slack
(`JustifyCenter`, `JustifyEnd`, `SpaceBetween`, `SpaceAround`, `SpaceEvenly`).
Any of those makes the widget fill its main axis. `.Align(...)` places
children across the axis (`AlignStart`, `AlignCenter`, `AlignEnd`,
`AlignStretch`, `AlignBaseline`); a Row centers by default, a Column starts
at the left. `AlignBaseline` lines a Row's children up on their first text
baseline, so a caption beside a title shares its baseline rather than its
centre; a child with no text rests on that baseline by its bottom edge. A
Column, `Wrap` or `Each` treats it as `AlignStart`.

```go
ggui.Row(ggui.Title("Inbox"), ggui.Caption("12 unread")).Gap(8).Align(ggui.AlignBaseline)
```

```go
ggui.Row(ggui.Text("Title"), ggui.Spacer(), ggui.Text("3 items")) // centered on its height
ggui.Column(ggui.Text("a"), ggui.Text("b")).Align(ggui.AlignStretch)
```

## Wrap and grid

`Wrap` and `Grid` cover the two other common arrangements. `Wrap` flows
children left to right and starts a new line where the next one would not
fit, for tags and toolbars; `.Gap(v)` spaces both axes, `.RunGap(v)` the
lines alone, `.Align(...)` places children within their line. `Grid(cols,
...)` deals children into equal-width columns, each given its cell width
tight so columns line up, with rows as tall as their tallest cell.

## Tooltips and popups

`Tooltip(child, text)` shows text below the child once the cursor has
rested on it for half a second (`.Delay(d)`). It registers no hit region, so
the child gets every event, and it paints through `Canvas.Overlay`, above
everything else.

`Popup(anchor, content)` floats content below its anchor (above it when
there is no room), painted through `Canvas.Overlay` over a scrim: a press
anywhere outside closes it and reaches nothing underneath. `Show`, `Hide`,
`Toggle` and `IsOpen` drive it, or `.Bind(sig)` keeps the state in a
`StateValue[bool]`; `.Keys(h)` keeps keyboard focus on the widget that opened it
while the pointer is in the content, and a widget inside can find its popup
with `PopupOf(env)` to close it after acting. `ui.Select` and `ui.Menu` are
built on it.

## Images

`Image(img)` draws an `*ebiten.Image` at its natural size, shrinking to
the room it gets with its aspect ratio kept; `.Size`, `.Width` or `.Height`
fix it, and `.Fit(FitContain | FitCover | FitFill | FitNone)` says how it
sits in a box of another shape. `DecodeImage(bytes)` and
`LoadImageFile(path)` read PNG, JPEG and GIF.

## Text

`Text` wraps at spaces to the width it is given, and between runes when a
word is wider than the line, so scripts without spaces wrap too. `.Size(px)`,
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Style(ts)`, `.Align(0.5)` and
`.NoWrap()` adjust it; what is not set is inherited (see [Styling and themes](styling.md)). The
built-in font is Go Regular; `LoadFont(ttf)` or `LoadFontFile(path)` load your
own, and `SetDefaultFont` makes one the default.
