# Carousel and Input OTP

[Documentation](README.md) · [Project README](../README.md)

Create swipeable slide collections and segmented code inputs with native keyboard and accessibility support.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

Native implementations of the [shadcn Carousel](https://ui.shadcn.com/docs/components/base/carousel)
and [Input OTP](https://ui.shadcn.com/docs/components/base/input-otp) designs. They
inherit ggui theme tokens and require no browser, JavaScript or new dependency.

## On this page

- [Carousel](#carousel)
- [Input OTP](#input-otp)
- [Examples and validation](#examples-and-validation)

## Carousel

```go
slide := ggui.State(0)
carousel := ui.Carousel(slide,
    ui.CarouselItem(ui.Card(ggui.Center(ggui.Title("1")))),
    ui.CarouselItem(ui.Card(ggui.Center(ggui.Title("2")))),
    ui.CarouselItem(ui.Card(ggui.Center(ggui.Title("3")))),
).Height(240).Loop(true)
```

`Carousel` accepts arbitrary widgets, `CarouselItem` wrappers, or a
`CarouselContent(items...)` group. Navigation buttons are included by default.
When controls are shown, the outer dimensions include up to 48 logical pixels
of space on either side for the buttons;
vertical carousels reserve that space above and below instead.

### Size and arrange slides

- `CarouselItem(...).Basis(.5)` fits two slides. `BasisWhen(func(viewport
  ggui.Size) float64)` supports responsive fractions. Gap is subtracted from the
  fraction, keeping complete slides within the viewport.
- `Gap(16)` changes spacing. `Align(0)`, `.Align(.5)` and `.Align(1)` select start,
  center and end alignment. Center is the default; end snaps are trimmed to
  avoid empty space. The binding is the **snap index**, which can differ from
  the item index when several items fit.
- `Vertical()` changes the axis. `RTL(true)` reverses horizontal layout,
  navigation and dragging. `Loop(true)` wraps without cloned widgets; it falls
  back to bounded navigation when there are too few slides to fill the loop.
- `Controls(false)` allows separate `CarouselPrevious(carousel)` and
  `CarouselNext(carousel)` widgets in your own layout.
### Navigation and playback

- `Next`, `Previous`, `ScrollTo`, `Selected`, `SnapCount`, `CanNext`,
  `CanPrevious` and `OnChange` provide programmatic control. Snap information
  becomes available after layout. External binding writes also move the strip.
- Drag with a mouse or touch. Arrow keys move along the active axis; Home/End
  reach the endpoints. Native accessibility supports increment/decrement and
  setting the selected snap. Interactive slide children retain their input;
  dragging begins on the non-interactive slide surface.
- `Autoplay(2*time.Second)` enables playback. Hover, keyboard focus and dragging
  pause it. User interaction stops it until `Play()`; use
  `StopOnInteraction(false)` to resume automatically. `Pause()` stops it.
- `Draggable(false)`, `Disabled` and `DisabledWhen` control interaction.

### Motion and rendering

Motion uses a closed-form version of Embla 8.6's default spring response
(duration parameter 25, friction .68 at 60Hz). It settles within 1800ms;
`Animation(duration)` scales this time, and zero disables motion. Reduced motion
makes navigation immediate and disables autoplay. This matches the reference's
entrance/glide character; it is not an Embla plugin compatibility layer.

Geometry is prepared at layout time. Painting uses a binary search and visits
only visible slides, including in loop mode; invisible slides contribute no hit
regions. No per-slide images, cloned trees, goroutines or autoplay timers are
allocated. Configure layout options before mounting, or rebuild the component
through a reactive boundary when those options change.

## Input OTP

```go
code := ggui.State("")
input := ui.InputOTP(code, 6).
    Groups(3, 3).
    Named("Verification code").
    OnComplete(func(code string) { /* submit to your verification handler */ })
```

Composition is also explicit when you need different groups:

```go
input := ui.InputOTP(code, 6,
    ui.InputOTPGroup(ui.InputOTPSlot(0), ui.InputOTPSlot(1)),
    ui.InputOTPSeparator(),
    ui.InputOTPGroup(ui.InputOTPSlot(2), ui.InputOTPSlot(3)),
    ui.InputOTPSeparator(),
    ui.InputOTPGroup(ui.InputOTPSlot(4), ui.InputOTPSlot(5)),
)
```

Every index from zero to length-minus-one must occur exactly once. Invalid
compositions panic at layout instead of silently dropping editable characters.
Groups/slots/separators are presentation specifications; the component is one
native text field, with one Tab stop and one accessible value.

### Accepted characters

- ASCII digits are accepted by default. `Alphanumeric()` allows ASCII letters
  and digits. `Pattern(*regexp.Regexp)` matches each complete character;
  `Accept(func(rune) bool)` supports other alphabets, including Unicode.
- Disallowed characters are removed and values are truncated to the configured
  number of Unicode code points. For example, pasting `123-456` gives `123456`.
  This normalization is a deliberate native adaptation of the reference's
  pattern validation. External binding writes should already be valid.
### Editing and callbacks

- Clicking an occupied slot selects that character for replacement. Dragging
  selects a range. Native arrow/selection, Home/End, Backspace/Delete,
  select-all, copy/cut/paste, undo/redo and IME editing remain available.
- `OnChange`, `OnComplete` and `OnSubmit` report edits, completion and Enter.
  Completion fires when an edit produces a new complete code, not on every frame
  or on an external binding write. Editing down to an incomplete code resets it.
- `Invalid` / `InvalidWhen`, `Disabled` / `DisabledWhen`, `Placeholder`,
  `SlotSize` and `RTL` configure appearance and behavior. `ui.Field` provides
  labels, helper text and validation messages.
- `Input()` exposes the underlying native editor. Its `Filter`, `Select`,
  `EditingState` and `PaintCustom` hooks are also available for other segmented
  text presentations. Filtering covers IME, clipboard, undo and accessibility
  edits. Custom painting supplies the actual caret rectangle to the native IME.

### Appearance and native limits

Slots default to 32px with joined borders, rounded group ends and a focus ring
around the active slot. The caret blinks at 500ms intervals and remains steady
under reduced motion. Narrow constraints compress slots without changing code
order. Native font metrics remain ggui's. Browser-specific SMS autofill and
password-manager integration are outside this native component's scope.

## Examples and validation

Run from the repository root:

```sh
go run ./examples/controls
go run ./examples/controls -dark -example input-otp-separator
go run ./examples/controls -render-dir /tmp/ggui-controls-renders
```

The catalog contains all 18 examples from the inspected shadcn Base UI docs and
one additional looping example. Both components also appear in the main gallery.
The form demo checks code length locally; connect your own verification handler
for real authentication.

The render tool uses the real GPU renderer to create 304 PNGs: 19 examples,
light/dark themes, 320/560px widths, and four states. Carousel samples show initial,
120ms glide, settled and final-snap states. OTP samples show initial, active,
complete and invalid states; disabled examples stay disabled throughout.

Review compares the live reference and its example sources against native
renders. Corrections include demo height, disabled opacity, joined focus-ring
corners and Embla's spring response. Tests exercise dragging, keyboard navigation,
loop boundaries, responsive snaps, external bindings, autoplay pause/stop,
reduced motion, visibility culling, filtering, clipboard, undo, slot replacement,
Unicode selection, IME commits, accessibility and reactive validation/disabled
states.

A 10,000-slide loop paints its visible portion in roughly 1.3µs in the included
**headless** benchmark on an Apple M1 Ultra. This excludes GPU submission and is
not an FPS measurement:

```sh
go test ./ui -run '^$' -bench BenchmarkCarouselVisiblePaint10000 -benchmem
```
