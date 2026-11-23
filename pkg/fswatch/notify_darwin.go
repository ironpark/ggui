package fswatch

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// On macOS the source is FSEvents, called through purego so that a
// program using the package still builds without cgo. A stream watches
// the few roots that hold the watched directories, recursively, and names
// each file that changed; kqueue, the alternative, needs a descriptor for
// each file.
//
// FSEvents calls back on a thread of its dispatch queue, which Go has
// never seen. purego's callback enters Go there as cgo's would: with cgo
// through runtime/cgo, and without it through purego's fakecgo, which
// supplies the same hooks. The callback copies the paths, queues them and
// returns; it never blocks.

const (
	cfStringEncodingUTF8 = 0x08000100
	eventIDSinceNow      = ^uint64(0)

	// FSEventStreamCreateFlags
	createNoDefer    = 0x02 // the first event after a quiet spell comes at once
	createWatchRoot  = 0x04 // say when a root, or a directory above it, moves
	createFileEvents = 0x10 // name the file, not just its directory

	// FSEventStreamEventFlags that mean events were lost or coalesced
	// into a directory, that a root moved, or that a volume was mounted
	// or unmounted below one: anything below may have changed.
	eventMustScanSubDirs = 0x01
	eventUserDropped     = 0x02
	eventKernelDropped   = 0x04
	eventRootChanged     = 0x20
	eventMount           = 0x40
	eventUnmount         = 0x80
	eventRescan          = eventMustScanSubDirs | eventUserDropped | eventKernelDropped |
		eventRootChanged | eventMount | eventUnmount

	// latency is how long FSEvents gathers events before calling back,
	// after the first, which NoDefer delivers at once.
	latency = 0.01
)

// fsContext is FSEventStreamContext. info carries the stream's key in
// streams; the stream copies the struct and calls no retain or release.
type fsContext struct {
	_    int // version, 0
	info uintptr
	_    [3]uintptr // retain, release, copyDescription: none
}

var fse struct {
	once sync.Once
	err  error

	create         func(alloc, callback uintptr, ctx *fsContext, paths uintptr, since uint64, latency float64, flags uint32) uintptr
	setQueue       func(stream, queue uintptr)
	start          func(stream uintptr) bool
	stop           func(stream uintptr)
	invalidate     func(stream uintptr)
	release        func(stream uintptr)
	cfString       func(alloc uintptr, s string, encoding uint32) uintptr
	cfArray        func(alloc uintptr, values *uintptr, n int, callbacks uintptr) uintptr
	cfRelease      func(cf uintptr)
	arrayCallbacks uintptr // &kCFTypeArrayCallBacks

	queue    uintptr // the serial dispatch queue every stream calls back on
	callback uintptr

	mu      sync.Mutex
	streams map[uintptr]*fsevents
	next    uintptr
}

func loadFSEvents() error {
	fse.once.Do(func() {
		cs, err := purego.Dlopen("/System/Library/Frameworks/CoreServices.framework/CoreServices", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			fse.err = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			fse.err = err
			return
		}
		sys, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			fse.err = err
			return
		}
		// RegisterLibFunc panics on a missing symbol; check first.
		for _, s := range []struct {
			lib  uintptr
			name string
		}{
			{cs, "FSEventStreamCreate"}, {cs, "FSEventStreamSetDispatchQueue"}, {cs, "FSEventStreamStart"},
			{cs, "FSEventStreamStop"}, {cs, "FSEventStreamInvalidate"}, {cs, "FSEventStreamRelease"},
			{cf, "CFStringCreateWithCString"}, {cf, "CFArrayCreate"}, {cf, "CFRelease"},
			{cf, "kCFTypeArrayCallBacks"}, {sys, "dispatch_queue_create"},
		} {
			if _, err := purego.Dlsym(s.lib, s.name); err != nil {
				fse.err = err
				return
			}
		}
		purego.RegisterLibFunc(&fse.create, cs, "FSEventStreamCreate")
		purego.RegisterLibFunc(&fse.setQueue, cs, "FSEventStreamSetDispatchQueue")
		purego.RegisterLibFunc(&fse.start, cs, "FSEventStreamStart")
		purego.RegisterLibFunc(&fse.stop, cs, "FSEventStreamStop")
		purego.RegisterLibFunc(&fse.invalidate, cs, "FSEventStreamInvalidate")
		purego.RegisterLibFunc(&fse.release, cs, "FSEventStreamRelease")
		purego.RegisterLibFunc(&fse.cfString, cf, "CFStringCreateWithCString")
		purego.RegisterLibFunc(&fse.cfArray, cf, "CFArrayCreate")
		purego.RegisterLibFunc(&fse.cfRelease, cf, "CFRelease")
		fse.arrayCallbacks, _ = purego.Dlsym(cf, "kCFTypeArrayCallBacks")
		var queueCreate func(label string, attr uintptr) uintptr
		purego.RegisterLibFunc(&queueCreate, sys, "dispatch_queue_create")
		// A serial queue (attr NULL): callbacks never overlap.
		fse.queue = queueCreate("fswatch", 0)
		if fse.queue == 0 {
			fse.err = fmt.Errorf("dispatch_queue_create failed")
			return
		}
		// One callback for every stream: purego never frees one.
		fse.callback = purego.NewCallback(fsCallback)
		fse.streams = map[uintptr]*fsevents{}
	})
	return fse.err
}

