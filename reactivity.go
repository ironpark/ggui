package ggui

import (
	"context"
	"reflect"
	"slices"
	"time"

	"github.com/ironpark/ggui/internal/reactive"
)

// The reactive core lives in internal/reactive: signals, effects, memos and
// the ownership tree, which need nothing from widgets or painting. It is
// reached through the names it has always had, so that ggui.State and
// ggui.Effect keep meaning what they did.
//
// Generic types can be aliased and bring their methods with them. Generic
// functions cannot, so those are one-line forwards, as ggui/a11y does for
// the geometry constructors.

type (
	// Readable is anything a computation can read and subscribe to.
	Readable[T any] = reactive.Readable[T]
	// Writable is anything a caller can write.
	Writable[T any] = reactive.Writable[T]
	// Binding is both, as a widget takes for two-way state.
	Binding[T any] = reactive.Binding[T]
	// StateValue is a signal: the unit of reactive state.
	StateValue[T any] = reactive.StateValue[T]
	// DerivedValue is a memo: a value computed from other signals.
	DerivedValue[T any] = reactive.DerivedValue[T]
	// Lens is a two-way view onto part of a StateValue.
	Lens[U any] = reactive.Lens[U]
	// Store makes a plain Go model something the UI follows: Select picks
	// what it shows, and Update publishes a change.
	Store[M any] = reactive.Store[M]
	// Cleanup is what an Effect returns to undo itself.
	Cleanup = reactive.Cleanup
)

// State returns a new signal holding v.
func State[T any](v T) *StateValue[T] { return reactive.State(v) }

// Derived returns a memo of fn, recomputed when what it reads changes.
func Derived[T any](fn func() T) *DerivedValue[T] { return reactive.Derived(fn) }

// Sprintf formats like fmt.Sprintf, reading arguments that implement
// GetAny() any on each computation. StateValue, DerivedValue, Lens, Tweened
// and Sprung implement that method. Other arguments pass through unchanged;
// Readable's Get() alone is not enough. Adapt a custom Readable with
// Derived(reader.Get), or format its Get() inside a Derived callback.
func Sprintf(format string, args ...any) *DerivedValue[string] {
	args = append([]any(nil), args...)
	return Derived(func() string { return reactive.Format(format, args) })
}

// Map returns a memo of fn applied to r.
func Map[T, U any](r Readable[T], fn func(T) U) *DerivedValue[U] { return reactive.Map(r, fn) }

// Combine returns a memo of fn applied to a and b.
func Combine[A, B, C any](a Readable[A], b Readable[B], fn func(A, B) C) *DerivedValue[C] {
	return reactive.Combine(a, b, fn)
}

// Watch runs fn whenever src changes, and returns a function to stop.
func Watch[T any](src Readable[T], fn func(T)) (dispose func()) { return reactive.Watch(src, fn) }

// Effect runs fn now and again whenever what it read changes.
func Effect(fn func() Cleanup) Cleanup { return reactive.Effect(fn) }

// Observe runs fn now and again as soon as what it read changes, before the
// layout that follows rather than after it as Effect does: for a widget
// keeping one piece of its state in step with another that layout reads. It
// returns a function to stop.
func Observe(fn func()) (dispose func()) { return reactive.Observe(fn) }

// Root runs fn under a fresh owner and returns a function to dispose it.
func Root(fn func()) (dispose func()) { return reactive.Root(fn) }

// OnCleanup registers fn to run when the current owner is disposed.
func OnCleanup(fn func()) { reactive.OnCleanup(fn) }

// Untrack runs fn without subscribing the running Effect to what it reads.
func Untrack[T any](fn func() T) T { return reactive.Untrack(fn) }

// Peek returns r's current value without subscribing anything to it, as
// Untrack(r.Get) does: for an action reading state it does not show, or a
// builder taking a snapshot on purpose.
func Peek[T any](r Readable[T]) T { return reactive.Untrack(r.Get) }

// Toggle inverts a boolean signal.
func Toggle(s Writable[bool]) { reactive.Toggle(s) }

// Add increments a numeric signal by d.
func Add[N Number](s Writable[N], d N) { reactive.Add(s, d) }

