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

- [Writing a tour](#writing-a-tour)
- [Driving the app](#driving-the-app)
- [ggui record](#ggui-record)
- [From Go](#from-go)
- [The GIF](#the-gif)
- [PNG frames and ffmpeg](#png-frames-and-ffmpeg)
- [What is not drawn](#what-is-not-drawn)

## Writing a tour

A tour lives in the app, beside what it shows. Register it before
`ggui.Run`, from an `init` function or from `main`:

```go
import "github.com/ironpark/ggui/record"

func init() {
	var m *Model
	record.Register(record.Tour{
		Size:  ggui.Sz(800, 500), // the window drawn, in logical pixels
		Scale: 2,                 // drawn at 1600×1000, as on a Retina display
		Setup: func(p *ggui.Probe) {
			m = NewModel(demoData()) // demo state, not the user's
			uitheme.Bind(ggui.Const(true), uitheme.Dark(), uitheme.Default())
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

## ggui record

```sh
ggui record                      # the default tour, to tour.gif
ggui record intro -o docs/intro.gif --width 800 --fps 10
ggui record --list
ggui record --frames /tmp/frames # PNG frames instead of a GIF
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

`WriteGIF` and `WritePNGs` take the same options as the command's flags:
`FPS`, `Width`, `Colors`, `Dither`, `Loop`, and `Background`, which fills
each frame with a color of your choosing instead of the background the
tour's `Env` holds (`ggui.Env.Background`, which a theme sets).

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
  the colors of every frame: a median cut that splits the colors where the
  error drops most, then a few rounds of k-means. A UI of fewer colors
  than the palette holds keeps every one exactly; a flat background
  never shifts between frames.
- **Only what changed**: each frame after the first is the rectangle that
  differs from the frame before, drawn over it. A frame that changes
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

- **No operating-system chrome**: no title bar, window frame or shadow.
  Put the frame around the GIF in the README, or draw one in the tree.
- **No native menus or tray icon**: the macOS menu bar, a native context
  menu and a tray icon's menu belong to the system. Menus and popovers
  ggui draws itself, in the window, are drawn.
- **No native dialogs**: the probe's file dialogs are stand-ins that pick
  nothing.
- **No pointer**: the cursor is the system's. Hover and press states show;
  the arrow does not.
- **One window**: the tour is one tree in one probe, the size of
  `Tour.Size`. A window the app opens with `OpenWindow` is not part of it;
  a multi-window app records each window it wants to show as a tour of its
  own, or composes them in one tree for the tour.
- **Desktop only**: a page in a browser cannot record. Record from a
  desktop build; on Linux the renderer needs EGL, as
  [the headless tests](testing.md) do.
