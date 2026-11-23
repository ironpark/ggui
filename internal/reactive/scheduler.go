package reactive

import (
	"cmp"
	"container/heap"
	"slices"
	"sync"
	"sync/atomic"
)

// A goroutine's layout generation counts the changes on it that can move
// something on screen: every StateValue write and every Invalidate. The
// runtime lays a tree out again only when it has advanced, or the window
// changed size, and paints every frame regardless. Its state generation
// counts the writes alone, so that settle can tell a write that happened
// during a layout and go round again.
//
// Both are kept per goroutine, in its scope: a runtime's signals are
// written on the goroutine that runs its frames, and a write a probe on
// another goroutine makes must neither lay this one out again nor make its
// settle go round. Every generation is drawn from genSeq, so no two are
// equal.
var genSeq atomic.Uint64

// RequestLayout asks the runtime to lay out the trees whose frames run on
// this goroutine again next frame. It belongs on the UI goroutine, as a
// signal write does.
func RequestLayout() {
	sc := current()
	sc.layoutGen = genSeq.Add(1)
	if sc.contained == 0 {
		sc.freeGen = sc.layoutGen
		if sc.onFree != nil {
			sc.onFree()
		}
	}
}

// Contained runs fn, whose writes and layout requests change only what a
// region of the window shows, as a scroll moving its content does: FreeGen
// does not count them, so a frame can paint that region alone. What
// reacts to them later, outside fn, counts as ever.
func Contained(fn func()) {
	sc := current()
	sc.contained++
	defer func() { sc.contained-- }()
	fn()
}

// FreeGen changes with every StateValue write and layout request made
// outside Contained, on this goroutine.
func FreeGen() uint64 { return current().freeGen }

// InRegion runs fn with the computations it makes, and those they own,
// in the region flag stands for: while *flag is true, what they do when they
// run again is Contained, as what the region's layout does is. A scroll
// lays its content out in one, so the rows that mount as it moves and react
// as they are measured change only its viewport.
func InRegion(flag *bool, fn func()) {
	sc := current()
	prev := sc.region
	sc.region = flag
	defer func() { sc.region = prev }()
	fn()
}

// WatchFree has fn called, nil for nothing, at every write and layout
// request that moves FreeGen on this goroutine, on the writer's stack: what
// keeps a frame from painting a region alone can then be told.
func WatchFree(fn func()) { current().onFree = fn }