// Append appends items to a slice signal.
func Append[T any](s Writable[[]T], items ...T) { reactive.Append(s, items...) }

// Remove drops the elements of a slice signal for which drop returns true.
func Remove[T any](s Writable[[]T], drop func(T) bool) { reactive.Remove(s, drop) }

// Const returns a Readable that never changes.
func Const[T any](v T) Readable[T] { return reactive.Const(v) }

// NewStore wraps model.
func NewStore[M any](model M) *Store[M] { return reactive.NewStore(model) }

// Select is the part of the store's model fn picks, computed again when the
// store publishes a change and passed on to its readers only when it
// differs.
func Select[M, T any](s *Store[M], fn func(M) T) *DerivedValue[T] { return reactive.Select(s, fn) }

// SelectBind is a Binding over the part of the model get picks: a control
// shows it and hands an edit to set, which runs as an Update.
func SelectBind[M, T any](s *Store[M], get func(M) T, set func(M, T)) Binding[T] {
	return reactive.SelectBind(s, get, set)
}

// Bind adapts a getter and setter pair into a Binding, for controls that
// edit state owned by a model method rather than a signal. The getter
// should read a signal, through the model's StateValue, Lens or Field:
// one that reads plain storage, or a copy captured when the control was
// built, leaves the control showing a stale value, and a ggui_debug build
// says so the first time it runs. State.Field and State.Lens bind part of
// a struct directly.
func Bind[T any](get func() T, set func(T)) Binding[T] {
	return &funcBinding[T]{get: get, set: set, origin: reactive.Origin()}
}

type funcBinding[T any] struct {
	get     func() T
	set     func(T)
	origin  string // where Bind was called, in a ggui_debug build
	checked bool
}

func (b *funcBinding[T]) Get() T {
	if !reactive.Debug || b.checked {
		return b.get()
	}
	b.checked = true
	return countReads(b.origin, b.get)
}

func (b *funcBinding[T]) Set(v T) { b.set(v) }

// Controlled binds a control to value, a copy taken when the view was
// built, and asks set for a change instead of making it. It is for a
// control whose change the model may refuse, such as a mode switch that a
// form with an invalid entry blocks: a change set takes rebuilds the view
// with the new value, and one it refuses leaves the control showing value.
// Unlike Bind, it reads no signal on purpose, so the view around it must
// rebuild when the model changes.
//
//	ggui.View(mode, func(m int) ggui.Widget {
//		return ui.ToggleGroup(ggui.Controlled(m, model.SwitchMode)).Options(modes)
//	})
func Controlled[T any](value T, set func(T)) Binding[T] {
	return controlled[T]{value, set}
}

type controlled[T any] struct {
	value T
	set   func(T)
}

func (c controlled[T]) Get() T  { return c.value }
func (c controlled[T]) Set(v T) { c.set(v) }

// countReads runs get, telling a layout recording in progress about what it
// reads as usual, and reports a getter that read no signal.
func countReads[T any](origin string, get func() T) T {
	var v T
	reads := 0
	outer := reactive.Recorder()
	reactive.Measure(func(src reactive.LayoutSource, version uint64) {
		reads++
		if outer != nil {
			outer(src, version)
		}
	}, func() { v = get() })
	if reads == 0 {
		reactive.ReportOnce("bind "+origin, "ggui: the Bind getter at %s read no signal, so its control will not see changes made elsewhere; read a StateValue, or bind with State.Field or State.Lens", origin)
	}
	return v
}

// Not is true while r is false, as a control's BindDisabled wants a "can"
// turned around.
func Not(r Readable[bool]) *DerivedValue[bool] {
	return Map(r, func(v bool) bool { return !v })
}

// Or is true while any one of rs is, as a control disabled while busy or
// while nothing is selected is.
func Or(rs ...Readable[bool]) *DerivedValue[bool] {
	return Derived(func() bool {
		for _, r := range rs {
			if r.Get() {
				return true
			}
		}
		return false
	})
}

