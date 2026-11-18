# State and components

[Documentation](README.md) · [Project README](../README.md)

Learn how state updates reach widgets, where component state lives, and how to run asynchronous work safely.

Examples use `ggui`, `widget` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Concepts](#concepts)
- [Signals](#signals)
- [Derived values](#derived-values)
- [Plain models](#plain-models)
- [Ownership and cleanup](#ownership-and-cleanup)
- [Threads](#threads)
- [Runtimes](#runtimes)
- [Readers, bindings, and lenses](#readers-bindings-and-lenses)
- [Builders and components](#builders-and-components)
- [State that outlives a rebuild](#state-that-outlives-a-rebuild)
- [Keeping state while developing](#keeping-state-while-developing)
- [Conditional and key blocks](#conditional-and-key-blocks)
- [Keyed lists](#keyed-lists)
- [Resources and await blocks](#resources-and-await-blocks)

## Concepts

Choose the smallest reactive boundary that expresses the change:

| Need | Use |
| --- | --- |
| Store an editable value | `State` |
| Compute a value from other signals | `Derived`, `Map`, or `Combine` |
| Show a plain Go model the UI was not written for | `Store` and `Select` |
| Run setup once and own local work | `Component` |
| Replace a subtree when data changes | `View` or `Reactive` |
| Keep state through those replacements | `Remember` |
| Keep state through `ggui dev`'s restart after a save | `State(v).Keep(key)` |
| Switch branches or reset an identity | `If` or `Key` |
| Animate a branch out before removing it | `Presence` |
| Build rows once from a plain slice | `List`, or `Children` for another container |
| Follow a list whose rows have no identity | `Each` (rows reused by position) |
| Preserve rows through collection edits | `EachKeyed` |
| Load asynchronously and render its status | `Resource` and `Await` |
| Wait for typing to pause before acting on it | `Debounced` |
| Synchronize with an external system | An owned `Effect` or `Watch` with cleanup |

## Signals

`State(v)` creates a `*StateValue[T]`. `Get()` tracks a read inside a
reactive computation; `Set(v)` and `Update(func(T) T)` publish changes.
`ggui.Peek(value)` reads the current value of any reader without
subscribing, as an action reading state it does not show does; it is
`Untrack(value.Get)`.

```go
count := ggui.State(0)
count.Update(func(n int) int { return n + 1 })
label := widget.Textf("Count: %d", count)
```

State is shallow: assigning `items.Get()[0].Name` does not publish a change.
Copy slices/maps before changing them, then call `Set` or `Update`.
`Append`, `Remove`, `Add`, and `Toggle` work with `Writable` values.

Equality uses a type's `Equal(T) bool` method, otherwise `==` for comparable
non-interface types. Other types notify on every write. `WithEqual(fn)`
customizes comparison; `WithEqual(nil)` always notifies. Equality functions
must be pure. Values with mutable references need immutable snapshots for
meaningful comparisons.

## Derived values

`Derived(func() T)` covers both Svelte `$derived` and `$derived.by`:

```go
doubled := ggui.Derived(func() int { return count.Get() * 2 })
total := ggui.Combine(price, qty, func(p float64, n int) float64 {
    return p * float64(n)
})
```

A `*DerivedValue[T]` computes on its first read and the next read after an
input changes. Unread values do not run during a frame flush. Dependencies
are recollected on each computation; `Get()` settles upstream values before
returning. `ggui.Peek(total)` also returns the latest value.

Derived calculations must be pure: state writes and recursive derived cycles
panic. `WithEqual` controls downstream notification, including slice results.
`Map`, `StateValue.Map`, and `DerivedValue.Map` are convenience derivations,
and `Not` and `Or` combine flags without a closure:

```go
ui.Button("Delete", del).BindDisabled(ggui.Or(m.Busy, ggui.Not(m.HasSelection)))
```

`Textf` and `Sprintf` unwrap arguments with `GetAny() any`: built-in state,
derived values, lenses, tweens and springs all provide it. A custom
`Readable[T]` with only `Get() T` is passed to `fmt.Sprintf` unchanged, rather
than read automatically. Adapt it with an owned derived value:

```go
// Inside Component or app setup; reader is a custom Readable[int].
label := widget.Textf("Count: %d", ggui.Derived(reader.Get))
```

Alternatively, format `reader.Get()` inside a `Derived` callback and pass that
string to `TextOf`. These keep the minimal `Readable` interface intact and use
normal dependency tracking. A custom reader must read reactive sources from
its `Get` method for changes to notify consumers.

A derived value belongs to its creation owner; disposing that owner stops it.
Unowned derivations must be explicitly disposed. A disposed value retains its
last computed result (the zero value if it was never read).

## Plain models

An app whose model is ordinary Go, a document with an undo history say,
need not turn every field into a signal. `NewStore(model)` wraps it, and
`Select` picks what the UI shows: a `Derived` computed again whenever the
store publishes a change, and passed on only when its part differs.

```go
store := ggui.NewStore(&Editor{})
title := ggui.Select(store, func(e *Editor) string { return e.Title })
tags := ggui.Select(store, func(e *Editor) []string { return slices.Clone(e.Tags) }).
    WithEqual(slices.Equal)
name := ggui.SelectBind(store,
    func(e *Editor) string { return e.Name },
    func(e *Editor, v string) { e.Name = v })

widget.TextOf(title)
ui.TextField(name)
ui.Button("Save", store.Action((*Editor).Save))
```

`Update(fn)` changes the model and publishes once, however many Updates `fn`
runs itself; `Action(fn)` is the same as an event handler. A change made
around the store, such as a worker's result posted back, is published with
`Changed()`. Forgetting to publish is the one way a Select goes stale, and a
`ggui_debug` build checks for it after every frame's input: a Select whose
model changed with nothing published is reported with where it was made.
Select copies of slices and maps rather than the model's own, so the next
change is something to compare against.

## Ownership and cleanup

Create local state and work in `Component` setup. `Effect` requires an owner
and runs after mount and layout, before paint. It returns a disposer and its
callback returns a cleanup, or `nil`:

```go
ggui.Effect(func() ggui.Cleanup {
    room := roomID.Get()
    return subscribe(room)
})
```

Cleanup runs untracked before another execution and on disposal. Multiple
writes before the next update are coalesced. An effect disposed before its
first execution never runs. `Watch(source, fn)` has the same deferred timing.
Use `Derived` for computed state rather than copying it through effects.

`OnCleanup(fn)` attaches cleanup directly to the current owner. `Root(fn)`
creates a scope that survives its enclosing computation's reruns; it ends
when its disposer is called or its parent is disposed. Disposers are
idempotent. For app-wide effects, use `App.Setup` or `Probe.Setup`:

```go
app.Setup(func() {
    theme.Bind(dark, theme.Dark(), theme.Default())
})
```

## Threads

State reads/writes, derived calculations, effects, construction and layout
belong to the UI goroutine. `Untrack` changes dependency collection, not
thread safety. A worker receives an immutable input snapshot and posts its
result with `App.Post`. Inside a mounted component or app setup,
`UIThread()` returns the owner's dispatcher. Closing an app drops queued
work; component-specific work must additionally check its lifetime, as
`Resource` does automatically. Debug builds (`-tags ggui_debug`) diagnose
reactive operations from the wrong goroutine while the app is running. The
[settings example](../examples/settings) saves on a worker this way.

`TextOf` and `Textf` borrow their inputs and read during layout; constructing them
creates no effect or derived value and requires no owner. `Textf` copies its
argument list. `Content(text)` and `BindContent(reader)` replace any previous
literal, reader or formatted source. `Sprintf` explicitly creates a derived
string and follows the normal derived ownership rules.

## Runtimes

Every computation belongs to a runtime: its owner's, or, when it has none,
the base runtime of the goroutine that created it. The App's windows share
one runtime, so a signal written in one window updates every window that
reads it. Each `Probe` builds its tree in a runtime of its own, beside its
own animations, clock and environment: its frames run its own effects and
never another probe's, a probe a test forgets to close leaves nothing
running in the next test, and `Probe.Advance` moves that probe's time alone.

Signals belong to no runtime. One signal can feed an App and several probes
at once, and a derived value built in one runtime can be read from another.
What is created outside every tree, such as a widget built before its probe
or state made in `main`, stays in its goroutine's base runtime, which the
app or probe made on that goroutine flushes as well.

The settings `SetEnv`, `SetClock`, `SetDefaultFont`, `SetEmojiFont` and `SetIME`
make belong to the same place: an app's or a probe's own when made during
its frames, and before any frame the goroutine's base, which every app and
probe made on that goroutine inherits until it is given its own.

What is running -- the computation reads subscribe, the owner new effects
join, the loop whose frame runs -- is kept per goroutine. Probes on
goroutines of their own, as `t.Parallel` runs them, therefore run at the
same time without seeing each other, as long as each keeps to its own
signals: a signal written on one goroutine and read by another's
computations is a data race, as it would be for any Go value.

## Readers, bindings, and lenses

| Interface | Methods | Use |
| --- | --- | --- |
| `Readable[T]` | `Get()` | Display-only data, including derived values. |
| `Binding[T]` | `Get()`, `Set(T)` | Two-way controls and animated values. |
| `Writable[T]` | `Binding[T]`, `Update(func(T) T)` | Immediate read-modify-write helpers. |

`BindX` never owns or disposes its reader. A reader must outlive the widget that
uses it. Nil, including a typed nil reader, is rejected; `X(value)` detaches it.
`Const(value)` adapts a literal only where a reader is required, for example
`ui.Progress(ggui.Const(0.5))`. `Bind(get, set)` adapts a getter and setter
pair, typically model methods, where a control needs a `Binding[T]`.
`Controlled(value, set)` binds a control to a value taken when its view was
built and hands a change to `set`, which may refuse it; the view around it
rebuilds when the model changes, so a `ggui_debug` build does not warn that it
reads no signal, as it does for `Bind`.

Layout bindings such as `BindDisabled`, `BindName` and `BindOptions` track
signal reads made inside a custom reader's `Get`, including under layout caches.
A getter over plain, non-reactive storage cannot notify the UI: publish through
a signal, or have the custom widget call `Invalidate` with its last layout Env.

`Field` and `Lens` bind controls to parts of a state value:

```go
form := ggui.State(Form{Name: "Ada"})
name := form.Field(func(f *Form) *string { return &f.Name })
field := ui.TextField(name)
```

`Field` copies the struct, not the nested objects it points to. Use `Lens`
with explicit copy logic for nested mutable containers. The
[settings example](../examples/settings) binds a whole form this way. Tweens and springs
implement `Binding`, not `Writable`; use `Target()` when updating a motion's
destination rather than its current animated value.

## Builders and components

`Component(func() Widget)` runs its setup once per mount, without tracking
setup reads. The app's root constructor also runs once. Bind values to
widgets or use explicit reactive blocks for subsequent changes:

```go
func Counter() ggui.Widget {
    return ggui.Component(func() ggui.Widget {
        count := ggui.State(0)
        return widget.Column(
            widget.Textf("Count: %d", count),
            ui.Button("Increment", func() { ggui.Add(count, 1) }),
        )
    })
}
```

Every builder runs untracked: app and `Component` setup, and the callbacks of
`View`, `If`, `Key` and `Each`. A snapshot expression such as
`Text(fmt.Sprint(count.Get()))` in one shows the value once and never again.
`TextOf`, `Textf` and reactive control bindings follow the value instead. A
`ggui_debug` build reports every place a builder reads a signal, once; read it
with `ggui.Peek` when a snapshot is what you mean. A keyed row reads what it keeps
for life, such as its ID, from `EachItem.Item`.

`View(source, build)` and `Reactive(build)` are explicit subtree replacement
boundaries: their callbacks rerun and replace locally created state/work. Keep
state outside these callbacks if it must survive their updates. `View` follows
only `source`; `Reactive` follows whatever its callback reads, which makes it
the one builder that is not untracked.

`View(state, build)` rebuilds on every change to `state`, whichever field
changed. `ViewOf(state, pick, build)` rebuilds only when the part `pick`
returns changes, so selecting an item need not rebuild a form that shows
only the settings:

```go
ggui.ViewOf(app, func(s AppState) Settings { return s.Settings }, settingsForm)
```

A state's value is compared with its `Equal` method or `==`. A struct that
holds a slice or a map has neither, so every `Set` notifies, even one that
changed nothing: give the type an `Equal` method, use `WithEqual`, or pick
comparable parts with `ViewOf` or `Map`. Svelte's snippet `{@render}` has no special
runtime counterpart; use ordinary Go functions to compose reusable widgets.

## State that outlives a rebuild

What a `View` builds goes when it rebuilds. State that should stay, a field's
draft while the page around it is rebuilt say, is asked for with
`Remember(key, init)`: the first build that asks makes it with `init`, and
every later build asking for the same key gets the same value back.

```go
ggui.View(fields, func(fs []FieldSpec) ggui.Widget {
    rows := []ggui.Widget{}
    for _, f := range fs {
        draft := ggui.Remember(f.ID, func() *ggui.DraftValue[string] {
            return ggui.Draft(f.Value, f.Commit)
        })
        rows = append(rows, ui.Field(f.Label, ui.TextField(draft)).BindError(draft.Error()))
    }
    return widget.Column(rows...)
})
```

A value lasts as long as builds keep asking for it. One that a View's build
stops asking for is let go when that build ends, and so is one asked for by a
View its parent rebuilt without; asking again later makes a fresh one. Keys
belong to the nearest `Component`, `If` or `Key` branch, or `Each` row,
together with the value's type, and everything remembered there goes when
it does. `init` runs untracked under an owner of its own, so a `Derived` or
`Effect` it makes lasts exactly as long as the value. Outside every builder,
`Remember` returns `init()`.

Focus, the caret and a `Scroll`'s offset need none of this: a rebuilt
control with the same `Key` takes them over from the one it replaced.

## Keeping state while developing

[`ggui dev`](cli.md#keeping-state-across-a-restart) restarts the app after
each save. `Keep(key)` marks a state that should come through the restart,
so that the form half filled in, the tab that was open and the page shown
are there on the new build. It returns the state, so it chains onto
`State`; the router keeps its location with one line of its own:

```go
func SettingsPage(c *router.Context) ggui.Widget {
    return ggui.Component(func() ggui.Widget {
        tab := ggui.State("general").Keep("settings.tab")
        profile := ggui.State(Profile{}).Keep("settings.profile")
        return widget.Column(
            ui.ToggleGroup(tab).Options(settingsTabs),
            profileForm(profile),
        )
    })
}

r := router.New(
    router.Page("/", home),
    router.Page("/settings", SettingsPage),
).Keep("router")
```

Before the old instance stops, its kept values are encoded as JSON, read
on the UI thread between frames; the new instance restores each one when
`Keep` is called for its key, before `Keep` returns. A state kept in app
setup therefore shows its value in the first frame, and one kept in a
component that mounts later, as the settings page does once the router
has restored `/settings`, takes it when it mounts. The router's kept URL
replaces its history's first entry; the entries behind it are not kept.

- **Claimed once.** A value is restored into the first state that keeps its
  key; a component mounted again afterwards starts from its initial value,
  as it would without `ggui dev`. A value no state has claimed yet, for a
  page not shown since the restart, is handed on to the next build.
- **Owned.** A state is kept while its owner lives, and let go with it:
  one kept in a page that was closed is not handed over.
- **One state a key.** A key kept by two states at once belongs to the one
  kept last, and a `ggui_debug` build says so once. A component shown more
  than once needs an ID in its key.
- **Type changes.** A value is decoded over a JSON copy of the state's
  initial value: a field added since keeps its initial value, one removed
  is ignored. A value that no longer decodes is dropped, said once on
  stderr, and the state keeps its initial value; one that does not encode,
  a func say, is not kept. What JSON leaves out, unexported fields, is not
  kept either, so keep plain data rather than a model with private parts.
- **Everywhere `ggui dev` runs.** In the viewer or a window of the app's
  own, on macOS, Linux and every Windows, where it travels over a named
  pipe rather than a unix socket, and in a browser with `--web`.
- **Free elsewhere.** Without `ggui dev`, as in a release build or a test,
  `Keep` does nothing: it returns the state after one check, and nothing
  runs, listens or is stored.

## Conditional and key blocks

```go
ggui.If(signedIn, func() ggui.Widget {
    return ProfileScreen()
}).Else(func() ggui.Widget {
    return LoginScreen()
})

ggui.Key(documentID, func(id string) ggui.Widget {
    return Editor(id)
})
```

`If` creates only the active branch; `.ElseIf` and `.Else` add branches.
Leaving a branch disposes its state, effects and resources. Returning creates
a fresh instance. Branch factories are builders, untracked like the rest; use
bindings or `View` inside them. `Key` recreates its subtree when its comparable key changes.
Configure fluent blocks before mount; later configuration panics.

A widget of your own mounts subtrees the way `Key`, `Switcher` and the rows
of `EachKeyed` do with a `Scope`, the place a builder runs: `UseScope()` in
a component's setup takes it, and `scope.Mount(build)` builds a subtree there
as a branch is built, untracked, with a `Remember` memory and an identity of
its own, returning it with the `Cleanup` that disposes it. What is mounted
stays until that `Cleanup` runs or the scope is disposed, so a switcher can
keep the old subtree while the new one mounts. `scope.MountAs(id, name,
build)` gives the subtree's controls the identity `id`, which a subtree
mounted again under the same `id` keeps, as a list's row does. `Follow(key,
fn)` calls `fn` with the key now, before it returns, and again whenever the
key changes to a different value, untracked; unlike `Watch`, which waits
for the frame's effects, it has the first subtree mounted by the first
layout. A component whose content changed with no rebuild, as a switcher's
does when a key mounts, calls `Invalidate()` on itself:

```go
comp = ggui.Component(func() ggui.Widget {
    scope := ggui.UseScope()
    ggui.Follow(page, func(p Page) {
        if dispose != nil {
            dispose()
        }
        shown, dispose = scope.Mount(func() ggui.Widget { return view(p) })
        comp.Invalidate()
    })
    return body
})
```

The keyboard key type is `KeyboardKey`; constants such as `KeyEnter` and the
`KeyEvent.Key` field retain their names. Widget `.Key(id)` setters still assign
input identity and are distinct from the `Key` control-flow block.

## Keyed lists

`Each(items, row)` reuses rows by position and permits duplicate values.
`EachKeyed(items, key, row)` preserves row identity across reordering:

```go
widget.EachKeyed(todos, func(t Todo) int { return t.ID },
    func(row widget.EachItem[Todo]) ggui.Widget {
        return ggui.View(row.Value, func(t Todo) ggui.Widget {
            return widget.Text(t.Title)
        })
    },
).Else(func() ggui.Widget { return widget.Text("No items") })
```

Each row gets reactive `Value` and `Index`. Its factory runs once per row
instance. Duplicate keys panic before any row update is applied. Empty-list
branches have their own lifetime. These blocks are layout widgets (vertical
by default), not fragments spliced into their parent's children.

`Gap`, `Space`, `Align`, and `Horizontal` configure layout. `ItemExtent` inside
`Scroll` virtualizes fixed-size rows. `EstimatedExtent(v)` virtualizes rows of
any size: a row counts as `v` long until it is first laid out and as its own
size after, and as the sizes of rows above the window come to be known, or rows
come in above it, the `Scroll` moves by as much, so what is in view stays in
place. `Retain(n)` bounds offscreen instances.
Evicted rows lose local state and are recreated when visible; focused or
captured rows are retained. `Transition` preserves exiting rows until their
animation completes; virtualization removes rows immediately.

## Resources and await blocks

`Resource(input, load, options...)` tracks input on the UI goroutine and runs
`load(context.Context, input)` on a worker. Create it in app setup or a mounted
component. Copy mutable input references before returning them from `input`.

```go
users := ggui.Resource(
    func() string { return query.Get() },
    func(ctx context.Context, q string) ([]User, error) {
        return api.SearchUsers(ctx, q)
    },
)

view := ggui.Await(users).
    Pending(func() ggui.Widget { return widget.Text("Searching…") }).
    Then(func(value ggui.Readable[[]User]) ggui.Widget {
        return widget.EachKeyed(value, userID, userRow)
    }).
    Catch(func(err ggui.Readable[error]) ggui.Widget {
        return widget.Textf("Search failed: %v", err)
    })
```

Input changes cancel the prior context. Request IDs and disposal checks
prevent old completions from committing, even when a worker ignores
cancellation. `Reload()` retries with the current input. `ResourceEqual(fn)`
customizes input equality; `nil` restarts on every input notification.
Resources execute independently of consumers; multiple `Await` blocks share
one task. Unmounting a consumer does not cancel a resource owned elsewhere.

A search that runs as the user types should wait for the typing to pause.
`Debounced(r, d)` follows `r` late: it takes the first value at once, and
afterwards a value only once `r` has held it unchanged for `d`. Feed it to
the resource, and the search runs once per pause and is cancelled when the
query changes again:

```go
q := ggui.Debounced(query, 250*time.Millisecond)
res := ggui.Resource(q.Get, search)
searching := ggui.Derived(func() bool {
    return q.Pending() || res.Get().Status == ggui.Pending
})
```

`q` keeps the equality of a `State`: typing that settles on the text it
started from, as `a`, `ab`, `a` does, leaves `q` unchanged, and no second
search runs for text already searched. So a "searching…" indicator shown
from the first keystroke must not wait for a new result alone, which in that
case never comes. `q.Pending()` is true from the change in `query` until the
wait ends, whether or not `q` then changes; `searching` above reads both, and
when the wait does start a search, the resource is pending in the same frame,
so the indicator never flickers off in between.

Like `Resource`, `Debounced` needs an owner with a frame loop (app setup or
a mounted component) and stops with it. It writes only on the UI goroutine,
and measures `d` by the clock `ClockTime` reads, so a test steps it with
`Probe.Advance`:

```go
query.Set("abc")
p.Frame()
p.Advance(250 * time.Millisecond) // q is now "abc"
```

`Await` accepts any `Readable[AsyncState[T]]`. A snapshot has `RequestID`,
`Status` (`Pending`, `Ready`, `Failed`), `Value`, and `Err`. Zero values are
valid successful results. Internally canceled requests do not display errors.
Omitted branches are empty. Within one request and status, the branch and its
reactive value are preserved; a new request creates a new branch even when
its intermediate pending state is not painted. Retaining stale results while
refreshing is not part of this API.