// markDirty records that a signal this Computation read has changed, and carries
// check on to the readers of the memo it computes.
func markDirty(e *Computation) {
	e.rt.effects.dirtyGen++
	if e.user {
		e.rt.effects.userPending = true
	}
	if e.state == stateDirty {
		return
	}
	e.mark(stateDirty)
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// mark moves e out of the clean state and queues it for the next flush:
// every such move goes through here, so a flush that runs only the queue
// misses none.
func (e *Computation) mark(state uint8) {
	e.state = state
	e.rt.effects.enqueue(e)
}

// markCheck records that an input of this Computation may have changed. It stops
// at an effect that is already dirty or already checked, so the walk is
// linear and a cycle terminates.
func markCheck(e *Computation) {
	if e.user {
		e.rt.effects.userPending = true
	}
	if e.state != stateClean {
		return
	}
	// A reader in another runtime learns of the change only through this
	// mark, so its flush must not take itself for settled.
	e.rt.effects.dirtyGen++
	e.mark(stateCheck)
	if e.cell != nil {
		e.cell.markSubsCheck()
	}
}

// Refresh settles e now: a checked Computation first settles the memos it read,
// which makes it dirty if one of them actually changed, and a dirty Computation
// re-runs. It reports whether the Computation ran, and leaves e clean either way.
func Refresh(e *Computation) (ran bool) {
	if e.disposed || e.running || e.state == stateClean {
		return false
	}
	if e.state == stateCheck {
		e.settleSources()
		if e.disposed || e.state == stateClean {
			return false
		}
	}
	if e.disposed {
		return false
	}
	if e.state == stateDirty {
		return runEffect(e)
	}
	e.state = stateClean
	return false
}

// settleSources refreshes the producers e read, stopping once e is dirty,
// settled or disposed.
func (e *Computation) settleSources() {
	for _, src := range e.sources {
		p := src.producer()
		if p == nil {
			continue
		}
		if p.running && e.cell != nil {
			panic("ggui: cyclic Derived")
		}
		Refresh(p)
		if e.disposed || e.state == stateClean || e.state == stateDirty {
			return
		}
	}
}

func runEffect(e *Computation) bool {
	e.reset()
	if e.disposed {
		return false
	}
	e.seq = 0

	sc, prevListener, prevOwner := enter(e, e)
	building, prevMeasuring := sc.building, sc.measuring
	sc.building, sc.measuring = false, nil
	defer func() {
		sc.listener, sc.owner, sc.building, sc.measuring = prevListener, prevOwner, building, prevMeasuring
	}()

	e.state, e.running = stateClean, true
	completed := false
	if e.region != nil && *e.region {
		sc.contained++
		defer func() { sc.contained-- }()
	}
	defer func() {
		e.running = false
		if !completed && !e.disposed {
			e.mark(stateDirty)
		}
	}()
	e.fn()
	completed = true
	return true
}

type effectSet struct {
	mu        sync.Mutex
	count     int
	nextOrder uint64

	// The computations marked since they last ran, in no order: what a
	// flush runs, so that a frame that changed one row of ten thousand
	// looks at that row and not the ten thousand. Bindings and user
	// effects wait apart, as they run in different phases. Confined to
	// the UI thread, like the states they mirror.
	queue, userQueue []*Computation

	// Like Computation.state, these are confined to the UI thread. A signal
	// write advances dirtyGen; only a quiet flush records settledGen.
	// Internal bindings run immediately; new user effects explicitly mark work.
	dirtyGen, settledGen uint64
	userPending          bool

	// The effects the last pass ran, kept for ErrCycle. The slice is
	// reused, so a settled frame allocates nothing for it.
	lastPass []*Computation

	// passBuf is runPass's sorted copy of the queue, kept from pass to
	// pass and taken while one runs, so that a flush nested in a binding
	// makes its own.
	passBuf []*Computation
}

// Runtime is one reactive graph's scheduler: the computations it owns and
// the bookkeeping that tells a flush whether any of them is stale. Signals
// belong to no runtime; a write marks each reader in the reader's own.
//
// A computation belongs to its owner's runtime, and one created with no
// owner to its goroutine's Base. A frame loop builds its tree under
// Runtime.Root, so what the tree creates is flushed by that loop alone, and
// a loop that is never closed leaves nothing behind in anyone else's frames.
//
// A runtime runs on one goroutine at a time. Two runtimes may run on two
// goroutines at once, as probes under t.Parallel do, as long as nothing one
// of them flushes is written from the other: a signal written by one
// goroutine and read by another's computations is a race, runtimes or not.
type Runtime struct {
	// The goroutine running the runtime's frames, in a ggui_debug build;
	// empty otherwise. See thread_debug.go.
	//lint:ignore U1000 only a ggui_debug build checks the goroutine
	runtimeThread
	effects effectSet

	// base is the Base of the goroutine that made this runtime, which holds
	// what was built there before the runtime's tree: a widget a test built
	// before its probe. Nil for a Base itself.
	base *Runtime

	// Host is whatever the package driving this runtime keeps beside it.
	// The root package keeps its animations and frame clock here.
	Host any
}

// NewRuntime returns an empty Runtime that flushes the running goroutine's
// Base along with its own computations.
func NewRuntime() *Runtime { return &Runtime{base: Base()} }

// Related returns the runtimes a flush of rt covers: rt itself, the Base it
// was made beside, and the running goroutine's Base, which holds what a
// handler running in rt's frame built with no owner. The array is padded
// with nil, so that a frame asking allocates nothing.
func (rt *Runtime) Related() (out [3]*Runtime) {
	out[0] = rt
	n := 1
	for _, x := range [...]*Runtime{rt.base, Base()} {
		if x != nil && x != out[0] && x != out[1] {
			out[n] = x
			n++
		}
	}
	return out
}

// Parent is the Base rt was made beside, or nil for a Base.
func (rt *Runtime) Parent() *Runtime { return rt.base }

func (s *effectSet) add(e *Computation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextOrder++
	e.order = s.nextOrder
	e.registered = true
	s.count++
}

func (s *effectSet) remove(e *Computation) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !e.registered {
		return
	}
	e.registered = false
	s.count--
}

