# Recording a tour

[Documentation](README.md) · [Project README](../README.md)

A README reads better with the app moving in it. Package
`github.com/ironpark/ggui/record` draws a *tour*, a scripted walk through an
app, into an animated GIF, or into numbered PNG frames for a video. Nothing
is captured from the screen: the tour's tree runs in a
[`Probe`](testing.md) on a clock of its own, and each frame is painted
offscreen. A recording takes about as long as drawing its frames, needs no
display, and comes out the same every time, so it can be made again after
every change to the UI.

## On this page

- [Writing a tour](#writing-a-tour)
- [Driving the app](#driving-the-app)
- [The pointer and the keyboard](#the-pointer-and-the-keyboard)
- [The window frame](#the-window-frame)
- [ggui record](#ggui-record)
- [From Go](#from-go)
- [The GIF](#the-gif)
- [PNG frames and ffmpeg](#png-frames-and-ffmpeg)
- [What is not drawn](#what-is-not-drawn)

## Writing a tour

A tour lives in the app, beside what it shows. Register it before
`ggui.Run`, from an `init` function or from `main`:

```go
import (
	"github.com/ironpark/ggui/record"
	"github.com/ironpark/ggui/ui/theme"
)

func init() {
	var m *Model
	record.Register(record.Tour{
		Size:  ggui.Sz(800, 500), // the window drawn, in logical pixels
		Scale: 2,                 // drawn at 1600×1000, as on a Retina display
		Setup: func(p *ggui.Probe) {
			m = NewModel(demoData()) // demo state, not the user's
			theme.Bind(m.Dark, theme.Dark(), theme.Default())
		},
		Build: func() ggui.Widget { return m.Build() },
		Script: func(r *record.Recorder) {
			r.Hold(1500 * time.Millisecond) // the first screen, for 1.5s
			r.Then(time.Second, func() { r.Probe().Tap("Settings") })
			r.Then(time.Second, func() { m.Dark.Set(false) })
		},
	})
}
```

| Field | |
| --- | --- |
| `Name` | What `ggui record NAME` selects. `""` is the default tour; when only one tour is registered it is the default whatever its name. |
| `Size` | The logical size of the window drawn. Required. |
| `Scale` | Device pixels per logical pixel; 0 means 1. 2 draws text as sharply as a HiDPI screen shows it. |
| `FPS` | Frames drawn per second of tour time; 0 means 15. The file can keep fewer (`--fps`). |
| `Start` | The time the tour's clock reads at its first frame. Zero means noon on 1 January 2026, so that a clock or a "2 minutes ago" in the app reads the same in every recording. |
| `Setup` | Runs under the probe's root owner before the tree is built, as `App.Setup` does: themes, the environment, demo state. |
| `Build` | The root of the tree. Required. |
| `Script` | The steps. Required. |
| `Frame` | What is drawn around the app: `record.FrameNone` (the default), `record.FrameMac` or another macOS window style; see [the window frame](#the-window-frame). |
| `Title` | The title a `FrameMac` style sets in its title bar, as the app's `Config.Title` would be. |
| `Subtitle` | The line `FrameMacUnified` sets under the title, as `NSWindow.subtitle`. |
| `TitlebarHidden` | Draw any `FrameMac` style as `FrameMacHidden`, as for a window whose `Config` sets `TitlebarHidden`: no bar, the window buttons over the content. |
| `ButtonInset` | Move `FrameMacHidden`'s buttons this many points right and down, into a sidebar the app draws inset from the window's edges. |

`Setup` and `Build` are the place for demo data. The tour runs in the
app's process, but `ggui.Run` draws it before it would have opened a
window or run the app's `Setup`; what `main` does before calling `Run`,
such as opening a database, has run. An app that does work there a
recording should skip calls `record.Main()` first: it draws the tour and
exits when the process was started to record, and returns at once
otherwise.

```go
func main() {
	record.Main()
	db := openDatabase()
	// ...
	ggui.Run(cfg, build)
}
```

## Driving the app

`Recorder.Hold(d)` shows what is there for `d`: a frame for each 1/FPS of
it, the clock moved on by as much before each, so animations, transitions
and spinners play as they would on a screen. `Recorder.Then(d, step)` does
a step, lays the tree out again, and holds what follows for `d`.

A step is anything the app does: set a signal, call the model, or use the
probe, `Recorder.Probe()`, as a test does:

```go
r.Then(800*time.Millisecond, func() { r.Probe().Tap("New file") })
r.Then(600*time.Millisecond, func() { r.Probe().Text("notes.md") })
r.Then(time.Second, func() { r.Probe().Key("enter") })
r.Then(time.Second, func() {
	p := r.Probe()
	if f, ok := p.Find("Search"); ok {
		p.Scroll(f.Center(), ggui.Pt(0, 240))
	}
})
```

The probe's input methods run frames that are not drawn; what they change
shows in the frames `Hold` draws next. The pointer stays where the last
input put it, so a hovered button looks hovered. A panic in the script, as
`Tap`'s when nothing has the label, ends the recording with an error that
names it, and no file is written.

`Recorder.Elapsed()` returns the tour time drawn so far, for a script that
times a step to something already on screen.

## The pointer and the keyboard

The probe's input teleports: `Tap` clicks with nothing on screen moving,
and a viewer cannot tell what was clicked. The `Recorder`'s own pointer
and keyboard steps do what a person would, a frame at a time at the
tour's FPS, as `Hold` does, and draw a cursor that moves with them:

```go
Script: func(r *record.Recorder) {
	r.Hold(time.Second)
	r.ClickLabel("New file")                // glide there, click
	r.Type("notes.md", 70*time.Millisecond) // a character at a time
	r.Key("enter")
	r.Hold(500 * time.Millisecond)
	r.MoveToLabel("Search", 600*time.Millisecond)
	r.Scroll(ggui.Pt(0, 240), 800*time.Millisecond)
	r.MoveTo(ggui.Pt(40, 300), 500*time.Millisecond)
	r.Drag(ggui.Pt(240, 300), time.Second) // select a range
	r.Hold(time.Second)
},
```

| Step | |
| --- | --- |
| `MoveTo(p, d)` | Glide to `p`, in logical pixels of the window, over `d`: eased in and out, along a slight curve. The app hears the pointer at each frame, so what it passes over lights up. A `d` of 0 puts it there without drawing a frame. |
| `MoveToLabel(label, d)` | Glide to the middle of what `Probe.Find` finds by `label`: a button by its text, a field by its name or placeholder. |
| `ClickLabel(label)` | Glide to `label`, taking 0.4 to 0.9 seconds by how far it is, and `Click`. |
| `Click()` | Press the left button where the pointer is, hold it for 100 ms, in which the app shows its pressed look and the cursor is drawn a little smaller, let go, and wait 100 ms. A ring fades out around the click over 400 ms. |
| `DoubleClick()`, `RightClick()` | Two clicks a frame apart; a click of the secondary button, as for a context menu. |
| `Drag(to, d)` | Press, glide to `to` over `d`, let go. |
| `Scroll(delta, d)` | Turn the wheel by `delta`, as `Probe.Scroll` takes it, over the pointer, spread over the frames of `d`. |
| `Type(text, perChar)` | Type `text` into what has the focus, a character every `perChar`; characters closer than a frame land in the same one. |
| `Key(chords...)` | Press each chord, as `Probe.Key` does (`"enter"`, `"cmd+a"`), holding 120 ms after each. |
| `HideCursor()`, `ShowCursor()` | Stop and start drawing the cursor; the steps still move it. |
| `Pointer()` | Where the pointer is: the middle of the window before the first pointer step. |

A label nothing has ends the tour with an error that names it. Before the
first pointer step the pointer is in the middle of the window and no cursor
is drawn.

The cursor is drawn over the frame at the tour's scale, as vector shapes in
the usual desktop style, outlined and with a soft shadow: the arrow, black
outlined in white as macOS draws it, the pointing hand where the app asks
for `CursorShapePointer`, and the I-beam where it asks for
`CursorShapeText`. The other shapes are drawn as the arrow. The
`Cursor(false)` option leaves it out while the steps still move, hover and
press as before. Steps taken through `Probe()` move the app's pointer but
not the drawn cursor.

## The window frame

A tour draws the app's content, the size of `Tour.Size`. With
`Frame: record.FrameMac`, or the `Frame(record.FrameMac)` option, each frame
is drawn as a macOS window on a backdrop:

- a title bar 28 points high with the tour's `Title` in the middle, in the
  theme's font at 13 points, semibold;
- the close, minimize and zoom buttons, 12 points across and 8 apart, the
  first 14 points from the left edge, in their colors with a darker rim;
- the corners rounded by 16 points, as macOS 26 rounds a window with a
  title bar (`CornerRadius(pt)`; 10 is the radius of the releases before);
- a hairline border, with a faint light edge inside it in a dark window;
- a shadow in two layers, a wide ambient one and a key light's falling
  below, on the backdrop: `DefaultBackdrop`, a neutral gray a light and a
  dark window both stand out from, or `Backdrop(c)`. 48 points of backdrop
  are left around the window for the shadow (`Margin(pt)`).

The title bar is a shade of the background the app's theme sets
(`Env.Background`, or the `Background` option): darker than a light one,
lighter than a dark one, with the title in the theme's text color for it. A
theme switched in the middle of a tour switches the frame with it. The
frame is drawn at the tour's `Scale`, so `Scale: 2` draws it as crisply as
the content; the image grows by the frame, and `Width` scales the framed
image as a whole. The frame stays the same from frame to frame until the
theme changes, so a GIF still stores only what changed in the content.

### Window styles

`FrameMac` is a window with a plain title bar. The other styles are the
same window, corners, border, shadow and backdrop, with the title bar a
Mac app of another kind shows. All measures are in points, drawn at the
tour's `Scale`; the image is the content plus the bar, plus the margin.

| Style | `--frame` | Bar | Buttons' centres | Title | Separator | For |
| --- | --- | --- | --- | --- | --- | --- |
| `FrameMac` | `mac` | 28 | y 14 | In the middle, 13 semibold | Yes | A plain window: a utility, a document, a settings window. |
| `FrameMacTitleless` | `mac-titleless` | 28 | y 14 | None | Yes | A window that hides its title (`titleVisibility = .hidden`). |
| `FrameMacUnified` | `mac-unified` | 52 | y 26 | After the buttons, 15 bold, with `Subtitle` under it in 11 regular | No | A unified toolbar (`toolbarStyle = .unified`), most apps since macOS 11. |
| `FrameMacCompact` | `mac-compact` | 38 | y 19 | After the buttons, 13 semibold | No | A compact unified toolbar (`.unifiedCompact`). |
| `FrameMacExpanded` | `mac-expanded` | 66: the 28 title row, a 38 toolbar row | y 14 | In the middle of the title row, 13 semibold | Yes, under the toolbar row | An expanded toolbar (`.expanded`), as in Mail's or Xcode's older look. |
| `FrameMacHidden` | `mac-hidden` | 0 | y 14 (+ `ButtonInset`) | None | No | A hidden title bar and full-size content (`Config.TitlebarHidden`), an app that draws its own top. |

The buttons are 12 points across, 8 apart, their centres at x 20, 40 and
60 in every style, and in the middle of the bar's height (of the title row
in the expanded style). A title after the buttons starts 14 points after
the zoom button, at x 80. In a unified toolbar the title's baseline is 2
points above the bar's middle and the subtitle's 12 below it, so the two
lines sit together in the middle of the bar; with no subtitle the title's
line is centred on the buttons. A title too long for the bar is cut where
it would reach the buttons, or as near the right edge. The toolbar row of
the expanded style is left plain: a tour draws the app's content, and the
toolbar items are the system's, not the app's.

The titles are set in the theme's font at those weights. A font of one
weight draws them all in it: a tour draws in Go Regular, as a `Probe`
does, unless its `Setup` sets a font (`ggui.SetDefaultFont`,
`gofont.Enable()` for Go Medium and Go Bold, or opt-in families such as
`fonts/inter` and `fonts/geist` registered with `theme.RegisterFont`; see
[Fonts, emoji, and icons](fonts.md)), so a tour that should look like the app
on a Mac sets the font the app uses.

The content is placed below the bar, the bar the height the table gives;
in `FrameMacHidden` it reaches the window's top edge and the buttons are
drawn over it. Every bar is the same shade of the theme's background as
`FrameMac`'s, and follows the theme when it changes.

The hairline separator between the bar and the content is what macOS
shows: one under a title bar and an expanded toolbar, none under a unified
or compact toolbar, which shows one only once the content scrolls under
it. `Separator(true)` or `Separator(false)` draws or leaves it out in any
style with a bar.

`TitlebarHidden: true` draws any `FrameMac` style as `FrameMacHidden`, as
for an app whose `Config` sets `TitlebarHidden`.

**A sidebar inset from the window** (macOS 26's floating sidebar) is not
a style of its own: the sidebar is part of the app's content, which the
frame does not draw. An app that draws its own sidebar panel inset from
the window, say 8 points from the edges, records with `FrameMacHidden` and
`ButtonInset: 8` (or the `ButtonInset(8)` option), which moves the buttons
8 points right and down, into the panel's top-left corner as they sit in
the window's: the close button's centre at (28, 22). Nothing is drawn
behind them that the app does not draw.

`Backdrop(color.Transparent)` leaves the backdrop clear. PNG frames keep
the transparency, the shadow fading out to nothing, for a page that puts
its own background behind them. A GIF has no partial transparency: there
the clear backdrop is drawn over white.

## ggui record

```sh
ggui record                      # the default tour, to tour.gif
ggui record intro -o docs/intro.gif --width 800 --fps 10
ggui record --list
ggui record --frames /tmp/frames # PNG frames instead of a GIF
ggui record --frame mac --backdrop '#1f2430'  # in a macOS window
ggui record --frame mac-unified               # with a unified toolbar
ggui record --frame none --cursor=false       # the content alone
```

`ggui record` builds the app with its release tags (`build.tags`) and runs
it with `GGUI_RECORD` set. `ggui.Run` sees the variable and, instead of
opening a window, draws the tour asked for and exits. The app's output goes
to the terminal, and ggui ends with what was written:

```text
✓ Wrote tour.gif (42.7 KB, 10 frames, 5.6s, 640×427)
```

The flags are in [the command's reference](cli.md#ggui-record). The app
runs in the project's directory with `dev.env` added to its environment.

An app that registers no tours, or does not import package `record`, says
so on its standard error and exits with status 2; so does one asked for a
tour it does not have, listing those it has.

## From Go

A program can draw a tour itself:

```go
if err := tour.WriteGIF("tour.gif", record.FPS(10), record.Width(800)); err != nil {
	log.Fatal(err)
}
```

`WriteGIF` and `WritePNGs` take options matching the command's flags —
`FPS`, `Width`, `Colors`, `Dither`, `Loop`, `Lossy`, `Frame`, `Backdrop`,
and `Cursor` — plus Go-only options with no CLI flag: `Background` (fill
each frame with a color instead of the background the tour's `Env` holds,
`ggui.Env.Background`), `Optimize` (described under [the GIF](#the-gif)),
and the frame layout options `CornerRadius`, `Margin`, `Separator`, and
`ButtonInset` (described under [the window frame](#the-window-frame)).

Drawing needs the renderer, which the package brings up with no window:
call them from the main goroutine, as `ggui.Run` is called, before any app
runs. The renderer stays up for the rest of the process, so a process that
records does not go on to run the app; `ggui.Run` and `record.Main` exit
when they are done. In a test run by `ggtest.Main` the renderer is already
running, inside a frame, and a tour draws in that frame, so a test can
record a tour and check the frames. A test under `ggtest.MainHeadless` has
no renderer to draw with.

## The GIF

The GIF is written in pure Go, in two passes, so nothing else needs to be
installed:

- **One palette** of up to 256 colors for the whole recording, chosen from
  the colors of every frame. A UI of fewer colors than the palette holds
  keeps every one exactly. Otherwise the colors that cover a 2000th of the
  pixels or more, up to a quarter of the palette, are kept exactly, so the
  flat backgrounds, a frame's title bar and its buttons never shift; the
  rest are chosen in Oklab, where a distance looks about as large wherever
  it is, so dark gradients and shadows get their share: a median cut that
  splits the colors where the error drops most, then a few rounds of
  k-means. Colors are mapped to it in Oklab too, each looked up once.
- **Only what changed**: each frame after the first holds the smallest
  rectangle around the pixels whose palette color changed, drawn over the
  frame before. Inside it, a pixel that shows the same color already may
  be written as a transparent index the frame does not otherwise use,
  whichever continues the LZW string being matched, so the unchanged parts
  compress to almost nothing (as `gifsicle -O2` does). A frame that changes
  nothing lengthens the one before instead of adding another. A tour that
  holds still for seconds costs a few bytes for them.
- **Bounded memory**: the first pass draws the frames, counts their colors
  and spools the changed rectangles to a temporary file; the second maps
  them to the palette and writes the GIF a frame at a time.

| Option | Flag | |
| --- | --- | --- |
| `FPS(n)` | `--fps n` | Keep `n` frames a second, those on screen at each 1/n. The tour draws 15 by default; 10 plays as smoothly as most UI needs and makes a smaller file. A GIF plays at most 50. |
| `Width(px)` | `--width px` | Scale down to `px` wide with an area filter, which keeps thin lines and text legible. Drawing at `Scale: 2` and scaling to the width wanted reads better than drawing at 1. |
| `Colors(n)` | `--colors n` | A smaller palette, 2 to 256: a smaller file, and gradients band sooner. |
| `Dither(s)` | `--dither s` | Ordered (Bayer) dithering, 0 to 1, where a color falls between two of the palette's. Off by default: flat UI needs none, and dithering costs size. 0.2 to 0.4 smooths gradients and soft shadows that band. The pattern is fixed to the pixel, so a part that does not change does not shimmer. |
| `Loop(n)` | `--loop n` | Play `n` times; 0, the default, loops for ever. |
| `Lossy(n)` | `--lossy n` | Let the compression write a pixel in a palette color up to n/1000 from its own in Oklab, or leave it as it shows, where that makes an LZW string longer, as `gifsicle --lossy` does; 0 to 200, 0 (exact) by default. Oklab's 0.02 is about the least difference the eye sees, so 20 to 40 is hard to see in UI; more speckles gradients and shadows. |
| `Optimize(level)` | | 1, the default, is the above: exactly what level 0 shows, which writes each changed rectangle whole, in fewer bytes. 2 also splits a frame whose changes are two regions far apart, as the cursor and a control, into two frames when that is smaller; the first shows for 20 ms, the least a browser shows a frame for, so one region changes that much before the other. |

The demo recording behind these numbers is 360×220 at `Scale: 2`, 5.9
seconds at 15 frames a second: a button clicked, a name typed and a dark
mode switched on. Sizes in KB:

| | Baseline (RGB, level 0) | Level 1 | Level 2 | `Lossy(30)` | Level 2, `Lossy(30)` |
| --- | --- | --- | --- | --- | --- |
| `FrameMac` and the cursor, 912×688 | 130.8 | 125.1 | 115.7 | 92.2 | 83.6 |
| The same, `Width(720)` | 116.7 | 107.2 | 100.5 | 78.1 | 71.9 |
| The content alone, 720×440 | 37.7 | 33.8 | 33.7 | 27.9 | 27.9 |
| The same, `Width(360)` | 17.0 | 15.4 | 15.4 | 13.7 | 13.7 |

The baseline column writes each changed rectangle whole with an RGB palette.
Encoding takes no measurable time beside drawing: each of these recordings
takes as long at every setting, 0.2 to 0.3 seconds, or about a second for
`Width(720)`, which scales the framed image.

## PNG frames and ffmpeg

`--frames dir`, or `WritePNGs`, writes `0000.png`, `0001.png` and on, as
many a second as `--fps` keeps (the tour's FPS by default). For a video:

```sh
ggui record --frames /tmp/frames --fps 30
ffmpeg -framerate 30 -i /tmp/frames/%04d.png -pix_fmt yuv420p tour.mp4
```

or for a GIF with ffmpeg's own palette, which also dithers by error
diffusion:

```sh
ffmpeg -framerate 15 -i /tmp/frames/%04d.png \
	-vf "split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=5:diff_mode=rectangle" \
	tour.gif
```

## What is not drawn

A tour draws its tree, as a window's content, and nothing around it:

- **No operating-system chrome** but the one the `FrameMac` styles draw:
  a macOS window on any platform, not the system's own title bar, and
  none on Windows' or Linux's pattern. A toolbar's items, as a sidebar
  toggle or a search field, are the system's and are not drawn.
- **No native menus or tray icon**: the macOS menu bar, a native context
  menu and a tray icon's menu belong to the system. Menus and popovers
  ggui draws itself, in the window, are drawn.
- **No native dialogs**: the probe's file dialogs are stand-ins that pick
  nothing.
- **The cursor only where the Recorder moves it**: the cursor drawn is the
  one the pointer steps move, in three shapes; input given through
  `Probe()` shows its hover and press but no cursor.
- **One window**: the tour is one tree in one probe, the size of
  `Tour.Size`. A window the app opens with `OpenWindow` is not part of it;
  a multi-window app records each window it wants to show as a tour of its
  own, or composes them in one tree for the tour.
- **Desktop only**: a page in a browser cannot record. Record from a
  desktop build; on Linux the renderer needs EGL, as
  [the headless tests](testing.md) do.
