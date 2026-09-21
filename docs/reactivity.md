# State and components

[Documentation](README.md) · [Project README](../README.md)

Learn how state updates reach widgets, where component state lives, and how to run asynchronous work safely.

Examples use `ggui` and `ui` imports and application-defined placeholders.
See [example conventions](README.md#start-here) before copying snippets.

## On this page

- [Concepts](#concepts)
- [Signals](#signals)
- [Derived values](#derived-values)
- [Ownership and cleanup](#ownership-and-cleanup)
- [Threads](#threads)
- [Readers, bindings, and lenses](#readers-bindings-and-lenses)
- [Builders and components](#builders-and-components)
- [Conditional and key blocks](#conditional-and-key-blocks)
- [Keyed lists](#keyed-lists)
- [Resources and await blocks](#resources-and-await-blocks)

## Concepts

Choose the smallest reactive boundary that expresses the change:

| Need | Use |
| --- | --- |
| Store an editable value | `State` |
| Compute a value from other signals | `Derived`, `Map`, or `Combine` |
| Run setup once and own local work | `Component` |
| Replace a subtree when data changes | `View` or `Reactive` |
| Switch branches or reset an identity | `If` or `Key` |
| Preserve rows through collection edits | `EachKeyed` |
| Load asynchronously and render its status | `Resource` and `Await` |
| Synchronize with an external system | An owned `Effect` or `Watch` with cleanup |

## Signals

`State(v)` creates a `*StateValue[T]`. `Get()` tracks a read inside a
reactive computation; `Set(v)` and `Update(func(T) T)` publish changes.
`Untrack(value.Get)` reads the current value without subscribing.

```go
count := ggui.State(0)
count.Update(func(n int) int { return n + 1 })
label := ggui.Textf("Count: %d", count)
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
returning. `Untrack(total.Get)` also returns the latest value.

Derived calculations must be pure: state writes and recursive derived cycles
panic. `WithEqual` controls downstream notification, including slice results.
`Map`, `StateValue.Map`, and `DerivedValue.Map` are convenience derivations.
A derived value belongs to its creation owner; disposing that owner stops it.
Unowned derivations must be explicitly disposed. A disposed value retains its
last computed result (the zero value if it was never read).

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
    ggui.BindTheme(dark, ggui.DarkTheme(), ggui.DefaultTheme())
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

## Readers, bindings, and lenses

| Interface | Methods | Use |
| --- | --- | --- |
| `Readable[T]` | `Get()` | Display-only data, including derived values. |
| `Binding[T]` | `Get()`, `Set(T)` | Two-way controls and animated values. |
| `Writable[T]` | `Binding[T]`, `Update(func(T) T)` | Immediate read-modify-write helpers. |

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
        return ggui.Column(
            ggui.Textf("Count: %d", count),
            ui.Button("Increment", func() { ggui.Add(count, 1) }),
        )
    })
}
```

Snapshot expressions such as `Text(fmt.Sprint(count.Get()))` do not subscribe setup. `TextOf`, `Textf`
and reactive control bindings do. `View(source, build)` and `Reactive(build)`
are explicit subtree replacement boundaries: their callbacks rerun and
replace locally created state/work. Keep state outside these callbacks if
it must survive their updates. Svelte's snippet `{@render}` has no special
runtime counterpart; use ordinary Go functions to compose reusable widgets.

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
a fresh instance. Reads in branch factories are untracked; use bindings or
`View` inside them. `Key` recreates its subtree when its comparable key changes.
Configure fluent blocks before mount; later configuration panics.

The keyboard key type is `KeyboardKey`; constants such as `KeyEnter` and the
`KeyEvent.Key` field retain their names. Widget `.Key(id)` setters still assign
input identity and are distinct from the `Key` control-flow block.

## Keyed lists

`Each(items, row)` reuses rows by position and permits duplicate values.
`EachKeyed(items, key, row)` preserves row identity across reordering:

```go
ggui.EachKeyed(todos, func(t Todo) int { return t.ID },
    func(row ggui.EachItem[Todo]) ggui.Widget {
        return ggui.View(row.Value, func(t Todo) ggui.Widget {
            return ggui.Text(t.Title)
        })
    },
).Else(func() ggui.Widget { return ggui.Text("No items") })
```

Each row gets reactive `Value` and `Index`. Its factory runs once per row
instance. Duplicate keys panic before any row update is applied. Empty-list
branches have their own lifetime. These blocks are layout widgets (vertical
by default), not fragments spliced into their parent's children.

`Gap`, `Space`, `Align`, and `Horizontal` configure layout. `ItemExtent` inside
`Scroll` virtualizes fixed-size rows; `Retain(n)` bounds offscreen instances.
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
    Pending(func() ggui.Widget { return ggui.Text("Searching…") }).
    Then(func(value ggui.Readable[[]User]) ggui.Widget {
        return ggui.EachKeyed(value, userID, userRow)
    }).
    Catch(func(err ggui.Readable[error]) ggui.Widget {
        return ggui.Textf("Search failed: %v", err)
    })
```

Input changes cancel the prior context. Request IDs and disposal checks
prevent old completions from committing, even when a worker ignores
cancellation. `Reload()` retries with the current input. `ResourceEqual(fn)`
customizes input equality; `nil` restarts on every input notification.
Resources execute independently of consumers; multiple `Await` blocks share
one task. Unmounting a consumer does not cancel a resource owned elsewhere.

`Await` accepts any `Readable[AsyncState[T]]`. A snapshot has `RequestID`,
`Status` (`Pending`, `Ready`, `Failed`), `Value`, and `Err`. Zero values are
valid successful results. Internally canceled requests do not display errors.
Omitted branches are empty. Within one request and status, the branch and its
reactive value are preserved; a new request creates a new branch even when
its intermediate pending state is not painted. Retaining stale results while
refreshing is not part of this API.