// Remember returns the value made for key the last time the widgets around
// it were built, or makes one with init the first time. It is how state a
// View or Reactive builds outlives that View's rebuilds, as a field's draft
// should when the page around it is rebuilt:
//
//	ggui.View(fields, func(fs []Field) ggui.Widget {
//		rows := []ggui.Widget{}
//		for _, f := range fs {
//			text := ggui.Remember(f.ID, func() *ggui.StateValue[string] { return ggui.State(f.Value) })
//			rows = append(rows, ui.TextField(text))
//		}
//		return widget.Column(rows...)
//	})
//
// A value is kept while the builds around it keep asking for it: one a
// View's build stops asking for is let go when that build ends, and so is
// one asked for by a View its parent rebuilt without, and everything made
// in a Component, an If or Key branch or an Each row goes with it. init
// runs untracked under an owner of its own, so a Derived or an Effect it
// makes lives exactly as long as the value.
//
// Keys belong to the nearest Component, branch or row, together with the
// type of the value, so different kinds of state can share a key; two
// Views asking for one key and type share the value. Outside every
// builder, Remember just returns init().
func Remember[K comparable, T any](key K, init func() T) T {
	b, _ := reactive.Memory().(*boundary)
	if b == nil {
		return Untrack(init)
	}
	m := b.mem
	k := memoKey{key, reflect.TypeFor[T]()}
	e := m.entries[k]
	if e == nil {
		e = &memoEntry{}
		reactive.WithOwner(m.owner, func() {
			e.dispose = reactive.Root(func() { e.value = init() })
		})
		if m.entries == nil {
			m.entries = map[memoKey]*memoEntry{}
		}
		m.entries[k] = e
	}
	if e.hold(b) {
		b.asked = append(b.asked, k)
	}
	return e.value.(T)
}

// memory is what one Component, branch, row or app keeps remembered values
// in; it goes when its owner does, and the values with it.
type memory struct {
	owner   *reactive.Computation
	entries map[memoKey]*memoEntry
	orphans []memoKey // asked for by Views since disposed; see boundary.build
}

type memoKey struct {
	key any
	typ reflect.Type
}

type memoEntry struct {
	value   any
	dispose func()
	holders map[*boundary]uint64 // live boundaries holding this entry → boundary.gen last asked
}

// hold records that b asked for e during b.gen, reporting whether this is the
// first time b asked in this build.
func (e *memoEntry) hold(b *boundary) bool {
	if e.holders == nil {
		e.holders = map[*boundary]uint64{b: b.gen}
		return true
	}
	if seen, ok := e.holders[b]; ok && seen == b.gen {
		return false
	}
	e.holders[b] = b.gen
	return true
}

// release drops b's hold when b did not re-ask for e in gen, and reports
// whether no live holders remain.
func (e *memoEntry) release(b *boundary, gen uint64) bool {
	if seen, ok := e.holders[b]; ok && seen != gen {
		delete(e.holders, b)
	}
	return e.pruneDisposed()
}

// pruneDisposed removes disposed holders and reports whether none remain.
func (e *memoEntry) pruneDisposed() bool {
	for h := range e.holders {
		if h.disposed {
			delete(e.holders, h)
		}
	}
	return len(e.holders) == 0
}

// boundary is a builder Remember is asked in: a View or Reactive, which
// runs again, or the setup of a Component, branch, row or app, which runs
// once and owns the memory.
type boundary struct {
	mem      *memory
	own      memory    // the memory, when this builder owns it
	gen      uint64    // its builds so far
	asked    []memoKey // what its last build asked for
	disposed bool
}

// remembering runs fn, a builder that runs once, with a memory of its own
// that lasts as long as the owner running it.
func remembering[T any](fn func() T) T {
	b := &boundary{}
	b.own.owner = reactive.CurrentOwner()
	b.mem = &b.own
	prev := reactive.SwapMemory(b)
	defer reactive.SwapMemory(prev)
	return reactive.Build(fn)
}

// newBoundary is the boundary of a View or Reactive being made now, in the
// memory of the builder making it, or nil outside every builder.
func newBoundary() *boundary {
	parent, _ := reactive.Memory().(*boundary)
	if parent == nil {
		return nil
	}
	b := &boundary{mem: parent.mem}
	if reactive.CurrentOwner() != nil {
		OnCleanup(func() {
			b.disposed = true
			b.mem.orphans = append(b.mem.orphans, b.asked...)
			b.asked = nil
		})
	}
	return b
}

