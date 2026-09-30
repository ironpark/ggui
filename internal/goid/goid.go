// Package goid tells goroutines apart. ggui's reactive core keeps what is
// running -- the computation reads subscribe to, the owner new effects join,
// the frame loop Now reads -- per goroutine, so that probes on goroutines of
// their own, as t.Parallel runs them, never see each other's. Go offers no
// goroutine-local storage, so the core keys that state by ID.
//
// ID is read from the runtime's own record of the goroutine, at an offset
// found at start-up by matching it against the id runtime.Stack prints and
// confirmed on goroutines of its own. Where that cannot be done, on an
// architecture without the few lines of assembly that find the record or a
// runtime whose layout no longer matches, ID parses runtime.Stack instead:
// slower, and never wrong.
package goid

import (
	"bytes"
	"runtime"
	"strconv"
	"unsafe"
)

// ID returns the running goroutine's id, which no other goroutine has had
// or will have while the process runs.
func ID() int64 {
	if offset >= 0 {
		return *(*int64)(unsafe.Add(getg(), offset))
	}
	return slowID()
}

// Fast reports whether ID reads the runtime's record rather than parsing a
// stack trace.
func Fast() bool { return offset >= 0 }

// offset is where the id sits in the runtime's record of a goroutine, or -1
// when ID has to parse the stack.
var offset = find()

// scanLimit bounds the search to the start of the record, which the id has
// always been well inside; nothing past the record is read.
const scanLimit = 256

// find returns the offset holding the id on this goroutine and on two more,
// each checked against the id runtime.Stack reports, or -1.
func find() uintptr {
	if !haveGetg {
		return ^uintptr(0)
	}
	candidates := matches(nil)
	for range 2 {
		done := make(chan []uintptr)
		go func() { done <- matches(candidates) }()
		candidates = <-done
	}
	if len(candidates) == 0 {
		return ^uintptr(0)
	}
	return candidates[0]
}

// matches returns the offsets in within the running goroutine's record that
// hold its id, from every word of the record when within is nil.
func matches(within []uintptr) []uintptr {
	g, id := getg(), slowID()
	if g == nil || id <= 0 {
		return nil
	}
	const word = unsafe.Sizeof(uintptr(0))
	if within == nil {
		for off := uintptr(0); off+8 <= scanLimit; off += word {
			within = append(within, off)
		}
	}
	var out []uintptr
	for _, off := range within {
		if *(*int64)(unsafe.Add(g, off)) == id {
			out = append(out, off)
		}
	}
	return out
}

// slowID parses the running goroutine's id from the header runtime.Stack
// writes: "goroutine 17 [running]:".
func slowID() int64 {
	var buf [64]byte
	b := buf[:runtime.Stack(buf[:], false)]
	b = bytes.TrimPrefix(b, []byte("goroutine "))
	if i := bytes.IndexByte(b, ' '); i >= 0 {
		if n, err := strconv.ParseInt(string(b[:i]), 10, 64); err == nil {
			return n
		}
	}
	return 0
}