// fsCallback is FSEventStreamCallback, on the dispatch queue's thread.
func fsCallback(_ uintptr, info uintptr, n uintptr, paths, flags, _ unsafe.Pointer) {
	fse.mu.Lock()
	s := fse.streams[info]
	fse.mu.Unlock()
	if s == nil || n == 0 {
		return
	}
	ps := unsafe.Slice((**byte)(paths), n)
	fl := unsafe.Slice((*uint32)(flags), n)
	for i, p := range ps {
		if fl[i]&eventRescan != 0 {
			s.lost()
			continue
		}
		s.push(s.given(cString(p)))
	}
}

// cString copies a NUL-terminated C string.
func cString(p *byte) string {
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// fsevents is a source on one FSEvents stream, replaced when the roots
// change.
type fsevents struct {
	queue
	key uintptr

	mu     sync.Mutex // held while the stream is replaced
	stream uintptr
	roots  []string

	// The callback reads real under its own lock, never under mu: a
	// stream may wait for its callback to return while it stops.
	rmu sync.Mutex
	// real maps a root as FSEvents names it, with symbolic links
	// resolved (/private/var for /var), to the root as given.
	real [][2]string
}

func newSource(bool, func(string) bool) (source, error) {
	if err := loadFSEvents(); err != nil {
		return nil, fmt.Errorf("FSEvents: %w", err)
	}
	s := &fsevents{queue: newQueue()}
	fse.mu.Lock()
	fse.next++
	s.key = fse.next
	fse.streams[s.key] = s
	fse.mu.Unlock()
	return s, nil
}

// given rewrites path, as FSEvents reports it, to be below a root as
// given.
func (s *fsevents) given(path string) string {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	for _, r := range s.real {
		if within(r[0], path) {
			return filepath.Join(r[1], path[len(r[0]):])
		}
	}
	return path
}

func (s *fsevents) watch(dirs []string) error {
	rs := roots(dirs)
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.Equal(rs, s.roots) && s.stream != 0 {
		return nil
	}
	var real [][2]string
	strs := make([]uintptr, 0, len(rs))
	defer func() {
		for _, c := range strs {
			fse.cfRelease(c)
		}
	}()
	for _, r := range rs {
		if rp, err := filepath.EvalSymlinks(r); err == nil && rp != r {
			real = append(real, [2]string{rp, r})
		}
		if strings.IndexByte(r, 0) >= 0 {
			continue
		}
		c := fse.cfString(0, r, cfStringEncodingUTF8)
		if c == 0 {
			return fmt.Errorf("FSEvents: cannot make a string of %s", r)
		}
		strs = append(strs, c)
	}
	slices.SortFunc(real, func(a, b [2]string) int {
		if d := len(b[0]) - len(a[0]); d != 0 {
			return d
		}
		return strings.Compare(a[0], b[0])
	})
	s.rmu.Lock()
	prevReal := s.real
	s.real = real
	s.rmu.Unlock()
	restoreReal := func() {
		s.rmu.Lock()
		s.real = prevReal
		s.rmu.Unlock()
	}
	var stream uintptr
	if len(strs) > 0 {
		arr := fse.cfArray(0, &strs[0], len(strs), fse.arrayCallbacks)
		if arr == 0 {
			restoreReal()
			return fmt.Errorf("FSEvents: cannot make an array")
		}
		ctx := fsContext{info: s.key}
		stream = fse.create(0, fse.callback, &ctx, arr, eventIDSinceNow, latency, createNoDefer|createWatchRoot|createFileEvents)
		fse.cfRelease(arr)
		if stream == 0 {
			restoreReal()
			return fmt.Errorf("FSEvents: FSEventStreamCreate failed")
		}
		fse.setQueue(stream, fse.queue)
		if !fse.start(stream) {
			fse.invalidate(stream)
			fse.release(stream)
			restoreReal()
			return fmt.Errorf("FSEvents: FSEventStreamStart failed")
		}
	}
	// The old stream stops once the new one runs, so nothing between
	// them is missed.
	if s.stream != 0 {
		stopStream(s.stream)
	}
	s.stream, s.roots = stream, rs
	return nil
}

func stopStream(stream uintptr) {
	fse.stop(stream)
	fse.invalidate(stream)
	fse.release(stream)
}

func (s *fsevents) close() {
	fse.mu.Lock()
	delete(fse.streams, s.key)
	fse.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stream != 0 {
		stopStream(s.stream)
		s.stream = 0
	}
}
