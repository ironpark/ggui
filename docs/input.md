# Input and focus

[Documentation](README.md) · [Project README](../README.md)

Handle pointer and keyboard input, manage focus, and preserve interaction state across rebuilds.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Input](#input)
- [Scrolling](#scrolling)
- [Touch input](#touch-input)
- [Pointer and keyboard events](#pointer-and-keyboard-events)
- [Shortcuts](#shortcuts)
- [Drag and drop](#drag-and-drop)
- [Native file dialogs](#native-file-dialogs)
- [Focus scopes](#focus-scopes)
- [Roles and labels](#roles-and-labels)
- [Custom input handlers](#custom-input-handlers)
- [Keeping interaction state across rebuilds](#keeping-interaction-state-across-rebuilds)

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

## Scrolling

`Scroll(child)` gives its child `Unbounded` height (or width, with
`.Horizontal()`), shows a window onto it, moves that window with the wheel and
clips both drawing and hit regions to the window. The offset carries across
a rebuild; `.BindOffset(sig)` binds it to a `StateValue[float64]` for programmatic
scrolling; `.Speed(px)` and `.Bar(color)` tune it. Widgets that fill their space
fall back to their content size on an unbounded axis, so `Center`, `Expanded`
and `.Justify` inside a `Scroll` do not blow up.

## Touch input

Touch uses the same pointer handlers as mouse input. The first finger becomes
the primary pointer; additional fingers do not create independent presses.
After the primary finger lifts, a new press waits until all fingers lift.

Dragging beyond the scroll threshold lets a scroll region take over the gesture
and prevents a tap on the original child. Scrolling can continue with momentum
after release. A custom control that must retain a touch drag can implement
`TouchDragCapturer`; return true from `CaptureTouchDrag()` while it owns that
gesture. Carousel uses this to keep slide dragging within the control.

## Pointer and keyboard events

`Pointer` has `OnTap`, `OnDown`, `OnUp`, `OnMove`, `OnDrag`, `OnEnter`,
`OnExit`, `OnHover` and `OnScroll`, and `.Cursor(shape)` sets the mouse
cursor over it; `Tap(child, fn)` is the one-callback shortcut. A tap is a
press and a release inside the same region, matched by `Rect`, so a tree
rebuilt in between still completes it. The region that took a press captures
the pointer: it gets `OnDrag` every frame until the release, wherever the
cursor went, which is what a slider or a text selection needs.

`Focus(child)`
takes keyboard focus when clicked and delivers `OnKey`, `OnText` and
`OnFocus`; a held key repeats, and `KeyEvent.Mods` carries Shift, Ctrl, Alt
and Meta (`Mods.Cmd()` is ⌘ on macOS and Ctrl elsewhere). Tab and Shift+Tab
move focus through the key regions in paint order; focus that arrived that
way is reported with `Key` set to `KeyTab`, which is when the controls draw
a focus ring, and a `Scroll` around the new target scrolls it into view.
Buttons press on Space or Enter, toggles flip, sliders step with the
arrows.

## Shortcuts

Shortcuts are chords: `app.Shortcut("cmd+s", save)` runs before the
focused widget and takes the key from it, as every chord with a modifier
does. A bare key such as `"space"` reaches the focused widget first and runs
the shortcut only when the widget did not consume it, so Space on a focused
button presses the button and a text field keeps every key but Escape;
`.Exclusive()` on the handle makes a bare key run first too. A widget says
what it consumes through `KeyConsumer`; `Interactive` claims Space and
Enter and the controls with more keys claim those. `ParseChord` reads the
names, `KeyEvent.Is(chord)` matches one in a handler, and `App.OnKey` stays
for what a chord cannot say.

`Shortcut`, `OnKey`, `OnDrop`, `OnFrame`, `Post`, `Perform`, `Announce`,
`Semantics` and `Close` are the same on `App` and `Probe`, and the `Host` interface
names that set, so a function that registers shortcuts takes a `ggui.Host`
and serves both main and its tests.

Keys, cursor shapes and mouse buttons carry ggui's own names: `ggui.KeyTab`,
`ggui.CursorShapePointer`, `ggui.MouseButtonRight`. They are aliases for
Ebitengine's, so they are the same values of the same types and an
`ebiten.Key` still works wherever one is wanted; what they buy is that a
widget, or an app, imports `ggui` alone.

## Drag and drop

Files dragged from the desktop and dropped onto the window arrive as a
`DropEvent`: the cursor position and a `DroppedFile` per item, each with
its `Name`, its absolute `Path` where the platform has one, whether it is a
`Dir`, and `Open()` to read it. In a browser there is no path, so `Open` is
the one way in; `ev.Paths()` collects the paths that exist.

A drop is routed like a pointer event. `Pointer(child).OnDrop(fn)` makes
the child a drop zone: the topmost zone under the cursor takes the drop,
and a `Pointer` without `OnDrop` lets it fall through to what is beneath.
A drop no zone took reaches the handlers registered with `App.OnDrop`,
which is where an app that accepts a drop anywhere listens.

```go
zone := ggui.Pointer(ui.Card(ggui.Text("Drop images here"))).
	OnDrop(func(ev ggui.DropEvent) {
		for _, f := range ev.Files {
			if !f.Dir {
				importImage(f)
			}
		}
	})
app.OnDrop(func(ev ggui.DropEvent) { openAll(ev.Paths()) })
```

A custom handler implements `DropHandler` beside `PointerHandler`; the
region registered with `HitPointer` then receives `HandleDrop`, and returns
false to let the drop pass. There is no drag-over event: the platform
reports the drop itself and nothing before it, so a zone cannot highlight
while a file hovers over it.

In a test, `Probe.Drop(pos, fsys)` drops the root entries of any `fs.FS`, a
`testing/fstest.MapFS` being the easiest, and `Probe.DropPaths(pos,
paths...)` drops real files with their paths; see
[examples/files](../examples/files) for both.

## Native file dialogs

The `runtime` package opens the platform's own file chooser, the one
with the user's favourites and recent places, rather than a browser drawn
by ggui. `Open`, `OpenMultiple`, `PickFolder` and `Save` each take an
`Options` of title, starting directory, proposed file name and type
`Filters`, and return the chosen path or `ErrCanceled`.

```go
path, err := runtime.OpenFile(runtime.FileDialog{
	Title:   "Open a file",
	Filters: []runtime.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg"}}},
})
if errors.Is(err, runtime.ErrCanceled) {
	return
}
```

Every call is modal and blocks until the user picks or cancels. On macOS
the panel is a sheet attached to the app's window, on Windows a modal
dialog; both run on the window's thread, so the window stops repainting
while one is up, which is how a modal dialog behaves there;
elsewhere the panel is the `zenity` or `kdialog` program found on `PATH`,
and where neither is, or in a browser, every call is `ErrUnsupported`. A
dialog needs a running `App`: ask for one from a handler, a shortcut or
posted work, not before `Run`.

`runtime.SetFilePicker` replaces the dialogs with any `FilePicker`; the
`runtime.StubFilePicker` answers every dialog with fixed paths and records what
was asked, which is what a headless test installs.

## Focus scopes

An open `Popup` and a `ui.Dialog` paint their content
through `dst.FocusTrap`: while it shows, Tab cycles inside it, focus is
moved in when it opens and returned to the opener when it closes, and an
Escape the focused widget did not consume closes it. `ui.Dialog(open,
content)` is a centered modal on a scrim that takes the clicks, with
`.Title`, `.Width` and `.OnClose`.

## Roles and labels

Every control carries a `Role` and a name: a button's text, a
checkbox's label, a field's `Name` or placeholder; `ButtonOf`, `Slider`
and `Select` take one through `.Name`. `.BindName(sig)` on a button or
field, or `BindName` on any control built on `Interactive`, binds the name
to a signal, so a row named after an editable title stays current without
a rebuild. The inspector shows them, and tests find controls by them.

## Custom input handlers

To make your own widget interactive, implement `PointerHandler` or
`KeyHandler` and call `dst.HitPointer(r, w)`, `dst.HitKey(r, w)` or
`dst.HitCursor(r, shape)` from `Paint`; calls for the same `Rect` merge into
one region. A `KeyHandler` that also implements `TickHandler` runs once per
frame while focused, which is how `TextInput` drives the IME. `dst.Clip(r)`
returns a Canvas that draws and registers regions only inside `r`,
`dst.Pointer()` is where the cursor is, and `dst.Overlay(fn)` paints above
the tree once it is done.

## Keeping interaction state across rebuilds

A rebuild replaces widgets, and with them the state they hold. A handler that
implements `Adopter` is handed the handler that held the same region in the
previous frame as it registers its own, so it can copy hover, press, a caret
or an animation in flight: the built-in controls, `TextInput`, `Scroll` and
`Popup` all do, which is why flipping a `ui.Switch` that rebuilds its own
subtree still slides the knob. The region is matched by the handler's
identity when it implements `Identified`, else by `Rect`; `.Key(k)` on a
control, `TextInput`, `Scroll` or `Popup` sets one, so a widget rebuilt and
moved in the same frame keeps its state. Inside a `Component`, keyed row, or `Key` branch every one of those gets an identity for free, the component's
mount instance plus its place in construction order, so a keyed form keeps focus and
carets through its own rebuilds with no keys on the fields.

`ggui.Interactive` is the shared body of a
control: embed it, call `Hit` from `Paint` and `Pointer` and `Keyboard` from
the handlers, and hover, press, focus, the focus ring and adoption come with
it; `ui` is built on it. A custom control calls `Interactive.SetKey(id)` to
assign an explicit identity and may expose its own fluent `Key(id)` method
that returns the concrete control.
