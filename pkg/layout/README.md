# Layout without ggui widgets

Import `github.com/ironpark/ggui/pkg/layout` to measure and position boxes without
importing the root `ggui` package. The package owns `Point`, `Size`, `Rect`, their
operations and numeric constructors. It depends only on the standard library and
does not import graphics,
windowing, text shaping, reactive state, or internal ggui packages. It is a
public package in the existing Go module, not a separately versioned module.

```go
var scratch layout.Scratch
result := scratch.Flow(
    layout.Loose(layout.Sz(200, 80)),
    layout.FlowStyle{Axis: layout.Horizontal, Gap: 10},
    []layout.Item{{}, {Weight: 1}},
    func(i int, c layout.Constraints) layout.Measurement {
        // Substitute the host toolkit's text/image/widget measurement here.
        return layout.Measurement{Size: c.Constrain(layout.Sz(50, 30))}
    },
)
// result.Size is {200, 30}.
// result.Children contains {{0, 0}, {50, 30}} and {{60, 0}, {140, 30}}.
```

See [the executable example](example_test.go) for recursive measurement with no
widgets or rendering backend.

## Operations

| Operation | Contract |
| --- | --- |
| `Scratch.Flow` | Row/Column, weighted flex, loose flex, justification, cross alignment, horizontal baseline alignment, RTL |
| `Scratch.Wrap` | Left-to-right lines with child gaps, line gaps, and vertical alignment |
| `Scratch.Grid` | Equal-width columns and tallest-child row heights; `GridResult.CellWidth` reports the column width |
| `Scratch.Stack` | Children overlaid at the origin, optionally filling bounded axes |
| `Box` | Optional child, padding, fixed dimensions, and maximum dimensions |
| `Align` | One child placed at fractions of free space, independently of direction |

`Flow` defaults to a vertical start-aligned layout. A ggui Row supplies horizontal
orientation and center cross alignment explicitly. Weighted children fill the
bounded main axis; on an unbounded main axis they measure naturally. These are
constrained box algorithms, not CSS flexbox or CSS grid: there is no flex-shrink,
percentage resolution, or browser intrinsic-size algorithm.

## Measurement and ownership

- Coordinates and sizes use logical pixels. Results contain parent-relative
  rectangles; the host translates them into world coordinates as needed.
- Constraints flow into `MeasureFunc`; return a size satisfying those constraints.
  Maximums may be `math.Inf(1)`. For contradictory bounds, minimums take priority.
- Flow measures rigid children first, then weighted children. Every child is
  measured once. It initially reserves gaps for every child and adjusts the gap
  count after measuring rigid children, matching ggui's existing behavior.
- Wrap, Stack, Box, and Align measure each child once. Grid measures each child
  twice when width is unbounded: natural width first, then tight cell width.
- A zero-sized child is still present. An absent rigid child returns
  `Measurement{Absent: true}` to omit its flow/wrap gap. A weighted child must
  reserve its slot and must not report absence on a bounded main axis. Grid
  always reserves cells.
- Supply `Baseline` and `HasBaseline` for text alignment. Result baselines come
  from the first present child with a baseline, translated by its position.
- Each multi-child container owns a `Scratch`. Use separate instances for nested
  or concurrent calls; do not reenter or copy an instance after first use.
- `Result.Children` is borrowed until the next call on the same scratch. Copy it
  to retain a snapshot. `Box` and `Align` return an owned `SingleResult` without
  allocating result buffers.
- The core does not cache measurements. The host owns invalidation, scheduling,
  clipping, painting, hit testing, and content measurement. Some constrained
  containers can have overflowing child rectangles; results do not imply clips.

## ggui integration

The root package delegates its Box, Row, Column, Stack, Align, Wrap, and Grid
calculations here. Existing `ggui.Constraints`, `Justify`, `CrossAlign`, and
`TextDirection` names alias the core types. Widget adapters resolve `Env`
spacing/direction and flex wrapper metadata before calling the core, while
existing property tracking and layout caches remain in ggui. Multi-child widget
adapters retain the engine's borrowed `Children` slice directly, with no separate
size/offset buffers or result-copy loop. Virtualized lists use the same `[]Rect`
representation for the rows they measure themselves.

`ggui.Point`, `Size`, and `Rect` alias the types in this package. Geometry
consumers import `github.com/ironpark/ggui/pkg/layout` directly.

Scroll behavior, virtualized item lifetimes/estimation, reactive caches, and text
shaping remain in ggui. Extracting those domains or publishing a separate module
is independent of using this layout package.

Run `go test ./pkg/layout` for the renderer-independent contract tests and example,
or `go test ./...` to verify ggui integration as well.