// build runs one build of the boundary, then lets go of what it asked for
// last time and not this time, and of what a View this build replaced
// asked for and no new one did.
func (b *boundary) build(fn func() Widget) Widget {
	if b == nil {
		return fn()
	}
	b.gen++
	last := b.asked
	b.asked = nil
	prev := reactive.SwapMemory(b)
	defer func() {
		reactive.SwapMemory(prev)
		b.mem.drop(last, func(e *memoEntry) bool { return e.release(b, b.gen) })
		orphans := b.mem.orphans
		b.mem.orphans = nil
		b.mem.drop(orphans, func(e *memoEntry) bool { return e.pruneDisposed() })
	}()
	return fn()
}

// drop disposes the entries of keys that gone reports gone.
func (m *memory) drop(keys []memoKey, gone func(*memoEntry) bool) {
	for _, k := range keys {
		if e := m.entries[k]; e != nil && gone(e) {
			delete(m.entries, k)
			e.dispose()
		}
	}
}

// DraftValue is an edit held back from the value it edits until it is
// committed. Build one with Draft.
type DraftValue[T any] struct {
	source Readable[T]
	commit func(T) error
	value  *StateValue[T]
	dirty  *StateValue[bool]
	err    *StateValue[string]
	eq     func(a, b T) bool
}

// Committer is a Binding that holds an edit back until it is committed, as
// a Draft does. A text input bound to one commits it when it loses focus
// or submits, and reverts it on Escape.
type Committer interface {
	// Commit hands the edit on and reports whether it was taken.
	Commit() bool
	// Revert drops the edit and any error, reporting whether there was
	// either.
	Revert() bool
}

// Draft is a Binding for a text field that edits source without writing
// it on every keystroke: the field shows source until it is edited, then
// the edit, until a commit hands the edit to commit. A commit that fails
// keeps the edit and shows commit's error through Error, so the user can
// repair it; Escape, or Revert, goes back to source.
//
//	name := ggui.Draft(ggui.Select(store, (*Project).Name), func(v string) error {
//		return project.Rename(v)
//	})
//	ui.Field("Name", ui.TextField(name)).BindError(name.Error())
//
// commit runs when the field loses focus or submits, as Commit does. It
// writes the value on to the model, and source follows from there; source
// may normalize what it was given. A Draft made in a View's build is lost
// with that build; keep one with Remember to have it, edit and error
// included, outlive the page rebuilding around it.
func Draft[T any](source Readable[T], commit func(T) error) *DraftValue[T] {
	return &DraftValue[T]{
		source: source,
		commit: commit,
		value:  State(*new(T)),
		dirty:  State(false),
		err:    State(""),
		eq:     reactive.ComparableEqual[T](),
	}
}

// Get returns the edit while there is one, and source otherwise.
func (d *DraftValue[T]) Get() T {
	if d.dirty.Get() {
		return d.value.Get()
	}
	return d.source.Get()
}

// GetAny implements the reader Textf and Sprintf unwrap.
func (d *DraftValue[T]) GetAny() any { return d.Get() }

// Set makes v the edit, or drops the edit when v is what source holds, and
// clears the error a failed commit left.
func (d *DraftValue[T]) Set(v T) {
	if d.eq != nil && d.eq(v, Untrack(d.source.Get)) {
		d.dirty.Set(false)
	} else {
		d.value.Set(v)
		d.dirty.Set(true)
	}
	d.err.Set("")
}

// Commit hands the edit to commit, if there is one, and reports whether it
// was taken. A failed commit keeps the edit and sets Error.
func (d *DraftValue[T]) Commit() bool {
	if !Untrack(d.dirty.Get) {
		return true
	}
	if err := d.commit(Untrack(d.value.Get)); err != nil {
		d.err.Set(err.Error())
		return false
	}
	d.dirty.Set(false)
	d.err.Set("")
	return true
}

