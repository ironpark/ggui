# Input routing: regions as a tree (proposal)

[Documentation](../README.md) · [Input and focus](../input.md) · [API conventions](../api-conventions.md)

Status: implemented, phases 1 to 7. The owner's answers and the
implementer's decisions are recorded as **Decided** notes under
[open questions](#risks-and-open-questions) and beside what they decide;
where the code differs from the text around a note, the note says how and
why.

## On this page

- [Problem](#problem)
- [The current model](#the-current-model)
- [How other toolkits route input](#how-other-toolkits-route-input)
- [The design in brief](#the-design-in-brief)
- [The region tree](#the-region-tree)
- [Pointer dispatch](#pointer-dispatch)
- [Hover](#hover)
- [What goes on inside a region](#what-goes-on-inside-a-region)
- [Keyboard and focus](#keyboard-and-focus)
- [Presses, capture and drags](#presses-capture-and-drags)
- [Wheel and scroll chaining](#wheel-and-scroll-chaining)
- [Context menus, cursors and observers](#context-menus-cursors-and-observers)
- [Accessibility](#accessibility)
- [API](#api)
- [Compatibility and migration](#compatibility-and-migration)
- [Implementation plan](#implementation-plan)
- [Rejected alternatives](#rejected-alternatives)
- [Risks and open questions](#risks-and-open-questions)

## Problem

Hit regions are a flat list in paint order. A region does not know which
region it was painted inside, so input cannot ask "is the pointer inside this
row", only "is this the topmost region that took the event". Everything that
needs the first question is written by hand today, in applications and in
`ui` itself.

The cases that exposed it, from mako's tab bar and panes:

| Case | What was wanted | What happens today |
| --- | --- | --- |
| A tab's close button shows while the pointer is over the tab | `widget.Pointer(tab).OnHover(hover.Set)` and a button built from `hover` | The tab's own button, and the close button inside it, claim `PointerMove`, so they take the hover and the wrapper never hears `PointerEnter`. mako reads `Canvas.Pointer()` in `Paint` (`roundTab`, `hoverShown` in `internal/gui/view.go`) and paints the button only then. That is not state: the hidden button leaves the Tab order and the accessibility tree. |
| A sidebar row shows a pin button while hovered | `widget.Pointer(row).OnHover(hover.Set)` (`pinButton` in `sidebartree.go`) | Works only over the parts of the row no control covers. Over the row's own button, or over the pin button once shown, the row is told it was left. |
| A press anywhere in a pane makes the pane current | A listener that hears the press before whatever takes it, and takes nothing | `pressWatch` in `layoutview.go` registers a region after its child, so it sits on top, and returns false from `HandlePointer`. `Canvas.ObserveInput` is not usable: it hears the wheel and the keyboard too. |
| A tab drags, and holds a close button | The button clicks; a press that moves drags the tab; a control that drags by itself keeps its drag | Works for a button, by the 4-pixel threshold. A slider or a text selection inside a `DragSource` loses its drag to the source. A press on a popup painted over a source can drag the source beneath it. |
| Keys of a focused control reach shortcuts of the pane it is in | Shortcuts that hold while focus is inside a pane | Shortcuts are the window's, or last as long as a widget is painted (`Canvas.Shortcut`). mako keeps "the current pane" in its model and routes global shortcuts by it. |
| A context menu on a row that holds buttons | The innermost menu opens, over a button too | `ui.ContextMenu` registers its pointer region after its child and keeps a list of the `Rect`s of the context menus inside it (`Canvas.Enclose`), to pass their presses on. |
| A scroll inside a scroll | The inner one scrolls; a gesture stays with the view it began in | A trackpad's momentum that reaches the inner view's end moves the outer one. |

`ui` holds the same workarounds: `FieldCardWidget.Paint` asks its control
whether it is hovered; `SidebarWidget.showsAction`, `CodeBlockWidget.Paint`,
`TooltipWidget.Paint`, `HoverCardWidget.Paint` and `CarouselWidget.Paint`
read `Canvas.Pointer()` and test their own `Rect`, which also holds when a
popup covers them.

## The current model

All of this is in the root package unless a path says otherwise.

**Regions.** A widget registers a `Rect` from `Paint`: `Canvas.HitPointer`,
`HitKey`, `HitCursor` (canvas.go), `HitDrag` (drag.go), or
`Interactive.Hit` and `HitReach` (widget.go), which register one region with
a pointer handler, a key handler and a cursor. `Canvas.add` trims the region
to the clip, places it when under a `Transform` (`paintSpace.placeRegion`),
hands an `Adopter` the handler that held the region last frame
(`frameState.adopt`), and appends a `hitRegion` to `frameState.hits`. A
region registered at the `Rect` of the one just before it, adding only what
that one lacks, is folded into it (`hitRegion.merge`): `Pointer(Focus(w))`
is one region.

A `hitRegion` (input.go) holds the `Rect` (clipped, unclipped and `reach`),
the handlers, the handler's `HitID`, the focus scope, the `Group`, a role and
label for `Probe.Find`, and the `paintSpace`. It holds no link to any other
region.

**Frames.** `frameLoop.paint` (loop.go) swaps the region buffers, paints the
tree and the overlays, and hands the list to input with
`inputState.setRegions`. Input routes the next frame's events to that list.
A frame painted in part (damage.go) paints through an inert `Canvas`, which
registers nothing: the regions of the last whole frame stand.

**Order of a frame's input.** `inputState.dispatch` runs `updateTrap`,
`notifyObservers`, `updateHover`, `updateCursor`, `updateDrag`, `panTouch`,
`dispatchPointer`, `updateAppDrag`, `dispatchDrop`, `consumeBindings` and
`dispatchKeys`, in that order.

**Pointer events.** `inputState.send` walks the regions from the last
painted to the first (`topmost`), offers the event to every region with a
pointer handler that contains the point, and stops at the first whose
`HandlePointer` returns true. That is the whole of the routing. An ancestor
hears an event its child declined because it registered before the child and
so lies beneath it; an unrelated region beneath hears it for the same
reason.

**Hover.** `updateHover` sends a `PointerMove` through `send` every frame.
The one region that claims it is the hovered region. When that region is not
the one hovered before (`continues`), the old one hears `PointerExit` and the
new one `PointerEnter`. A region rebuilt under a still pointer, the same
region with a new handler, hears `PointerEnter` and the handler it replaced
hears nothing.

**Presses.** `dispatchPointer` sends `PointerDown` through `send`; the region
that consumed it is `pressed`, and receives `PointerDrag` every frame and
then `PointerUp` directly, wherever the pointer is, and `PointerTap` if the
release is inside it. Each press also moves focus to the topmost key region
under the pointer (`findKey`), or clears it.

**Identity across frames.** Input keeps copies of the regions it follows
(`keep`, `regionSlot`), never pointers into the buffer. A kept region is
found again in the current list by `continuing`: the region with the same
handler (`regionOf`, with a remembered index), else the topmost with the same
`HitID`, else, for a handler with no ID, the topmost at the same `Rect`,
which is a rebuilt widget.

**Keys.** `consumeBindings` (keys.go) takes Tab, `App.OnKey` hooks and the
chords that run before the focused widget (a modifier other than Shift, or
`Exclusive`), unless the focused handler's `ChordClaimer` claims the chord.
`dispatchKeys` hands the rest to the focused handler. `afterFocused` then
runs, for each press the handler's `KeyConsumer` does not consume, the focus
trap's Escape and the bare-key shortcuts. No key reaches a region other than
the focused one.

**Focus scopes.** `Canvas.FocusTrap` marks the regions painted through it
with a `focusScope`. While one is painted (`activeScope`), Tab cycles inside
it, focus moves in and back (`updateTrap`), and observers and painted
shortcuts outside it are silent.

**Observers.** `Canvas.ObserveInput(r, notify)` is told, before dispatch, of
any press or wheel at a point in `r` and of any key while the focused
region's `Rect` overlaps `r` (`notifyObservers`). It is geometric, and takes
nothing.

**Drags.** `pressDrag` (drag.go) notes the topmost `HitDrag` region under a
left press, whatever took the press. `updateAppDrag` starts the drag once
the pointer has moved `dragThreshold` pixels, takes the press from the
region that held it (`releasePress`: a `PointerUp` with no tap), and follows
the topmost `DropTarget` under the pointer that accepts the value.

**Repainting.** `inputState.deliver` notes the `Rect` of every region an
event was handed to; the frame paints those again (`Window.frameOn`,
`addRepaints`). A `PointerMove` is noted only when it was claimed and the
pointer moved. A widget that reads `Canvas.Pointer()` in `Paint` is painted
again on every move.

**Semantics.** The accessibility tree is a separate array built explicitly
(`Canvas.Node`, `Leaf`, `Describe` in semantics.go); a `semNode` has a
parent index. It is not derived from the hit regions and this proposal does
not change that.

## How other toolkits route input

| | Target | Who else hears | "Handled" | Before the target | Hover |
| --- | --- | --- | --- | --- | --- |
| Web DOM | Topmost element under the point; `pointer-events: none` passes | Ancestors only, capture then bubble | `stopPropagation` stops the walk; `preventDefault` cancels the default action and stops nothing | Capture phase, root first | `:hover` and `mouseenter`/`mouseleave` hold for the element and its ancestors; `mouseover`/`mouseout` fire on every crossing |
| Flutter | Hit test collects a path, deepest first; `HitTestBehavior` decides whether a box hides what is behind it | Every entry on the path gets the raw event | None for raw events; gestures compete in an arena, one winner | None | `MouseRegion` enters and exits per region; nested regions are hovered together; `opaque` hides regions behind, not ancestors |
| Android View | `dispatchTouchEvent`, root down | The parent, when the child returned false for the down | Boolean from `onTouchEvent` | `onInterceptTouchEvent`: a parent takes the gesture, the child hears cancel | One hovered view: a group is hovered only while no child takes the hover |
| GTK 4 | Widget under the pointer | Ancestors: capture, target, bubble phases per controller | Handler returns stop or propagate | Capture phase | `GtkEventControllerMotion` has `contains-pointer` (widget or descendant) and `is-pointer` (widget itself) |
| Qt | Widget under the cursor, or the grabber | The parent, when the event was ignored | `accept()` / `ignore()` | Event filters | Enter and leave are sent along the path to the common ancestor: entering a child does not leave the parent |
| egui, Dear ImGui | Each widget asks as it is drawn | Nobody | The topmost item is the hovered one | None | One hovered item. A button over a selectable row needs an explicit overlap opt-in; egui splits `hovered()` from `contains_pointer()` |

Notes on what matters for ggui:

- **A consumed event stops** in Android, GTK and Qt, by a boolean. The DOM
  is the exception: there an ancestor hears a click its child handled unless
  the child stops it, which is why a clickable row with a button inside needs
  `stopPropagation`. ggui's `HandlePointer` returning true already means what
  Android's and GTK's boolean means.
- **Hover is hierarchical in the DOM, Qt, GTK and Flutter**: the parent
  stays hovered while the pointer is over a child. Android and the
  immediate-mode toolkits have one hovered item, and egui and Dear ImGui
  both grew an escape hatch for a control inside a row. ggui's exclusive
  hover is theirs, while its `ui` styles are taken from CSS.
- **Looking before the target** exists in every retained toolkit: capture
  phase, `onInterceptTouchEvent`, event filters. Flutter has none and uses
  the arena instead.
- **Fall-through to what is painted beneath but is not an ancestor** exists
  only in Flutter (`HitTestBehavior.translucent`) and as the DOM's
  `pointer-events: none`. Both are opt-in. ggui does it for every event a
  region declines.
- **Pointer capture.** The DOM's `setPointerCapture`, Qt's implicit grab and
  Android's "the view that took the down gets the rest" are the same thing:
  ggui's `pressed`.
- **Focus.** `focusin`/`focusout` bubble, `:focus-within` holds for
  ancestors; GTK has `contains-focus`. ggui has `Canvas.FocusWithin(r)`,
  geometric, and `Canvas.PaintFocus`, exact but only during a paint.
- **Wheel.** The DOM scrolls the nearest scrollable ancestor that can move,
  chains outward at an end unless `overscroll-behavior: contain`, and latches
  a trackpad gesture to the view it began in. Qt offers an ignored wheel
  event to the parent.
- **Gestures.** Flutter's arena and UIKit's recognizers settle tap against
  drag by letting both watch and one win. ggui settles it with a threshold:
  a press is a click until it moves 4 pixels over a drag source.

What ggui takes: the tree and hierarchical hover from the DOM, Qt and GTK;
the boolean that consumes from Android and GTK; a pass before the target
from capture and `onInterceptTouchEvent`; `is-pointer` beside
`contains-pointer` from GTK; gesture latching for the wheel from the
browsers. It does not take the DOM's separate `stopPropagation`, nor an
arena.

## The design in brief

1. **Regions form a tree.** Each region records the region it was painted
   inside. The link is found at registration from the scopes `Canvas.Paint`
   already opens, costs one integer per region, and is rebuilt with the list
   on every whole paint. Nothing about it persists between frames.
2. **Delivery order does not change.** An event is still offered topmost
   first to the regions under the pointer, and true still consumes it. A
   region is always registered after the region it is inside, so this order
   was already a bubble order; the tree adds what the flat list could not
   say.
3. **Hover follows the tree.** The topmost region that claims `PointerMove`
   is the hot region. The regions it is inside that also claim the move are
   hovered with it. `PointerEnter` and `PointerExit` are sent for the
   difference between one frame's path and the next, so a row is entered
   once and left once however the pointer moves among its children.
4. **A region can hear what goes on inside it.** `Canvas.HitWithin`
   registers a region whose handler is offered presses and wheel movement
   before the regions inside it, is told when focus comes inside or leaves,
   and is offered the key presses of the focused widget inside it.
5. **Drags are resolved through the tree.** A press starts a drag only from
   a source the pressed region is inside of, or that is inside the pressed
   region. A pressed control can keep its drag.
6. **A scroll gesture stays with the view that took it.**

Press capture, cursors, drop targets, focus traps, the Tab order,
`ObserveInput` and the semantics tree stay as they are.

## The region tree

### What "inside" means

Region A is *inside* region B when B is on A's chain: A's parent, that
region's parent, and so on. The parent of a region is decided when it is
registered:

- A region is inside the region registered by the nearest widget around it
  that registered one. Widgets that register nothing, `Column`, `Box`,
  `Reactive`, are passed over. "Around" is by `Canvas.Paint`: the widget
  whose `Paint` called `dst.Paint(child, r)`, directly or through any number
  of widgets.
- A container therefore registers before it paints its children, as
  `HitPointer`'s documentation already asks. A region registered after the
  children were painted, as `pressWatch` and `ui.ContextMenu` do, is beside
  them, not around them.
- When one widget registers several regions in one `Paint`, a later region
  is inside the nearest earlier one of that widget whose unclipped `Rect`
  covers its own, and beside the others. A scroll's thumb is inside the
  scroll; the day cells of a calendar are beside each other; a table that
  registers a row and then paints the row's cells has the cells inside the
  row.
- Regions painted by an `Overlay` start a chain of their own: a popup is
  inside nothing. (See [open questions](#risks-and-open-questions).)

Position plays no part beyond the third rule. A child painted outside its
parent's `Rect` is still inside the parent's region; a region painted over
another by a sibling is not inside it.

### Data

```go
type hitRegion struct {
	// ...
	parent int32 // index in the same list, plus one, of the region this one is inside; 0 for none
	within any   // from HitWithin; see below
	seen   uint32 // the event this region's within handler was last offered; see offerWithin
}
```

`parent` is an index plus one, as `semNode.parent` is, so the zero value is
"none" and a list a test builds by hand is a list of roots.

> **Decided** (implementer 1, 2): `hitRegion` stays 320 bytes, five cache
> lines. A 328-byte region made `BenchmarkDispatchStill/2000` 24% slower,
> the scan touching a line more per region. So `parent int32` and a
> `within bool` flag sit in the padding after `shaped`; the within handler
> is kept beside the list, as `withinEntry{at, h}` in `frameState.withins`
> (swapped with `hits` every paint) and `inputState.withins`, found by
> binary search; and there is no `seen` field: `inputState.offered`, a
> reused slice of the region indices the event being sent has been
> offered to, does its work.

**Invariant.** A region's parent is registered before it: `parent-1` is
less than its own index. Scanning from the end of the list therefore reaches
a region before any region it is inside.

### Building it during paint

`frameState` gets a cursor of two indices, both plus one:

```go
// regionCursor is where in the region tree the paint is.
type regionCursor struct {
	at    int32 // the last region the widget painting registered, else outer
	outer int32 // the region the widget painting is inside
}
```

- `frameState.openScope` saves the cursor in the `paintScope` it pushes and
  sets `outer = at`. `closeScope` restores the saved cursor. The cursor is in
  `frameState`, not in the scope, so that a `Paint` called with no scope
  open, as the tests' `paintFrame` calls the root's, still works.
- `Canvas.add`, after clipping and placing, either merges the region into
  the previous one or appends it:

  ```go
  i := len(f.hits)
  if i > 0 && f.hits[i-1].merge(h) {
  	i--
  } else {
  	h.parent = f.parentFor(&h)
  	f.hits = append(f.hits, h)
  }
  f.cur.at = int32(i + 1)
  ```

  ```go
  // parentFor returns the region h is inside: the nearest region the widget
  // painting registered itself that covers h, else the region the widget is
  // inside.
  func (f *frameState) parentFor(h *hitRegion) int32 {
  	for p := f.cur.at; p != f.cur.outer && p != 0; p = f.hits[p-1].parent {
  		if q := &f.hits[p-1]; q.space == h.space && covers(q.full, h.full) {
  			return p
  		}
  	}
  	return f.cur.outer
  }
  ```

  `covers` is the one in semantics.go. Regions in different paint spaces are
  never inside each other by the third rule.
- `Canvas.paintOverlays` saves the cursor, zeroes it before each overlay's
  `fn`, and restores it.
- `frameLoop.beginPaint` zeroes the cursor. `parentFor` also starts again
  from a zero cursor when the list was emptied under it without a paint
  beginning, as a benchmark that reuses a `Canvas` does.

A region dropped by `add`, because the `Canvas` is inert or the region lies
wholly outside the clip, does not move the cursor: what is painted after it
nests under the next region out.

A merge leaves the merged region's parent as it was and moves the cursor to
it, so `Pointer(Focus(w))` is one node and what `w` registers is inside it.

A partial frame registers nothing and so leaves the tree of the last whole
frame, as it leaves the list.

### Reading it

```go
// parentOf returns the region r is inside, or nil. r must point into
// in.regions: a kept copy's parent indexes a list that is gone.
func (in *inputState) parentOf(r *hitRegion) *hitRegion
```

`keep`, `regionSlot.keep` and the path buffers below zero `parent` and
`seen` in the copies they make, so that following one from a copy fails at
once rather than reads another frame's list. Input finds the current region
for a kept one with `continuing`, as it does now, and walks from there.

### Across rebuilds

Nothing is carried. The list and its links are made again by every whole
paint. What input follows from frame to frame are *paths*: copies of the
regions on a chain, matched to the next frame's as a single kept region is
matched today, by handler, then by `HitID`, then, for a handler with no ID,
by `Rect`:

```go
// sameRegion reports whether now, of this frame, is the region prev was
// copied from, by the pointer handler when key is unset and by the key
// handler otherwise.
func sameRegion(now, prev *hitRegion, key bool) bool
```

A path is kept in two buffers that alternate, as `regionSlot` keeps one
region, so keeping one allocates nothing once the buffers have grown:

```go
// regionPath keeps the regions of a chain input follows across frames.
type regionPath struct {
	buf [2][]hitRegion
	cur uint8
}
```

Paths are compared pairwise, entry against entry. That needs no scan of the
frame's regions, unlike `continues`, which looks a handler up; a path is a
handful of entries.

> **Decided** (implementer 1): `sameRegion` falls back to the ID or the
> `Rect` only when the kept handler is painted nowhere this frame, as
> `continuing` does, so two regions sharing a `Rect` stay two; and a path
> is matched by handler in a first pass, before ID or `Rect` in a second,
> so a wrapper and a child rebuilt at the wrapper's `Rect` are told apart.
> This frame's path is found as indices into the list, compared with the
> kept one, and only then copied, so `regionPath` is one buffer, not two.
> The focus path is a `withinPath`, which keeps each entry's within handler
> beside its copy, since a copy does not carry it.

## Pointer dispatch

### The delivery pass

`inputState.send` keeps its shape: regions from the last painted to the
first, those that contain the point, the first whose `HandlePointer` returns
true consumes the event and is returned.

- **Target.** There is no single target. The topmost region under the
  pointer is offered the event first. In DOM terms it is the target, and what
  follows is the bubble.
- **Handled.** True from `HandlePointer` consumes the event and ends the
  pass, as now. There is no separate "stop". (See
  [rejected alternatives](#rejected-alternatives).)
- **Pass-through.** A region that returns false passes the event to what is
  beneath: the regions it is inside, and regions that merely lie under it.
  A subtree painted through `Canvas.Inert` registers nothing. No new option
  is added; these are `pointer-events: none` already.
- **Fall-through to regions that are not ancestors stays.** `Stack` layers,
  a `DropZone` over content, `ui.ContextMenu`'s region over its child,
  `reorderCatcher`, a popup's scrim that lets hover pass: all rely on it,
  and so does `TestUnhandledEventsFallThrough`. A surface that must hide
  what is beneath consumes, as `popupSink` and `modalScrim` do.

### The pass before it

For a `PointerDown` and a `PointerScroll`, when the frame registered any
`HitWithin` region (`regionIndex.within`), each region about to be offered
the event first has the within handlers on its chain offered it, outermost
first, itself included, each handler once per event:

```go
func (in *inputState) send(ev PointerEvent) *hitRegion {
	watch := in.index().within && (ev.Kind == PointerDown || ev.Kind == PointerScroll)
	if watch {
		in.event++
	}
	for i := len(in.regions) - 1; i >= 0; i-- {
		r := &in.regions[i]
		if r.pointer == nil && !(watch && r.within != nil) || !r.contains(ev.Pos) {
			continue
		}
		if watch {
			if w, took := in.offerWithin(r, ev); took {
				return w // nil when w has no pointer handler
			}
		}
		if r.pointer != nil && in.deliver(r, ev) {
			return r
		}
	}
	return nil
}

// offerWithin offers ev to the within handlers of r and of the regions it
// is inside that have not been offered it, outermost first, and reports
// whether one took it.
func (in *inputState) offerWithin(r *hitRegion, ev PointerEvent) (*hitRegion, bool) {
	if r == nil || r.seen == in.event {
		return nil, false
	}
	r.seen = in.event
	if w, took := in.offerWithin(in.parentOf(r), ev); took {
		return w, true
	}
	if h, ok := r.within.(PointerWithinHandler); ok && h.HandlePointerWithin(r.pointerAt(ev)) {
		if r.pointer == nil {
			return nil, true
		}
		return r, true
	}
	return nil, false
}
```

`seen` is set before walking up: a region already offered the event has had
its whole chain offered it. The recursion is as deep as regions nest.

- A within handler that returns false has only looked. This is `pressWatch`.
- One that returns true takes the event: the regions inside it never hear
  it. For a press, its region becomes `pressed` when it also has a pointer
  handler, which then receives `PointerDrag`, `PointerUp` and `PointerTap`;
  with none, the press is dropped. This is `onInterceptTouchEvent`. A region
  that took an event this way is noted for repainting as `deliver` notes one.
- The handler hears the press on its own `Rect` as well, where no region
  inside took it or there is none: "anywhere in the pane".
- It does not hear a press on a region painted over it by something it is
  not around: a popup, a sibling layer. `ObserveInput` and a geometric test
  both would.
- `PointerMove`, `PointerUp`, `PointerTap` and `PointerDrag` are not offered.
  A move would cost a handler call per frame; the others go straight to the
  pressed region.

A modal scrim consumes every press, so nothing behind an open dialog is
offered one, and no handler behind it hears one.

## Hover

### The path

Each frame, `updateHover`:

1. Sends `PointerMove` through `send`, as now. The topmost region that
   claims it is the **hot** region. A claimed move still does not reach
   regions beneath that the hot region is not inside of.
2. Walks up from the hot region and offers the same move, with `Inner` set,
   to each region with a pointer handler on its chain, nearest first. Those
   that return true join the **hover path**. One that returns false is not
   hovered, and the walk goes on past it.

```go
func (in *inputState) updateHover(f frameInput) {
	move := PointerEvent{Kind: PointerMove, Pos: f.pos, Mods: f.mods}
	// ... moved, lastMove as now
	hot := in.send(move)
	path := in.hover.next() // the buffer not holding the last path, emptied
	if hot != nil {
		path = append(path, *hot)
		move.Inner = true
		for a := in.parentOf(hot); a != nil; a = in.parentOf(a) {
			if a.pointer != nil && in.deliver(a, move) {
				path = append(path, *a)
			}
		}
	}
	in.hoverChanged(in.hover.last(), path, f.pos)
	in.hover.keep(path)
}
```

So a region is hovered when it claims `PointerMove`, exactly as now, and
either is the topmost such region under the pointer or has that region
inside it. What changes is one thing: claiming the move no longer hides it
from the regions the claimer is inside.

A region on the path need not contain the pointer. A child painted past its
parent's edge keeps the parent hovered, as `:hover` does.

### Enter and exit

`hoverChanged` compares the last path with the new one, entry against entry
with `sameRegion`:

- Every entry of the last path with no match in the new one hears
  `PointerExit`, **innermost first**, with `Inner` unset. As now, the exit goes to the handler
  holding the region in the current frame (`findPointer`), which is another
  handler when the widget was rebuilt since it was entered, and to the kept
  copy's handler when the region is gone.
- Then every entry of the new path with no match in the last one hears
  `PointerEnter`, **outermost first**, with `Inner` set on all but the hot
  region.
- An entry matched to one with a different handler (`gem.Same` is false) is
  a region rebuilt under the pointer. Its new handler hears `PointerEnter`
  and the old one hears nothing, as `TestARebuiltRegionIsToldItIsHovered`
  pins for the single region today.

All exits precede all enters, as the one exit precedes the one enter today.

For a row holding a button, with the pointer coming from outside onto the
row, then the button, then back, then out:

| Pointer goes | Row hears | Button hears |
| --- | --- | --- |
| onto the row | `PointerEnter` | |
| onto the button | `PointerMove` with `Inner` each frame | `PointerEnter` |
| back onto the row | `PointerMove` each frame | `PointerExit` |
| out | `PointerExit` | |
| from outside straight onto the button | `PointerEnter` with `Inner` | `PointerEnter`, after the row's |

Today the row hears an exit on the second line and an enter on the third,
and nothing at all on the fifth.

### What widgets see

- `Interactive.Pointer` sets `Hovered` on `PointerEnter` and `PointerMove`
  and clears it on `PointerExit`. With no change to it, every control built
  on `Interactive` is hovered while the pointer is over a control inside it:
  a card, an item with an action, a table row, a button that holds a button.
  This is the CSS `:hover` that `ui`'s styles were taken from.
- `widget.Pointer(row).OnHover(fn)` is called with true once when the
  pointer comes over the row or anything inside it, and with false once when
  it has left all of it. This is the reactive hover-within state: `OnHover`
  fits a signal's `Set`, and what is built from the signal is real widgets,
  in the Tab order and in the accessibility tree.
- `PointerEvent.Inner` tells a handler that the pointer is over a region
  inside it. A control that should look hovered only when a click would land
  on it ignores an enter or a move with `Inner` set, and clears its look on a
  move that has it. `PointerWidget.OnMove` passes the event, so its callback
  can do the same.
- The button inside still shows its own hover: it is the hot region and
  hears what it heard before.

### A rebuild under a still pointer

The path is made again every frame from the frame's regions. A hover that
writes a signal which rebuilds what is inside the row is the case that
matters:

1. The pointer comes onto the tab. The wrapper hears `PointerEnter`,
   `hover` is set, the close button is built.
2. The next frame's regions hold the close button. If it is under the
   pointer it is the new hot region and hears `PointerEnter`. The wrapper is
   matched, by handler, to the entry already on the path and hears nothing.
3. The pointer leaves. Button and wrapper hear `PointerExit`, in that order.

No frame in this sequence tells the wrapper it was left, so nothing
flickers. What can still loop is a hover that moves the hovered region out
from under the pointer, as in CSS; and a wrapper that is itself rebuilt by
the signal it sets, has no `HitID` and changes `Rect`, which then matches
nothing. A wrapper outside the part it rebuilds, as mako's are, is matched
by its handler.

### Cost

One `HandlePointer` call per frame for each region with a pointer handler
around the hot region, and a copy of each one that claimed. No scan and no
allocation in a frame whose path did not change.

`deliver` notes the regions a move was claimed by when the pointer moved, so
a hovered container is now painted again as the pointer moves over a control
inside it, where before only the control was. This is what already happens
while the pointer is over the container's own area.

## What goes on inside a region

```go
// HitWithin registers r as a region that is told of what goes on inside
// it: at r, where nothing painted over it by another widget takes the
// event, and in every region painted inside it, wherever that shows. It
// takes no input by itself. h implements any of PointerWithinHandler,
// FocusWithinHandler and KeyWithinHandler. A region is inside another when
// the widget that registered it was painted, through any number of widgets,
// by the one that registered the other, so a container calls HitWithin
// before it paints its children.
func (c *Canvas) HitWithin(r Rect, h any)
```

It goes through `Canvas.region` and `Canvas.add` as the others do, with
`hitRegion{within: h}`: the region takes its `HitID` from `h`, its scope and
group from the `Canvas`, is trimmed by the clip, and merges with a
neighbour of the same `Rect`. `hitRegion.merge` refuses when both sides have
a within handler. `inputView.note` sets `regionIndex.within`, which every
path below tests first: a frame with no such region pays one branch.

A within region with no pointer handler is not hovered, pressed or focused,
and is not a Tab stop. It is a node of the tree.

The three interfaces are described where they act:
[`PointerWithinHandler`](#the-pass-before-it),
[`FocusWithinHandler` and `KeyWithinHandler`](#keyboard-and-focus).

## Keyboard and focus

### Focus within

The **focus path** is the within regions on the chain of the focused
region, itself included, whose handler is a `FocusWithinHandler`. Input
keeps it as it keeps the hover path, and compares it:

- at the end of `inputState.focus`, which every change of focus goes
  through: a press, Tab, `RequestFocus`, a trap opening or closing, the
  focused region disappearing;
- once a frame in `dispatchKeys`, after it has found the focused region in
  the current list, for a rebuild that moved the focused widget under
  another region.

Entries that left hear `HandleFocusWithin(false)`, innermost first; entries
that came hear `HandleFocusWithin(true)`, outermost first; a matched entry
with a new handler hears true. A handler is not told when focus moves
between two regions that are both inside it. Input calls `RepaintAll` when
it tells any, as it does for `KeyFocus` and `KeyBlur`.

`Canvas.FocusWithin(r)` and `Canvas.PaintFocus` stay for a widget that asks
in `Paint`.

### Keys within

A `KeyWithinHandler` is offered key presses while focus is in its region or
inside it. When it is offered one follows the rule shortcuts have, in
`Canvas.Shortcut`'s words: a chord with a modifier before the focused
widget, a bare key only if the focused widget left it.

The order for one `KeyPress`, new steps marked:

1. The focused handler's `ChordClaimer`. A claimed chord skips to step 6.
2. Tab moves focus.
3. **New.** A press with a modifier other than Shift is offered to the
   within handlers on the focused region's chain, nearest first. True
   consumes it.
4. `App.OnKey` hooks.
5. Shortcuts that run before the focused widget: window, `App` and painted.
6. The focused handler: `HandleTick`, then `HandleKey`.
7. For a press the focused handler's `KeyConsumer` does not consume:
   1. **New.** A press without such a modifier is offered to the within
      handlers on the chain, nearest first. True consumes it.
   2. Escape goes to the active focus trap.
   3. Bare-key shortcuts.

Step 3 is in `consumeBindings`, after the Tab test and before
`withoutShortcuts`; step 7.1 is in `afterFocused`, before the Escape test.
Both find the focused region with `findKeyRegion` and walk `parentOf`.

> **Decided**: neither step offers Tab, and step 7.1 does not offer a
> press the focused handler claims either, so a claimed chord reaches no
> within handler at any step.

Nearest first, because the nearest is the most specific: a pane's ⌘W closes
the pane's tab and the window's ⌘W, registered as a window shortcut, never
runs while focus is in a pane that takes it.

Consequences:

- A text field consumes every bare press but Escape
  (`TextInputWidget.ConsumesKey`), so typing never reaches a pane's bare-key
  handler, and Escape does.
- A terminal claims its chords (`ClaimsChord`), so they reach nothing else.
- A frame whose keys the IME took (`TickHandler` returned true) runs no
  step 7, as now.
- With nothing focused there is no chain and nothing is offered. A pane
  that wants its keys while a press landed on nothing focusable keeps a key
  region of its own (`widget.Focus`) under its content, which `findKey`
  already focuses.
- A dialog's content is painted by an overlay, a chain of its own: handlers
  behind an open dialog are not on the focused region's chain.
- A search field that must give Up, Down and Enter to a list keeps using
  `TextInputWidget.OnKey` and `ClaimKeys`. Those are bare keys the field
  consumes; step 7.1 is not for them.

`KeyText` and `KeyRelease` are not offered.

## Presses, capture and drags

**Capture is unchanged.** The region that consumed `PointerDown`, or whose
within handler took it, holds the press until the button comes up.
`PointerDrag`, `PointerUp` and `PointerTap` go to it directly; no pass runs
for them. Hover keeps following the pointer during a press, as now.

**Focus on a press is unchanged.** The topmost key region under the pointer.

**Which source a press may drag.** `pressDrag` runs after the press was sent
and takes the pressed region:

```go
// pressDrag notes the drag source a press may pick up: the topmost drag
// region under at that pressed is inside of, or that is inside pressed, or
// any when nothing took the press.
func (in *inputState) pressDrag(at Point, pressed *hitRegion)
```

- The pressed region is inside the source: a button in a draggable tab.
- The source is inside the pressed region: `Pointer(DragSource(w)).OnTap(fn)`,
  where the press fell through the source's bare area to the wrapper.
- Neither: the press landed on something painted over the source by a
  widget that is not part of it, a popup or a sibling layer, and drags
  nothing. Today it drags the source beneath.

Both tests are walks up a chain.

**Click against drag** stays a threshold. A press on a button inside a
source is the button's; if it moves `dragThreshold` pixels before the
release, the drag starts and the button hears `PointerUp` with no tap. No
gesture arena is added: with one drag session and one threshold there is one
contest, and it is already decided the way Flutter's arena decides tap
against drag.

**A control that drags by itself keeps the gesture.**

```go
// DragCapturer is a PointerHandler that keeps the drags of a press it took,
// instead of letting a drag source around it pick its child up: a slider's
// knob in a row that drags, a selection in a text field in a tab.
type DragCapturer interface {
	CaptureDrag() bool
}
```

`updateAppDrag`, at the threshold, asks the handler now holding the pressed
region; if it captures, the candidate is dropped and the press goes on.

> **Decided** (implementer 5): every control that is a
> `TouchDragCapturer` is a `DragCapturer` too, on the same condition, and
> `widget.TextInput` besides: `ui.Slider`, `ui.RangeSlider` and its thumb,
> `ui.Resizable`, `ui.Carousel`, `ui.InputOTP`, a sheet's drawer handle,
> `widget.Scroll` and its thumb while dragging, a selectable
> `widget.RichText`, `widget.TextInput` while enabled, and `PointerWidget`
> with `OnDrag`.
`TouchDragCapturer` stays what it is, the same question asked for a touch
pan. `PointerWidget` captures when it has `OnDrag`. In `ui` and `widget`,
the slider, the range slider, `TextInput`, the scroll thumb, `Resizable`'s
handle and the carousel should.

**Drop targets are unchanged and stay exclusive.** `DropHover(true)` means
"the drop would land here", and it lands on one target. A zone around the
target is not told; see [rejected alternatives](#rejected-alternatives).

## Wheel and scroll chaining

Chaining stays what it is: a scroll that returns false is offered to what is
beneath, the scroll around it first. `sendScroll` still splits a diagonal
scroll into one event per axis.

Added: a **latch** per axis, for scrolls that are part of a gesture, that
is, whose `ScrollPhase` is not `ScrollPhaseNone`.

- `ScrollPhaseBegan` clears both latches.
- A scroll of a gesture with no latch on its axis goes through `send`; the
  region that took it becomes the latch, kept as `touchScroll` is.
- A scroll of a gesture with a latch goes to the latched region alone
  (`findPointer`), after the within handlers on its chain. What it returns
  is not acted on: a view at its end does not hand the rest of the gesture,
  or its momentum, to the view around it.
- `ScrollPhaseMomentumEnded`, any press, and the latched region
  disappearing clear the latch. A latched region in a focus scope other than
  the active one is dropped, as `stepTouchMomentum` drops its target.

A wheel's notches, and every scroll on a platform that reports no phases,
are not latched and chain as now.

> **Decided**: a frame's phases are merged, so `laterPhase` now keeps
> `ScrollPhaseBegan` over `ScrollPhaseChanged` in the same frame, the
> latch is cleared on a frame whose phase is `ScrollPhaseBegan` even when
> it moves nothing, and a `ScrollPhaseChanged` after the fingers lifted
> begins a new gesture too, for a `Began` lost in a lift's frame. Only a
> scroll in pixels with a phase, a trackpad's, is latched; the notches of
> `frameInput.wheel` are not, whatever their phase. A lift, which moves
> nothing, goes to the views latched on either axis.

With the latch in place `ScrollWidget.wheelBy` should report honestly
whether a finger-driven scroll moved anything. Today it reports true for
every scroll that is not momentum, which keeps a gesture from changing views
midway but also keeps a new gesture over an inner view at its end from
moving the outer one, while momentum does leak outward. With both changes a
gesture goes to the innermost view that can move when it begins and stays
there, which is what a browser does. A view that should never chain
(`overscroll-behavior: contain`) is a `Scroll` option that returns true; it
needs nothing from routing.

> **Decided**: `wheelBy` reports honestly only for a latched gesture, a
> scroll in pixels with a phase. A touch pan, which has no phase, and a
> wheel's notches still report every scroll that is not momentum as used:
> a pan begun over a view at its end must still be taken from the press
> beneath, or the press would end in a tap. The kernel's `scroller`
> fixture follows the same rule. `WindowProbe.TrackpadGesture` drives a
> gesture in a test.

`PointerWithinHandler` is offered scrolls before the view that takes them,
which is the wheel half of `ObserveInput`.

## Context menus, cursors and observers

**Context menus.** `ui.ContextMenu` keeps working unchanged: its pointer
region, registered after its child, is a leaf beside the child's regions
and above them, and still hears a secondary press first. It is not rewritten
as a within handler, because those run outermost first and a context menu
wants the innermost. The principled form is a region *under* the child that
hears the secondary presses nothing inside consumed, with no list of inner
menus; that needs `Interactive.Pointer` to stop consuming presses of buttons
it does not act on, which is a change of its own (see
[open questions](#risks-and-open-questions)).

**Cursors.** Unchanged: the topmost region under the pointer that asks
decides, ancestor or not. That a popup's arrow hides a text field's beam
beneath it is exactly the fall-through this rule wants.

**Observers.** `Canvas.ObserveInput` is unchanged. It is geometric: it hears
input at a point in its `Rect` whatever is painted there, and keys while
the focused `Rect` overlaps. A within handler hears the same things by the
tree instead, and can tell a press from a wheel. `pressWatch` becomes
`widget.Within(pane).OnDown(fn)`.

## Accessibility

- The semantics tree stays explicit and separate. The region tree is not
  published and is not a second source for it: layout widgets are absent
  from it, a disabled control registers no region, and text never does.
- Hover has no meaning to assistive technology. What hover reveals must be
  reachable without it. The pattern this design is meant to allow is: build
  the control always, show it while the pointer *or the focus* is inside the
  row, and paint it invisible otherwise:

  ```go
  hover, focus := ggui.State(false), ggui.State(false)
  shown := ggui.Combine(hover, focus, func(h, f bool) float64 {
  	if h || f {
  		return 1
  	}
  	return 0
  })
  tab := widget.Within(widget.Row(
  	label,
  	widget.Opacity(closeButton, 0).BindOpacity(shown),
  )).OnHover(hover.Set).OnFocus(focus.Set)
  ```

  `widget.Opacity` at zero still registers regions and semantics, so the
  button is in the Tab order and the tree at all times; tabbing into the row
  shows it.
- `Window.Perform` carries out `ActionPress` by calling the handler
  directly (`press` in semantics.go). No pointer event is routed, so no
  within handler hears a press. `ActionFocus` goes through
  `inputState.focus`, so `FocusWithinHandler` is told. A pane that tracks
  "current" should follow focus as well as presses.
- `Probe.Find` reads roles from the regions. A within region has a role
  only if its handler is `Semantic`; `widget.Within` is not.

## API

Root package:

```go
type PointerEvent struct {
	// ...

	// Inner is set on a PointerMove or PointerEnter that reaches a region
	// because the pointer is over a region painted inside it, which claimed
	// the move, rather than over the region itself.
	Inner bool
}
```

```go
const (
	PointerMove  // the cursor is over the region this frame, or over a region painted inside it
	PointerEnter // the cursor came over the region or over a region painted inside it
	PointerExit  // the cursor left the region and everything painted inside it
)
```

To `PointerHandler`'s documentation:

```go
// A PointerMove is claimed rather than consumed. The topmost region that
// returns true for it is the one the pointer is over; regions beneath it
// that it is not painted inside hear nothing, and each region it is painted
// inside is offered the move in turn, with Inner set, and is hovered with it
// if it returns true as well. A hovered region hears PointerEnter once as
// that begins and PointerExit once as it ends, however the pointer moves
// among the regions inside it.
```

```go
// HitWithin: see "What goes on inside a region".
func (c *Canvas) HitWithin(r Rect, h any)

// PointerWithinHandler is the handler of a HitWithin region that is offered
// each press and each scroll of the wheel headed for the region or for one
// painted inside it, before any of those hears of it, and after the within
// handlers of the regions it is inside itself. Returning false lets the
// event go on. Returning true takes it: nothing inside hears it, and a
// press taken belongs to the region, whose PointerHandler, if it has one,
// receives the PointerDrag, PointerUp and PointerTap that follow.
type PointerWithinHandler interface {
	HandlePointerWithin(ev PointerEvent) bool
}

// FocusWithinHandler is the handler of a HitWithin region that is told when
// keyboard focus comes to a region painted inside its own, or to its own,
// and when it has left them all. It is not told as focus moves from one
// region inside to another.
type FocusWithinHandler interface {
	HandleFocusWithin(within bool)
}

// KeyWithinHandler is the handler of a HitWithin region that is offered key
// presses while focus is in the region or inside it, where a shortcut would
// run: a press with a modifier other than Shift before the focused widget
// and the window's shortcuts, unless the widget claims the chord; a bare key
// after the focused widget, when it did not consume it, before a focus
// trap's Escape and the bare-key shortcuts. The handler nearest the focus is
// offered a press first, and returning true takes it from the rest.
type KeyWithinHandler interface {
	HandleKeyWithin(ev KeyEvent) bool
}

// DragCapturer: see "Presses, capture and drags".
type DragCapturer interface {
	CaptureDrag() bool
}
```

Package `widget`:

```go
// WithinWidget tells what goes on inside its child without taking any of
// it: the pointer over it, focus in it, a button going down in it, the keys
// of what has focus in it. Build one with Within. It takes no space of its
// own: the child's Rect is its region, and what the child paints is inside
// it wherever that shows.
type WithinWidget struct{ /* ... */ }

// Within wraps child in a region that hears what goes on inside it. Attach
// callbacks with the On methods.
func Within(child ggui.Widget) *WithinWidget

// OnHover fires with true when the pointer comes over the child or anything
// painted inside it, and with false when it has left all of it; hover.Set
// fits it. A Within with OnHover claims the pointer's hover over the child,
// as Pointer's OnHover does: a widget beneath that is not around it is not
// hovered through it.
func (w *WithinWidget) OnHover(fn func(bool)) *WithinWidget

// OnFocus fires with true when keyboard focus comes inside the child and
// with false when it has left; focused.Set fits it.
func (w *WithinWidget) OnFocus(fn func(bool)) *WithinWidget

// OnDown fires when a button goes down over the child or anything painted
// inside it, before whatever it landed on hears of it, and whether or not
// that takes it.
func (w *WithinWidget) OnDown(fn func(ggui.PointerEvent)) *WithinWidget

// OnKey is offered the key presses of the widget that has focus inside the
// child, where a shortcut would run: a chord with a modifier before that
// widget, a bare key only if the widget left it. Return true to take the
// press.
func (w *WithinWidget) OnKey(fn func(ggui.KeyEvent) bool) *WithinWidget
```

`WithinWidget.Paint` calls `dst.HitWithin(r, w)`, then, when `OnHover` is
set, `dst.HitPointer(r, w)`, which merges into the same region, then
`dst.Paint(w.child, r)`. `HitWithin` with a nil handler registers nothing. Its `HandlePointer` returns true for
`PointerMove`, calls the hover callback on `PointerEnter` and
`PointerExit`, and returns false for everything else. Its
`HandlePointerWithin` calls `OnDown` for a `PointerDown` and returns false.
The callbacks are structural setup in the sense of
[the API conventions](../api-conventions.md): set before mount.

`PointerWidget` gains nothing but meaning. `OnHover`, `OnEnter` and `OnExit`
follow the path; `OnMove` also fires, with `Inner` set on its event, while
the pointer is over something inside the child that claims it;
`CaptureDrag` reports whether `OnDrag` is set.

`Interactive` does not change.

## Compatibility and migration

Nothing is opt-in. The tree is built for every region, and the hover rule
applies to every handler. What a handler must do to stay as it was is
nothing, in every case but the first row below.

### What changes

| Behaviour | Before | After | Phase |
| --- | --- | --- | --- |
| A region that claims `PointerMove`, with the pointer over a region inside it that also claims | Hears `PointerExit`; hears `PointerEnter` on the way back; hears no move meanwhile | Stays hovered; hears `PointerMove` with `Inner` | 2 |
| The same, with the pointer arriving straight onto the inner region | Hears nothing | Hears `PointerEnter` with `Inner` | 2 |
| `Interactive.Hovered` of a control that holds controls | False while an inner control is hovered | True | 2 |
| What a mouse move repaints | The hot region | The hover path | 2 |
| A press on a region painted over a `DragSource` by a widget that is not part of it | Can drag the source | Drags nothing | 5 |
| A `DragCapturer` pressed inside a `DragSource` | Loses its drag at 4 pixels | Keeps it | 5 |
| A trackpad gesture whose view reaches its end | Momentum moves the view around | Stays with its view | 6 |
| A trackpad gesture begun over a view already at its end | Does nothing | Moves the view around | 6 |

### What stays

- The order events are offered in, and that true consumes.
- Fall-through of a declined event to regions beneath that are not
  ancestors. `TestUnhandledEventsFallThrough`, `TestTopmostRegionWins`.
- Which region is hot. A handler that returns false for `PointerMove` is
  never hovered and hears no enter or exit, as now.
- That a claimed move hides hover from regions beneath that the hot region
  is not inside: a popup still masks the buttons under it.
- Press capture, `PointerTap` matching, focus on press, Tab order, focus
  traps, `ChordClaimer`, `KeyConsumer`, shortcuts.
- `continuing` and `Adopter`: how one region is followed across a rebuild.
- Cursors, `DropTarget`, `DropHandler`, `DragHandler`, `ObserveInput`,
  `Probe`.
- Regions built by hand in tests: a zero `parent` is a root.

### Widgets and tests likely affected

Phase 2, hover. Run the whole suite; look at these first.

- `ui/fieldcard.go`: the `hovered()` question to the control is redundant.
  `ui/fieldcard_test.go` should pass before and after removing it.
- `ui/sidebar_list.go` `showsAction`: the `Canvas.Pointer()` test is
  redundant once the row's handler hears moves over its action
  (`TestSidebarActionOnHover`).
- `ui/item.go`, `ui/card.go`, `ui/bubble.go`, `ui/attachment.go`,
  `ui/table.go`, `ui/tree.go`, `ui/collapsible.go`, `ui/accordion.go`,
  `ui/numberinput.go`, `ui/inputgroup.go`, `ui/select.go`,
  `ui/combobox.go`: controls that can hold controls. Where they are built
  on `Interactive`, their hover fill now stays under an inner control. `ui/surfaces_parity_test.go`
  (`TestItemWithAnActionIsALinkAndOneWithoutTakesNoInput`),
  `ui/display_parity_test.go`, `ui/bubble_hover_test.go` and the painted
  baselines are the ones that could notice.
- `ui/menu.go` `MenuItemWidget.HandlePointer`, `ui/sidebar_parts.go` (the
  rail), `ui/navigationmenu.go`, `ui/chart_content.go`: handlers that act on
  `PointerMove` or `PointerExit` themselves. None holds a claiming region
  today as far as the code shows; check.
- `widget/scroll.go`: the thumb's region is inside the scroll's. The scroll
  is offered the move over the thumb, returns true for it there, and hears an
  enter it ignores. `widget/scroll_test.go` (`thumbHovered`).
- `widget/popup.go` `popupSink`, `ui/modal.go` `modalSink`: now hear
  `PointerEnter` with `Inner` for what is inside them, and ignore it.
- `widget/interact_test.go` `TestPointerCallsItsCallbacks`,
  `internal/apitest/pointer_test.go`, `input_test.go`
  (`TestHoverEnterAndExit`, `TestHoverExitReachesARebuiltControl`,
  `TestARebuiltRegionIsToldItIsHovered`), `damage_test.go`
  (`TestDeliveredPointerEventsNoteTheirRegions`,
  `TestAMoveIsNotedOnlyWhenThePointerMovedAndItWasClaimed`): no nesting, so
  no change expected; they pin what must stay.

Phase 5, drags: `internal/apitest/dragdrop_test.go`, `widget/dragdrop.go`
(`reorderRow`, `ReorderHandle`), and the controls that should capture.

Phase 6, scroll: `widget/scroll.go` `wheelBy`, `widget/scroll_test.go`,
`ui/codeblock_scroll_test.go`, `ui/messagescroller_test.go`,
`window_test.go` `TestScrollPhaseReachesTheFrame`, the touch-pan tests in
`input_test.go` (touch is not latched by this; it has `touchScroll`).

### mako

| Today | After |
| --- | --- |
| `roundTab` reading `Canvas.Pointer()`, `tabHover`, `hoverShown` | `widget.Within(tab).OnHover(hover.Set).OnFocus(focus.Set)` and the close button at `Opacity` 0 or 1 |
| `widget.Pointer(row).OnHover(hover.Set)` in `sidebartree.go`, `sidebaragents.go`, `diffview.go` | Unchanged source; now correct over the row's buttons |
| `pressWatch` | `widget.Within(pane).OnDown(...)`, with `.OnFocus` for the keyboard |
| "Current pane" routing of window shortcuts | `widget.Within(pane).OnKey(...)` where a shortcut is the pane's own |
| `OnHover(func(bool) { m.leaveHint(id) })` on every nested drop zone | Unchanged; drop hover stays exclusive |

> **Decided**: confirmed against what was built, with these details. The
> tab's `Within` goes around `roundTab`, so the tab's button, its context
> menu's region and the close button are all inside it; `roundTab` paints
> its tint from the `hover` signal instead of `Canvas.Pointer()`, and
> `hoverShown` becomes the close button under `widget.Opacity(...)
> .BindOpacity(shown)`, with the dirty dot drawn at the opposite opacity.
> The tab's own ghost `ButtonOf` now stays hovered while the pointer is on
> its close button, its hover fill under the close button's. `pressWatch`
> becomes `widget.Within(column).OnDown(...)`, with `OnFocus(func(in bool)
> { if in { m.focusGroup(id) } })` for focus moved by the keyboard; mako's
> `layoutview_test.go` tests `pressWatch` and goes with it. A pane's
> `OnKey` hears nothing while nothing in the pane has focus, nor a chord a
> focused terminal claims.

## Implementation plan

Each phase leaves the suite green and can ship alone. Phases 2, 3 and 5 need
phase 1; phase 4 needs phase 3; phase 6 needs nothing, and offers a latched
scroll to within handlers only once phase 3 is in.

New tests are written as the existing ones are: in package `ggui` on an
`inputState` with `paintFrame` and the fixtures of `fixtures_test.go`
(`onPointer`, `tapped`, `padded`, `column`, `pile`, `hoverBox`), or on a
`Probe` with `Move`, `Press`, `Click`, `Key` and `Find`.

### Phase 1: the tree

`hitRegion.parent`; `regionCursor` in `frameState`; `openScope`,
`closeScope`, `Canvas.add`, `parentFor`, `paintOverlays`, `beginPaint`;
`inputState.parentOf`; `keep` and `regionSlot.keep` zeroing the link. No
behaviour changes.

Tests (canvas_test.go, input_test.go):

- `TestARegionIsInsideTheRegionOfTheWidgetAroundIt`:
  `onPointer(padded(onPointer(sized(20, 20), h), 10), h)`; the second
  region's parent is the first, the first's is none.
- `TestWidgetsThatRegisterNothingDoNotBreakTheChain`: the same with
  `column` and `padded` between.
- `TestRegionsOfSiblingsAreBesideEachOther`: two `onPointer` in a `pile`
  at overlapping `Rect`s; both roots.
- `TestARegionRegisteredAfterTheChildIsBesideIt`: a widget that paints its
  child and then calls `HitPointer`, as `pressWatch` does; the child's
  region is not inside it.
- `TestAWidgetsOwnRegionsNestByWhatCoversWhat`: one `Paint` registering a
  track, a knob within it and a region beside it; and three disjoint cells
  with a child widget painted after each.
- `TestMergedRegistrationsAreOneNode`: `onPointer` around `focusable`
  around `onPointer`, same `Rect`; two regions, the inner inside the merged
  one.
- `TestOverlayRegionsStartAChainOfTheirOwn` (Probe): a widget that
  registers a region and schedules an overlay that registers one.
- `TestARegionClippedAwayIsNoParent`.
- `TestARegionIsRegisteredAfterTheRegionItIsInside` (Probe, a tree of `ui`
  controls with a popup open): every parent index is below its region's.
- `TestRegisteringRegionsAllocatesNothingMore`: `testing.AllocsPerRun` over
  a second paint of a fixed tree, zero.

> **Decided** (implementer 4): the routing is in `routing.go`, its tests in
> `routing_test.go`. `TestARegionIsRegisteredAfterTheRegionItIsInside` is
> written on the kernel's fixtures, an overlay scheduled from an overlay,
> a scroller, merges, clips and regions after a child among them: the
> kernel's tests cannot import `ui`, and nothing outside the kernel can see
> a region's parent.

### Phase 2: hover

`PointerEvent.Inner`; `regionPath`; `sameRegion`; `updateHover`,
`hoverChanged`; `inputState.hover` in place of `hovered` and `hoverSlot`.
Documentation of `PointerMove`, `PointerEnter`, `PointerExit`,
`PointerHandler`, `PointerWidget.OnHover` and `OnMove`.

Tests:

- `TestHoverReachesTheRegionsAroundTheHotOne` (input_test.go): two nested
  `onPointer` that claim everything, each logging enter and exit. Outside
  to inner: outer enter, inner enter. To the outer's padding: inner exit.
  Back: inner enter. Out: inner exit, outer exit.
- `TestARegionThatPassesTheMoveIsNotHovered`: the outer returns false for
  `PointerMove`; it hears no enter and no exit, and a claiming region around
  *it* still does.
- `TestAClaimedMoveStillHidesWhatIsBeneathAndNotAround`: a `pile` of two
  overlapping claimers; the lower hears nothing.
- `TestAMoveSaysItIsAnInnerRegions`: `Inner` on the outer's move and enter,
  not on the inner's.
- `TestAChildPastItsParentsRectKeepsTheParentHovered`.
- `TestTheHoverPathSurvivesARebuild` (Probe, `Reactive`, two nested
  `hoverBox` with `Adopt`): after a rebuild under a still pointer both are
  hovered, the new handlers each heard one enter, the old ones no exit; after
  the pointer leaves, neither is hovered. This is
  `TestHoverExitReachesARebuiltControl` and
  `TestARebuiltRegionIsToldItIsHovered` at two levels.
- `TestAHoveredRegionAroundIsPaintedAgain` (damage_test.go): `takeTouched`
  holds the outer's `Rect` after the enter, and does not hold that of an
  ancestor that passed the move.
- `TestHoverAllocatesNothing`: `AllocsPerRun` of a dispatch over nested
  regions with the pointer at rest, zero.
- **The tab case**, `TestARowHearsHoverOverAButtonInsideIt` (package
  `ui_test` or `internal/apitest`):

  ```go
  var hover []bool
  closeBtn := ui.ButtonOf(ui.Icon(icons.Plus), func() {}).Ghost().Name("Close")
  row := widget.Pointer(widget.Row(widget.Box().Size(100, 30), closeBtn)).
  	OnHover(func(on bool) { hover = append(hover, on) })
  p := ggui.NewProbe(widget.Column(row, widget.Box().Size(100, 30)), ggui.Sz(200, 100))
  defer p.Close()
  x, _ := p.FindRole(ggui.RoleButton, "Close")
  p.Move(ggui.Pt(10, 10))  // the row's own area
  p.Move(x.Center())       // the button
  // hover is [true]; closeBtn.Hovered
  p.Move(ggui.Pt(10, 10))
  // hover is [true]; !closeBtn.Hovered
  p.Move(ggui.Pt(10, 80))
  // hover is [true false]
  ```

  And once more with the pointer going from outside straight onto the
  button: `[true]`, then `[true false]`.
- `TestAButtonShownOnHoverStaysWhileThePointerIsOverIt`: the reactive form
  mako wanted. `hover := ggui.State(false)`; a ghost `ButtonOf` whose child
  row ends in `ggui.View(hover, ...)` building a "Close" button or an empty
  box of its size; `widget.Pointer(tab).OnHover(hover.Set)` around it. Move
  onto the tab: "Close" is found. Move onto its centre and run three frames:
  still found, and `Hovered`. Click it: the close callback ran once and the
  tab's did not. Move away: not found.
- `TestAFieldCardIsHoveredOverItsControl` after removing the workaround.

> **Decided** (implementer 3): `updateHover` finds the path every frame;
> the walk up the hot region's chain and the pairwise compare are cheap
> and allocate nothing, which `TestHoverAllocatesNothing` holds. The tab
> case and its reactive form are in `internal/apitest/hover_test.go`.

### Phase 3: within, for presses and focus

`hitRegion.within`, `hitRegion.seen`, `inputState.event`,
`regionIndex.within`; `Canvas.HitWithin`; `merge`; `PointerWithinHandler`,
`FocusWithinHandler`; `send`, `offerWithin`; the focus path in `focus` and
`dispatchKeys`; `widget.Within` with `OnHover`, `OnFocus`, `OnDown`.

Tests:

- `TestWithinHearsAPressBeforeWhatTakesIt`: order of a shared log.
- `TestWithinHearsAPressNothingInsideTakes`: on its own area.
- `TestWithinIsNotToldOfAPressBesideItOrOverIt`: a sibling; and an overlay
  painted over it that consumes.
- `TestWithinHandlersHearOutermostFirstAndOnce`: nested; and a press that
  falls through two regions inside calls each handler once.
- `TestAWithinHandlerThatTakesAPressKeepsIt`: the region inside hears
  nothing; the handler's `HandlePointer` hears the drag, the up and the tap.
- `TestWithinHearsTheWheelAndNotTheMove`.
- `TestFocusWithinFollowsTheChain`: Tab into a child, true; Tab to another
  child inside, no call; Tab out, false; nested, outermost told first coming
  in and last going out; the focused region removed, false; a rebuild of the
  handler, the new one told true.
- `TestPerformFocusTellsFocusWithin` (Probe, `Perform` with `ActionFocus`).
- **The pressWatch case**, `TestWithinHearsEveryPressInAPane`
  (`widget/interact_test.go`):

  ```go
  downs, taps := 0, 0
  pane := widget.Within(widget.Column(
  	ui.Button("Run", func() { taps++ }),
  	widget.Box().Size(100, 50),
  )).OnDown(func(ggui.PointerEvent) { downs++ })
  p := ggui.NewProbe(widget.Row(pane, ui.Button("Other", func() {})), ggui.Sz(300, 120))
  defer p.Close()
  p.Tap("Run")                  // downs 1, taps 1: heard, not taken
  p.Click(ggui.Pt(20, 80))      // downs 2: the pane's bare area
  p.Tap("Other")                // downs 2
  p.Scroll(ggui.Pt(20, 20), ggui.Pt(0, -3)) // downs 2: a wheel is no press
  ```

  And with a `Popup` opened from "Run" and a press on its content: not
  heard.
- `TestWithinAllocatesNothing`.

> **Decided**: the pressWatch case is in `internal/apitest/within_test.go`,
> with the popup case as `TestWithinDoesNotHearAPressOnAPopupItOpened` and
> the hover-or-focus pattern of [Accessibility](#accessibility) as
> `TestWithinShowsAButtonOnHoverOrFocus`: `widget`'s tests are in package
> `widget` and cannot import `ui`. `widget/within_test.go` tests `Within`
> on `widget`'s own controls. `TestWithinAllocatesNothing` compares a
> frame with within handlers against the same frame without them: a press
> already allocates the copy of the region that holds it.

### Phase 4: keys within

`KeyWithinHandler`; `consumeBindings`, `afterFocused`;
`WithinWidget.OnKey`.

Tests (input_test.go, beside `TestShortcutsRunAroundTheFocusedWidget` and
`TestClaimedChordsSkipRuntimeBindings`):

- `TestBareKeysWithinFollowTheFocusedWidget`: a focused region that
  consumes one key and not another; the handler hears only the second;
  nearest first; true keeps it from the outer handler, from a trap's Escape
  and from a bare-key shortcut.
- `TestChordsWithinPrecedeTheFocusedWidgetAndTheWindow`: a chord with a
  modifier taken within runs neither the window's shortcut nor the focused
  widget; one not taken runs both as before; a claimed chord is not offered.
- `TestTypingInAFieldDoesNotReachKeysWithin` (Probe, `widget.TextInput`):
  letters are not offered, Escape and a ⌘ chord are.
- `TestTabIsNotOfferedWithin`.
- `TestKeysWithinNeedFocusInside`: focus beside the region, and no focus.

### Phase 5: drags

`pressDrag(at, pressed)` after `send` in `dispatchPointer`; `DragCapturer`
in `updateAppDrag`; `PointerWidget.CaptureDrag`; the controls listed above.

Tests (`internal/apitest/dragdrop_test.go`):

- `TestAButtonInASourceClicksOrDrags`: a click taps; a press that moves
  drags and does not tap. Pins today's behaviour.
- `TestAPressThatFallsThroughASourceStillDragsIt`:
  `widget.Pointer(widget.DragSource(box, 1)).OnTap(fn)`.
- `TestAPressOnAPopupOverASourceDragsNothing`.
- `TestADragCapturerInASourceKeepsItsDrag`: a slider in a `DragSource`
  moved 40 pixels changes its value; the source never starts.
- `TestAReorderableRowWithASliderStillReorders`: by its other parts.

### Phase 6: scroll latch

Per-axis latch in `inputState`; `sendScroll`; `ScrollWidget.wheelBy`.

Tests (input_test.go with `frameInput{wheelPx, phase}`; widget/scroll_test.go):

- `TestAGestureStaysWithTheViewThatTookIt`: nested scrollers; the inner
  reaches its end mid-gesture and the outer does not move, nor on momentum.
- `TestANewGestureGoesToTheViewThatCanMove`: `ScrollPhaseBegan` with the
  inner at its end moves the outer.
- `TestAWheelsNotchesAreNotLatched`.
- `TestALatchedViewThatGoesLetsTheGestureOn`.
- `TestADiagonalGestureLatchesEachAxisToItsView`.

### Phase 7: what the tree makes unnecessary

Remove the workarounds in `ui` listed under
[widgets and tests](#widgets-and-tests-likely-affected); update
[Input and focus](../input.md) (the Input, Pointer and keyboard events,
Shortcuts, Drag and drop and Custom input handlers sections) and
[Testing](../testing.md); show a region's chain in the inspector. Each
removal is its own change with the widget's tests.

> **Decided** (implementer 6): the inspector's Computed pane has an
> "Input" section for a widget that registered a region: what the region
> takes (pointer, keys, within, drag, cursor) and the regions it is
> inside, nearest first, by the type of their handlers
> (`inspectSource.region`). Of the `ui` workarounds, `FieldCardWidget`'s
> question to its control (and `toggle.hovered`, which only it asked) and
> `SidebarWidget.showsAction`'s `Canvas.Pointer()` test are gone. The other
> readers of `Canvas.Pointer()` named under [Problem](#problem) stay:
> `Tooltip` and `HoverCard` have no region of their own and wait on
> `Canvas.PaintHover` (owner question 7); `CodeBlock` and `Carousel` were
> not part of the list phase 7 names. `hoverPick`, the hover of tabs,
> toggle groups and sidebar rows, ignores a move with `Inner` when another
> item has the hover, so that an item never takes the hover from an item
> inside it.
>
> Two consequences the proposal did not foresee, met in `ui`. A widget that
> registers a header's region and then paints the body it discloses has
> the body inside the header, by the first rule: `Collapsible`, an
> `Accordion`'s headers and a `Trace` step's line would stay hovered over a
> control in the body. Each now passes a move with `Inner` whose position
> is outside its own header (`beyond` in `ui/ui.go`), so it is hovered over
> itself and the controls in it alone. And a `Table` row that does not
> select took the move without claiming it, so it never heard an exit and
> kept its hover fill once the pointer had crossed it; it now claims the
> hover, takes no press, and hears the pointer leave.

## Rejected alternatives

**Deliver to the target and its ancestors only, as the DOM does.** The
clean model, and what the earlier proposal sketched as capture, target and
bubble. It ends fall-through to regions that are not ancestors, which
`Stack` layers, drop zones, the context menu's region, scrims that pass
hover and the wheel through a floating control all use, and it would need a
pass-through flag on each of them. The tree's ancestors already hear an
event after their descendants in today's order; the only thing the DOM model
adds is taking siblings away.

**Ancestors first, then what lies beneath.** Offer a declined event up the
chain, then fall through to unrelated regions. It differs from today only
when an unrelated region lies between a region and its ancestor in paint
order, changes who wins there, and needs marks on every region per event.

**`Target`, `CurrentTarget`, `Phase` and `Stop()` on `PointerEvent`.**
`Stop` apart from "handled" exists in the DOM because there a handled event
goes on to ancestors; here it does not, so true is both. `PointerEvent` is a
value: a `Stop` method needs shared state behind it, per event. A target
would be another widget's handler in the hands of an ancestor. The two
things a phase is used for have their own entry points instead:
`HandlePointerWithin` before, `HandlePointer` after.

**Two kinds of hover: exclusive enter and exit as now, and a new
notification for "inside".** Leaves every existing handler alone, at the
cost of two vocabularies and of keeping the behaviour no retained toolkit
has. Every `Interactive` container would need its own within registration
to get what `:hover` gives. `Inner` gives the exclusive answer to the
handler that wants it.

**A hover path of every ancestor, claiming or not.** Simpler to state, but
it would send enters to scrolls, scrims and tap-only wrappers that never
asked for hover, and repaint them.

**Parents by geometry.** A region inside whatever region covers it. Wrong
for a wrapper and its child at the same `Rect`, for a child that overflows,
and for a popup over a list. Geometry is used only among the regions of one
widget, where there is nothing else to go by.

**Explicit nesting, `dst.HitNode(r, h, fn)`, as `Canvas.Node` nests
semantics.** Every container widget would have to change before the tree
was right. Semantics needs the explicit form because most of what it
describes registers nothing; regions do not.

**A tree kept across frames.** Nothing needs one. Regions are rebuilt every
paint; paths are matched as single regions already are.

**A gesture arena.** One contest exists, click against drag, and the
threshold settles it. `DragCapturer` and `TouchDragCapturer` are the two
places a control must be able to say "mine".

**Drop hover along the path.** A zone around the target would stay lit.
But the highlight says where the drop lands, and it lands in one place.
"A drag is somewhere over this bar" is a different question; mako's
`leaveHint` answers it by hand. If it recurs, it is a `DropTarget` method of
its own, not a change to `DropHover`.

**`Canvas.HasHover(h)`.** A handler on the path already heard
`PointerEnter`; `Interactive.Hovered` is that answer.

## Risks and open questions

Risks:

- **A widget painted without `Canvas.Paint`.** A child painted with
  `child.Paint(dst, r)` opens no scope, so its regions are registered as its
  parent's own and nest by what covers what. About a dozen call sites do
  this, nearly all plain delegation. The covering rule makes them come out
  right where the parent registered a region around them.
- **Merges across widgets.** Two sibling widgets at one `Rect`, one
  registering a pointer handler and the next a key handler, are one region
  today and one node here; what the second paints is inside the first's
  handler too.
- **Handlers that assume a move is inside their `Rect`.** A move with
  `Inner` can lie outside it, for an overflowing child.
- **Applications that relied on the exit.** A container that hid something
  when the pointer went onto its child will now keep showing it. This is the
  change asked for, and it is silent.
- **Repainted area.** Hovered containers repaint with their children as
  the pointer moves. A pane-sized `OnHover` repaints the pane on every move
  inside it.
- **Stale links.** A kept copy's `parent` must never be followed. Zeroing
  it in every copy makes a mistake a nil chain, not a wrong one.
- **Assistive presses** reach no within handler; see
  [Accessibility](#accessibility).

For the owner:

1. **`Interactive.Hovered` follows the path** (recommended), or only the
   hot region. The second keeps every `ui` control looking as it does and
   needs `Interactive.Pointer` to ignore `Inner`; containers then opt in one
   by one.
   **Decided** by the owner: it follows the path.
2. **Overlays as roots** (recommended for now), or inside the region of the
   widget that scheduled them, as `Overlay`'s `SemRef` owner attaches them
   in the semantics tree. Inside: a press in a pane's dropdown reaches the
   pane's `OnDown`, a row stays hovered while its menu is open and pointed
   at, a submenu keeps its item hovered. Also inside: a button looks hovered
   while the pointer is in the dialog it opened. The data supports either;
   `Canvas.Overlay` would record the cursor and `paintOverlays` restore it.
   **Decided** by the owner: overlays are roots.
3. **The scroll latch and `wheelBy`'s honesty** change how nested scrolling
   feels on a trackpad. Recommended, on the browsers' precedent; phase 6 can
   be dropped without touching the rest.
   **Decided** by the owner: included.
4. **`Interactive.Pointer` consuming every button.** If it returned false
   for a press, release or tap of a button other than the left, a secondary
   press would reach what is beneath a control, and `ui.ContextMenu` could
   register under its child and drop its list of inner menus. A third-party
   handler that returns true for everything would then block the menu, which
   the current arrangement survives.
   **Decided**, not covered by the owner's answers: left as it is, with
   `ui.ContextMenu` unchanged, as the section on context menus recommends.
5. **`ObserveInput`**: keep as the geometric observer (recommended), or
   re-express `MessageScroller` and `Carousel` on within handlers and
   deprecate it.
   **Decided**, as recommended: kept.
6. **Focus on a press** could go to the nearest key region on the pressed
   region's chain rather than the topmost under the pointer, so that a press
   on a panel floating over a field does not focus the field. Not proposed
   here; nothing reported it.
   **Decided**, as recommended: not done.
7. **A `Canvas.PaintHover(w, r) bool`**, the pointer's `PaintFocus`, for
   wrappers with no region of their own such as `Tooltip`, which today opens
   over a `Rect` a popup covers. Not proposed here.
   **Decided**, as recommended: not done.
8. **Names**: `HitWithin`, `widget.Within`, `Inner`, `DragCapturer`, and
   the three `...WithinHandler` interfaces.
   **Decided** by the owner: as written.

For the implementer:

1. `int32` or `int` for `parent`; the cursor's exact home; whether
   `regionPath` is one type for hover and focus.
   **Decided**: `int32`, beside `shaped` in what was padding; the cursor is
   `paintedInput.cur`, saved in `paintScope.cur`; two types, `regionPath`
   for hover and `withinPath` for focus. See [Data](#data) and
   [Across rebuilds](#across-rebuilds).
2. `seen` marks against a small fixed array of handlers already offered.
   **Decided**: neither; a reused slice of region indices,
   `inputState.offered`. See [Data](#data).
3. Whether `updateHover` skips rebuilding the path when the region list and
   the hot index are those of the last frame.
   **Decided**: it does not skip; it allocates nothing either way.
4. Whether the new routing lives in input.go or a file of its own.
   **Decided**: `routing.go`.
5. Which `ui` controls implement `DragCapturer` in phase 5 and which wait
   for a report.
   **Decided**: those that are `TouchDragCapturer`s, and `TextInput`; see
   [Presses, capture and drags](#presses-capture-and-drags).
6. The form of the inspector's chain display.
   **Decided**: an "Input" section of the Computed pane; see
   [Phase 7](#phase-7-what-the-tree-makes-unnecessary).
