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

- [Configuration and runtime updates](#configuration-and-runtime-updates)
- [Choosing a layout](#choosing-a-layout)
- [Rows, columns, and flex](#rows-columns-and-flex)
- [Wrap and grid](#wrap-and-grid)
- [Popups](#popups)
- [Images](#images)
- [Text](#text)

## Configuration and runtime updates

Value setters such as `Box.Width`, `Row.Gap`, `Text.Size`, `Text.LineHeight`,
`Color` and `Placeholder` work after mount. Measurement-affecting changes refresh
ancestor caches; paint-only values are used by the next paint. Repeating the
same value does not schedule another layout.

Use `X(value)` for a fixed value and `BindX(reader)` to follow a source. A literal
setter removes the corresponding reader, even when its current value is equal.
For example, `box.BindWidth(width).Size(100, 40)` detaches both dimensions;
`box.Size(100, 40).BindWidth(width)` retains a height of 40. `Pad` and `Padding`
write the same property; `Style` merges only its nonzero/non-nil fields.

Tree structure, identity, orientation, input construction modes and formatter,
renderer or event callback installation are configured before mount. Rebuild
those with `View` or `Reactive`. `Text.Content`/`BindContent` replace a text
source, while `Scroll.Offset(value)`/`BindOffset(binding)` select local or
externally bound scroll position. Bind methods require non-nil readers.

Custom parents may set child literals before measuring them. Install bindings
in setup or UI handlers, not Layout or Paint. Built-in property revisions are
tracked by layout caches without subscriptions. For custom non-reactive data,
retain the layout `Env` and call `Invalidate(env)` when measurement changes.
Widget mutation belongs on the UI goroutine; workers use `App.Post`.

## Choosing a layout

| Need | Widgets |
| --- | --- |
| A line of children | `Row`, `Column` |
| Share remaining space | `Flex`, `Expanded`, `Spacer` |
| Flow onto multiple lines | `Wrap` |
| Equal-width columns | `Grid` |
| Overlap children | `Stack` |
| Position one child | `Align`, `Center` |
| Decorate, inset or cap content | `Box`, `Padding`, `Box.MaxWidth` |
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

`Box(w).MaxWidth(640)` stops a form or a paragraph from growing past 640
pixels, padding included, in a wide window; `MaxHeight` caps the height. A
parent that stretches its children still wins, so center the box to keep it
narrow there: `ggui.Center(ggui.Box(form).MaxWidth(640))`.

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
at the left. `.Stretch()` is short for `.Align(ggui.AlignStretch)`, the usual
arrangement of a form or a page. `AlignBaseline` lines a Row's children up on their first text
baseline, so a caption beside a title shares its baseline rather than its
centre; a child with no text rests on that baseline by its bottom edge. A
Column, `Wrap` or `Each` treats it as `AlignStart`.

```go
ggui.Row(ui.Title("Inbox"), ui.Caption("12 unread")).Gap(8).Align(ggui.AlignBaseline)
```

```go
ggui.Row(ggui.Text("Title"), ggui.Spacer(), ggui.Text("3 items")) // centered on its height
ggui.Column(ggui.Text("a"), ggui.Text("b")).Stretch() // Align(ggui.AlignStretch)
```

### Changing arrangement by width

`Responsive(breakpoint, wide, narrow)` lays out `wide` while it has at least
`breakpoint` of width and `narrow` below it. Both may hold the same widgets,
so a field keeps its text and focus as the window crosses the breakpoint:

```go
search, filters := ui.TextField(query), ui.ToggleGroup(filter).Options(kinds)
ggui.Responsive(760,
    ggui.Row(ggui.Expanded(search), filters).Gap(16),
    ggui.Column(search, filters).Gap(12).Stretch(),
)
```

## Wrap and grid

`Wrap` and `Grid` cover the two other common arrangements. `Wrap` flows
children left to right and starts a new line where the next one would not
fit, for tags and toolbars; `.Gap(v)` spaces both axes, `.RunGap(v)` the
lines alone, `.Align(...)` places children within their line. `Grid(cols,
...)` deals children into equal-width columns, each given its cell width
tight so columns line up, with rows as tall as their tallest cell.

## Popups

`Popup(anchor, content)` floats content below its anchor (above it when
there is no room), painted through `Canvas.Overlay` over a scrim: a press
anywhere outside closes it and reaches nothing underneath. `Show`, `Hide`,
`Toggle` and `IsOpen` drive it, or `.BindOpen(sig)` keeps the state in a
`StateValue[bool]`; `.Keys(h)` keeps keyboard focus on the widget that opened it
while the pointer is in the content, and a widget inside can find its popup
with `PopupOf(env)` to close it after acting. `ui.Select` and `ui.Menu` are
built on it. `ui.Tooltip` and `ui.HoverCard` paint through the same
`Canvas.Overlay` without a scrim, so their anchors keep every event.

## Images

`Image(img)` draws a `*ggfx.Image` at its natural size, shrinking to
the room it gets with its aspect ratio kept; `.Size`, `.Width` or `.Height`
fix it, and `.Fit(FitContain | FitCover | FitFill | FitNone)` says how it
sits in a box of another shape. `DecodeImage(bytes)` and
`LoadImageFile(path)` read PNG, JPEG and GIF.

Those decode on the calling goroutine, which for a large photo or a list of
thumbnails is a visible stall. `AsyncImage(src)` reads and decodes `src` on a
goroutine instead, shows `.Placeholder(widget)` until it is ready (and
`.Fallback(widget)` if it fails), then lays out and paints as `Image` does,
with the same `.Fit`, `.Size`, `.Width`, `.Height`, `.Pixelated` and `.Alt`.
`.BindSrc(r)` follows a signal; a new src cancels the old load through its
`context`. `.Loader(fn)` replaces the file read, `.MaxSize(px)` downscales
before upload, and `.Status()` / `.Err()` report the state.

```go
ggui.AsyncImage(photo.Path).MaxSize(256).Size(96, 96).Fit(ggui.FitCover).
	Placeholder(ui.Skeleton(96, 96))
```

Decoded images live in one process-wide cache keyed by src and max size, so
the same src shown twice decodes once. Images no widget shows are kept in
least-recently-used order within a byte budget, `SetImageCacheLimit(bytes)`
(256 MiB by default), and deallocated when evicted; an image on screen is
never evicted.

## Text

`Text` wraps at spaces to the width it is given, and between runes when a
word is wider than the line, so scripts without spaces wrap too. `.Size(px)`,
`.Color(c)`, `.Font(f)`, `.LineHeight(mult)`, `.Style(ts)`, `.Align(0.5)` and
`.NoWrap()` adjust it; what is not set is inherited (see [Styling and themes](styling.md)). The
default font is the platform's (see [Fonts](fonts.md#choose-text-fonts));
`LoadFont(ttf)` or `LoadFontFile(path)` load your own, and `SetDefaultFont`
makes one the default.