// Revert drops the edit and the error, and reports whether there was
// either.
func (d *DraftValue[T]) Revert() bool {
	had := Untrack(d.dirty.Get) || Untrack(d.err.Get) != ""
	d.dirty.Set(false)
	d.err.Set("")
	return had
}

// Fail shows msg as the error, as a check made elsewhere does; Set and a
// commit clear it.
func (d *DraftValue[T]) Fail(msg string) { d.err.Set(msg) }

// Error is the error of the last failed commit, or Fail's, or "".
func (d *DraftValue[T]) Error() Readable[string] { return d.err }

// Dirty reports whether there is an edit not yet committed.
func (d *DraftValue[T]) Dirty() Readable[bool] { return d.dirty }

// AsyncStatus is the state of one asynchronous request.
type AsyncStatus uint8

const (
	Pending AsyncStatus = iota
	Ready
	Failed
)

// AsyncState is an atomic snapshot. Value is valid in Ready (including its
// zero value); Err is valid in Failed. RequestID identifies a request, not a
// frame, so fast requests still remount an Await branch.
type AsyncState[T any] struct {
	RequestID uint64
	Status    AsyncStatus
	Value     T
	Err       error
}

// ResourceOption configures input comparison before the first request.
type ResourceOption[I any] func(*resourceOptions[I])
type resourceOptions[I any] struct{ equal func(I, I) bool }

// ResourceEqual supplies input equality. Nil restarts on every notification.
func ResourceEqual[I any](equal func(I, I) bool) ResourceOption[I] {
	return func(options *resourceOptions[I]) { options.equal = equal }
}

// ResourceValue owns a cancellable asynchronous task. Read it on the UI
// goroutine; workers receive input snapshots and return results through Post.
type ResourceValue[T any] struct {
	state    *StateValue[AsyncState[T]]
	reload   *StateValue[uint64]
	disposed bool
}

// Resource tracks input on the UI goroutine and runs load on a worker. Create
// it in an app/probe setup or a mounted Component. Mutable input references
// must be copied by input before handing them to the worker.
func Resource[I, T any](input func() I, load func(context.Context, I) (T, error), options ...ResourceOption[I]) *ResourceValue[T] {
	reactive.CheckUIThread("Resource")
	owner := reactive.CurrentOwner()
	if owner == nil || owner.Loop() == nil {
		panic("ggui: Resource requires an app or mounted component owner")
	}
	opts := resourceOptions[I]{equal: reactive.ComparableEqual[I]()}
	for _, option := range options {
		option(&opts)
	}
	source := Derived(input).WithEqual(opts.equal)
	r := &ResourceValue[T]{state: State(AsyncState[T]{Status: Pending}), reload: State(uint64(0))}
	OnCleanup(func() { r.disposed = true })
	var requestID uint64
	var runner *reactive.Computation
	runner, _ = reactive.EffectWith(func() {
		value := source.Get()
		r.reload.Get()
		requestID++
		id := requestID
		ctx, cancel := context.WithCancel(context.Background())
		OnCleanup(cancel)
		r.state.Set(AsyncState[T]{RequestID: id, Status: Pending})
		Effect(func() Cleanup {
			go func() {
				value, err := load(ctx, value)
				loopPost(owner)(func() {
					if r.disposed || runner.Disposed() {
						return
					}
					// A model write can precede this posted result without a frame flush.
					// Settle its input watcher first so that stale results cannot commit.
					reactive.Refresh(runner)
					if r.disposed || id != requestID || ctx.Err() != nil {
						return
					}
					state := AsyncState[T]{RequestID: id, Status: Ready, Value: value}
					if err != nil {
						state.Status, state.Err = Failed, err
						state.Value = *new(T)
					}
					r.state.Set(state)
				})
			}()
			return nil
		})
	})
	return r
}

func (r *ResourceValue[T]) Get() AsyncState[T] { return r.state.Get() }

// Reload starts a new request with the current input at the next update.
func (r *ResourceValue[T]) Reload() {
	reactive.CheckUIThread("Resource.Reload")
	if !r.disposed {
		r.reload.Update(func(n uint64) uint64 { return n + 1 })
	}
}