// unsettled returns the effects the last flush pass ran, which when the
// passes ran out are the ones the cycle turns, with how many effects there
// were in all. Reading the states afterwards would miss half of them: an
// Computation in a cycle is clean the moment after it runs and dirty the moment
// after its partner does.
func (s *effectSet) unsettled() (stuck []*Computation, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPass, s.count
}

func (s *effectSet) len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

// MaxFlushPasses bounds how far a change propagates through derived values in
// one frame. Chains settle in a pass or two; the cap only stops a cycle.
const MaxFlushPasses = 16

// settled reports whether every write this set was told of has been
// flushed.
func (s *effectSet) settled() bool { return s.dirtyGen == s.settledGen }

// enqueue puts a computation that just stopped being clean in line for
// the flush that runs its kind. A memo is not queued: it runs when read.
func (s *effectSet) enqueue(e *Computation) {
	if e.queued || e.cell != nil || e.disposed {
		return
	}
	e.queued = true
	if e.user {
		s.userPending = true
		s.userQueue = append(s.userQueue, e)
	} else {
		s.queue = append(s.queue, e)
	}
}

// orderHeap is a min-heap of computations by creation order.
type orderHeap []*Computation

func (h orderHeap) Len() int           { return len(h) }
func (h orderHeap) Less(i, j int) bool { return h[i].order < h[j].order }
func (h orderHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *orderHeap) Push(x any)        { *h = append(*h, x.(*Computation)) }
func (h *orderHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	old[len(old)-1] = nil
	*h = old[:len(old)-1]
	return e
}

// runPass runs the queued bindings oldest first, as a scan of every
// computation in creation order would, and reports whether any ran. One
// marked while the pass runs joins it if it comes later in that order, and
// waits for the next pass if it comes earlier, as the scan would leave it.
//
// The queue is sorted once and walked in order. Only what is marked while
// the pass runs, usually nothing, goes into a heap, merged with the walk
// by order: heaping the whole queue spent a third of a large flush in
// container/heap's interface calls. The sorted copy is taken from the set
// for the length of the pass, since a binding may flush in the middle of
// one.
func (s *effectSet) runPass() (ran bool) {
	pass := append(s.passBuf[:0], s.queue...)
	s.passBuf = nil
	clear(s.queue)
	s.queue = s.queue[:0]
	slices.SortFunc(pass, func(a, b *Computation) int { return cmp.Compare(a.order, b.order) })
	var marked orderHeap
	var later []*Computation
	var cur *Computation
	i := 0
	defer func() {
		if cur != nil && !cur.disposed && cur.state != stateClean {
			s.enqueue(cur)
		}
		for _, e := range pass[i:] {
			if !e.disposed && e.state != stateClean {
				e.queued = true
				s.queue = append(s.queue, e)
			} else {
				e.queued = false
			}
		}
		for _, e := range marked {
			if !e.disposed && e.state != stateClean {
				e.queued = true
				s.queue = append(s.queue, e)
			} else {
				e.queued = false
			}
		}
		for _, e := range later {
			if !e.disposed && e.state != stateClean {
				e.queued = true
				s.queue = append(s.queue, e)
			} else {
				e.queued = false
			}
		}
		clear(pass)
		s.passBuf = pass[:0]
	}()
	for i < len(pass) || marked.Len() > 0 {
		var e *Computation
		if marked.Len() > 0 && (i == len(pass) || marked[0].order < pass[i].order) {
			e = heap.Pop(&marked).(*Computation)
		} else {
			e = pass[i]
			i++
		}
		e.queued = false
		cur = e
		if Refresh(e) {
			ran = true
			s.lastPass = append(s.lastPass, e)
		}
		cur = nil
		for _, n := range s.queue {
			if n.order > e.order {
				heap.Push(&marked, n)
			} else {
				later = append(later, n)
			}
		}
		clear(s.queue)
		s.queue = s.queue[:0]
	}
	return ran
}

