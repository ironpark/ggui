package reactive

import (
	"sync"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/goid"
	"github.com/ironpark/ggui/internal/property"
)

// A scope is what one goroutine is running: the computation its reads
// subscribe, the owner the computations it creates join, the layout
// recorder its reads report to, how deep it is inside Derived and inside
// layout, and the frame loop whose frame it runs or ran last.
//
// These were package variables, which was only right while one goroutine
// ran every frame of the process. Probes run under t.Parallel each run on
// a goroutine of their own, so each goroutine gets its own scope, keyed by
// goid.ID. A scope is touched by its own goroutine alone and needs no lock.
type scope struct {
	listener, owner *Computation
	measuring       func(src LayoutSource, version uint64)
	derivedDepth    int
	layoutDepth     int
	layoutGen       uint64 // see RequestLayout
	stateGen        uint64 // see StateGen
	loop            any

	// base is the runtime what this goroutine creates with no owner
	// belongs to; see Base.
	base *Runtime

	hit *scopeHit
}

type scopeHit struct {
	id int64
	s  *scope
}

// scopes holds a scope for every goroutine that has entered one. They are
// not dropped when the goroutine ends: a base runtime can outlive it, as
// the widget a test built before its probe outlives neither.
var scopes sync.Map // int64 -> *scope

// lastScope is the scope looked up last, which in an app, where one
// goroutine runs every frame, is the scope of nearly every lookup.
var lastScope atomic.Pointer[scopeHit]

// current returns the running goroutine's scope, making it on first use. A
// goroutine that only reads makes one too, so that its next lookup is the
// cached one rather than a miss in scopes.
func current() *scope {
	id := goid.ID()
	if h := lastScope.Load(); h != nil && h.id == id {
		return h.s
	}
	if v, ok := scopes.Load(id); ok {
		s := v.(*scope)
		lastScope.Store(s.hit)
		return s
	}
	s := &scope{}
	s.hit = &scopeHit{id: id, s: s}
	scopes.Store(id, s)
	lastScope.Store(s.hit)
	return s
}

// Base returns the running goroutine's base runtime: the one a computation
// created on it with no owner belongs to, such as the bindings of a widget a
// test builds before its probe. A runtime made on the goroutine flushes it
// along with its own; see Runtime.Related.
func Base() *Runtime {
	s := current()
	if s.base == nil {
		s.base = &Runtime{}
	}
	return s.base
}

// enter makes listener and owner the running goroutine's and returns what
// puts the previous ones back.
func enter(listener, owner *Computation) (s *scope, prevListener, prevOwner *Computation) {
	s = current()
	prevListener, prevOwner = s.listener, s.owner
	s.listener, s.owner = listener, owner
	return s, prevListener, prevOwner
}

// Measure runs fn with record installed as the layout recorder, restoring
// whatever was installed before, so that nested layouts each see their own.
func Measure(record func(src LayoutSource, version uint64), fn func()) {
	s := current()
	previous := s.measuring
	s.measuring = record
	defer func() { s.measuring = previous }()
	fn()
}

// Recording reports whether a layout recorder is installed.
func Recording() bool { return current().measuring != nil }

// Record reports a read of src at version to the running layout, if any.
func Record(src LayoutSource, version uint64) {
	if s := current(); s.measuring != nil {
		s.measuring(src, version)
	}
}

// SetRunningLoop records loop as the frame loop the running goroutine runs
// a frame of, and returns the one recorded before. It stays recorded after
// the frame, so that a test reading Now between frames reads its probe's
// clock, until ClearRunningLoop or another loop replaces it.
func SetRunningLoop(loop any) (prev any) {
	s := current()
	prev, s.loop = s.loop, loop
	return prev
}

// RunningLoop returns the frame loop the running goroutine last ran a frame
// of, or nil.
func RunningLoop() any { return current().loop }

// ClearRunningLoop forgets loop if it is the running goroutine's, as a loop
// that closes does.
func ClearRunningLoop(loop any) {
	if s := current(); s.loop == loop {
		s.loop = nil
	}
}

// leaveLayout undoes EnterHook. It is a function of its own rather than a
// closure over the scope, so that entering layout allocates nothing.
func leaveLayout() { current().layoutDepth-- }

// Layout depth is per goroutine as well: a property written while one
// goroutine lays out must not keep another's writes from scheduling a
// frame.
func init() {
	property.EnterHook = func() func() {
		current().layoutDepth++
		return leaveLayout
	}
	property.InLayout = func() bool { return current().layoutDepth > 0 }
}