// AwaitWidget renders one branch of a resource without owning the resource.
type AwaitWidget[T any] struct {
	comp    *ComponentWidget
	mounted bool
	pending func() Widget
	then    func(Readable[T]) Widget
	caught  func(Readable[error]) Widget
}

// Await accepts any asynchronous snapshot source, including a State in tests.
// Omitted branches render nothing. Finish configuration before mount.
func Await[T any](source Readable[AsyncState[T]]) *AwaitWidget[T] {
	w := &AwaitWidget[T]{}
	w.comp = Component(func() Widget {
		w.mounted = true
		type branch struct {
			request uint64
			status  AsyncStatus
		}
		selected := Derived(func() branch { s := source.Get(); return branch{s.RequestID, s.Status} })
		return Key(selected, func(b branch) Widget {
			switch b.status {
			case Pending:
				if w.pending != nil {
					return w.pending()
				}
			case Ready:
				if w.then != nil {
					return w.then(Derived(func() T { return source.Get().Value }))
				}
			case Failed:
				if w.caught != nil {
					return w.caught(Derived(func() error { return source.Get().Err }))
				}
			default:
				panic("ggui: invalid Await status")
			}
			return nil
		})
	})
	return w
}
func (w *AwaitWidget[T]) checkConfig() {
	if w.mounted {
		panic("ggui: Await configured after mount")
	}
}
func (w *AwaitWidget[T]) Pending(build func() Widget) *AwaitWidget[T] {
	w.checkConfig()
	w.pending = build
	return w
}
func (w *AwaitWidget[T]) Then(build func(Readable[T]) Widget) *AwaitWidget[T] {
	w.checkConfig()
	w.then = build
	return w
}
func (w *AwaitWidget[T]) Catch(build func(Readable[error]) Widget) *AwaitWidget[T] {
	w.checkConfig()
	w.caught = build
	return w
}
func (w *AwaitWidget[T]) Layout(c Constraints, env Env) Size { return w.comp.Layout(c, env) }
func (w *AwaitWidget[T]) Paint(dst *Canvas, r Rect)          { w.comp.Paint(dst, r) }

// Vacant implements Vacant: the state it is in builds nothing.
func (w *AwaitWidget[T]) Vacant() bool { return blockChild(w.comp) == nil }

// Async runs work on a goroutine and hands what it returns to done on the
// UI thread, before the next frame of the window or probe that called it.
// It is the shape of every slow call a handler makes: a file dialog, a
// request, a query. done may write signals; work must not. If the window
// closes first, done never runs.
//
//	ui.Button("Open…", func() {
//		host := ggui.UseHost()
//		ggui.Async(func() (string, error) {
//			return host.Dialogs().OpenFile(runtime.FileDialog{})
//		}, func(path string, err error) {
//			if err == nil {
//				m.Path.Set(path)
//			}
//		})
//	})
//
// Resource is the form for a value that follows a signal and is cancelled
// when it changes; Async is for a one-off action.
func Async[T any](work func() (T, error), done func(T, error)) {
	post := UseHost().Post
	go func() {
		v, err := work()
		post(func() { done(v, err) })
	}()
}

