package ggui

import (
	"context"

	"github.com/ironpark/ggui/internal/reactive"
)

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
func (w *AwaitWidget[T]) absent() bool                       { return blockChild(w.comp) == nil }