// Flush re-runs every dirty Computation of rt and the runtimes it covers,
// repeating until they are all quiet so that a DerivedValue feeding another
// Computation lands in the same frame. Called once per frame by the
// runtime. It reports false when the effects were still dirty after
// MaxFlushPasses, which only a cycle causes.
func (rt *Runtime) Flush() (settled bool) {
	if rt.Settled() {
		return true
	}
	for range MaxFlushPasses {
		if !rt.flushPass() {
			// Read after the quiet pass: cleanup and nested flushes may
			// have written signals while earlier passes were running.
			for _, x := range rt.Related() {
				if x != nil {
					x.effects.settledGen = x.effects.dirtyGen
				}
			}
			return true
		}
	}
	return false
}

// flushPass runs one pass over the runtimes rt covers, reporting whether
// any Computation ran.
func (rt *Runtime) flushPass() bool {
	ran := false
	for _, x := range rt.Related() {
		if x == nil {
			continue
		}
		s := &x.effects
		clear(s.lastPass)
		s.lastPass = s.lastPass[:0]
		if s.settled() {
			// Every mark bumps dirtyGen, so a set with none since the
			// last flush has nothing to run.
			continue
		}
		if s.runPass() {
			ran = true
		}
	}
	return ran
}

// Settled reports whether every write reaching rt or a runtime it covers
// has been flushed.
func (rt *Runtime) Settled() bool {
	for _, x := range rt.Related() {
		if x != nil && !x.effects.settled() {
			return false
		}
	}
	return true
}

// FlushUsers executes one post-layout pass of the user effects of rt and
// the runtimes it covers that belong to loop, reporting whether any ran.
// Layout is settled again before another pass, so effects never observe a
// partially mounted tree.
func (rt *Runtime) FlushUsers(loop any) bool {
	ran := false
	for _, x := range rt.Related() {
		if x == nil || !x.effects.userPending {
			continue
		}
		if x.effects.flushUsers(loop) {
			ran = true
		}
	}
	return ran
}

func (s *effectSet) flushUsers(loop any) (ran bool) {
	s.userPending = false
	var pending []*Computation
	kept := s.userQueue[:0]
	for _, e := range s.userQueue {
		switch {
		case e.disposed || e.state == stateClean:
			e.queued = false
		case e.loop == nil || e.loop == loop:
			e.queued = false
			pending = append(pending, e)
		default:
			kept = append(kept, e)
			s.userPending = true
		}
	}
	clear(s.userQueue[len(kept):])
	s.userQueue = kept
	slices.SortFunc(pending, func(a, b *Computation) int { return cmp.Compare(a.order, b.order) })
	clear(s.lastPass)
	s.lastPass = s.lastPass[:0]
	i := 0
	defer func() {
		for _, e := range pending[i:] {
			if !e.disposed && e.state != stateClean {
				s.userPending = true
				s.enqueue(e)
			}
		}
		clear(pending)
	}()
	for i < len(pending) {
		e := pending[i]
		if Refresh(e) {
			ran = true
			s.lastPass = append(s.lastPass, e)
		}
		i++
	}
	return ran
}

// Unsettled names the effects the last pass of rt's flush ran, for
// ErrCycle, with how many effects the runtimes it covers hold in all.
func (rt *Runtime) Unsettled() (stuck []*Computation, total int) {
	for _, x := range rt.Related() {
		if x != nil {
			pass, n := x.effects.unsettled()
			stuck, total = append(stuck, pass...), total+n
		}
	}
	return stuck, total
}

// Count is how many effects rt holds, for a test asserting that a
// construction or teardown left none behind.
func (rt *Runtime) Count() int { return rt.effects.len() }

// Flush is Base().Flush, for the running goroutine.
func Flush() bool { return Base().Flush() }

// FlushUsers is Base().FlushUsers, for the running goroutine.
func FlushUsers(loop any) bool { return Base().FlushUsers(loop) }

// StateGen counts StateValue writes, so that a frame can tell whether one
// happened while it was laying out.
func StateGen() uint64 { return current().stateGen }

// LayoutGen changes whenever something that can move a tree laid out on
// this goroutine changes: a StateValue write or a RequestLayout here.
func LayoutGen() uint64 { return current().layoutGen }

// Count is Base().Count, for the running goroutine.
func Count() int { return Base().Count() }