// Debounced follows r, but late: it takes r's value at once the first time,
// and afterwards each new value only once r has held it unchanged for d. A
// search field typed into runs one search when the typing pauses rather
// than one per keystroke. Together with Resource, which cancels the request
// for a query that changed, it is search-as-you-type:
//
//	q := ggui.Debounced(query, 250*time.Millisecond)
//	res := ggui.Resource(q.Get, search)
//	searching := ggui.Derived(func() bool {
//		return q.Pending() || res.Get().Status == ggui.Pending
//	})
//
// The value keeps State's equality: when the typing settles on the text it
// started from, as "a", "ab", "a" does, the value does not change and no
// second search runs for text already searched. That is why an indicator
// shown from the first keystroke until the results come must read Pending
// as well as the resource, as searching does above: Pending is true from
// r's change until d has passed, and goes false then whether or not the
// value changed. A search that does start makes the resource pending in
// the same frame, before anything is painted, so the indicator does not
// flicker off between the two.
//
// d is measured by the clock ClockTime reads, so a test steps it with
// Probe.Advance or SetClock; a value comes through in the frame that
// reaches it, or, when no frame does, in work posted for it at that time.
// Nothing is written but on the UI thread. Like Resource it needs an owner
// with a frame loop: create it in App.Setup or a component's setup, and it
// stops with that owner, keeping the last value it took, with nothing
// pending.
func Debounced[T any](r Readable[T], d time.Duration) *DebouncedValue[T] {
	reactive.CheckUIThread("Debounced")
	l := loopOf(reactive.CurrentOwner())
	if l == nil {
		panic("ggui: Debounced requires an app or mounted component owner")
	}
	w := l.world()
	dv := &DebouncedValue[T]{value: State(Untrack(r.Get)), pending: State(false)}
	var (
		next     T
		real     *time.Timer
		disposed bool
	)
	t := &clockTimer{fire: func() {
		dv.value.Set(next)
		dv.pending.Set(false)
	}}
	var arm func(time.Duration)
	arm = func(wait time.Duration) {
		if real != nil {
			real.Stop()
		}
		real = time.AfterFunc(wait, func() {
			l.post(func() {
				if disposed {
					return
				}
				now := w.frame.raw()
				w.fireTimers(now)
				// If the timer woke slightly early (or t.due moved later),
				// reschedule for the remaining wait so an idle window never stalls.
				if slices.Contains(w.timers, t) && now.Before(t.due) {
					arm(t.due.Sub(now))
				}
			})
		})
	}
	first := true
	Observe(func() {
		v := r.Get()
		if first {
			first = false
			return
		}
		if d <= 0 {
			dv.value.Set(v)
			return
		}
		next = v
		dv.pending.Set(true)
		w.schedule(t, w.frame.raw().Add(d))
		// The frame clock moves only while frames run, and a window that
		// shows nothing new runs none: a timer of the process's own posts
		// the check for the time d is up. It writes nothing itself.
		arm(d)
	})
	OnCleanup(func() {
		disposed = true
		if real != nil {
			real.Stop()
		}
		w.unschedule(t)
		dv.pending.Set(false)
	})
	return dv
}

// DebouncedValue is a Readable that follows another late, as Debounced
// makes it, and tells whether a value may still come.
type DebouncedValue[T any] struct {
	value   *StateValue[T]
	pending *StateValue[bool]
}

// Get returns the value the source last held for the whole wait, and
// subscribes the running computation to it.
func (v *DebouncedValue[T]) Get() T { return v.value.Get() }

// GetAny implements the reader Textf and Sprintf unwrap.
func (v *DebouncedValue[T]) GetAny() any { return v.Get() }

// Pending reports whether the source changed and the wait since has not
// ended: true from the change until the wait's end, whether or not the
// value it ends with differs from Get's. Like Get, it subscribes the
// running computation.
func (v *DebouncedValue[T]) Pending() bool { return v.pending.Get() }

// clockTimer is something to do once the frame clock's raw time reaches
// due. It belongs to one world and runs on its UI thread.
type clockTimer struct {
	due  time.Time
	fire func()
}

// schedule has t fire once the raw time reaches due, replacing the time it
// was scheduled for before.
func (w *world) schedule(t *clockTimer, due time.Time) {
	t.due = due
	if !slices.Contains(w.timers, t) {
		w.timers = append(w.timers, t)
	}
}

// unschedule drops t if it has not fired.
func (w *world) unschedule(t *clockTimer) {
	w.timers = slices.DeleteFunc(w.timers, func(o *clockTimer) bool { return o == t })
}

// fireTimers fires, in the order they were scheduled, the timers whose time
// now has reached, and forgets them. Each frame's tick calls it, as does
// the work a timer of the process's posts when its time is up.
func (w *world) fireTimers(now time.Time) {
	var due []*clockTimer
	w.timers = slices.DeleteFunc(w.timers, func(t *clockTimer) bool {
		if now.Before(t.due) {
			return false
		}
		due = append(due, t)
		return true
	})
	for _, t := range due {
		t.fire()
	}
}
