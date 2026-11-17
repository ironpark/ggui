package reactive

import (
	"fmt"
	"runtime"
	"strings"
)

// writeTraceMax is how many writes TraceWrites keeps, and writeTraceDepth
// how many calls of each one's stack.
const (
	writeTraceMax   = 6
	writeTraceDepth = 14
)

// TraceWrites keeps, until stop is called, where each StateValue write on
// this goroutine is made: the stack that made it, past the store itself.
// It is for a diagnosis on a failing path, a frame whose layout keeps
// writing state, and costs a write a stack walk while on.
func TraceWrites() (stop func() []string) {
	sc := current()
	var writes []string
	outer := sc.writes
	sc.writes = &writes
	return func() []string {
		sc.writes = outer
		return writes
	}
}

func noteWrite(writes *[]string) {
	if len(*writes) >= writeTraceMax {
		return
	}
	pcs := make([]uintptr, writeTraceDepth+4)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	shown := 0
	for shown < writeTraceDepth {
		f, more := frames.Next()
		if !strings.Contains(f.Function, "/internal/reactive.") {
			fmt.Fprintf(&b, "\n      %s\n        %s:%d", f.Function, f.File, f.Line)
			shown++
		}
		if !more {
			break
		}
	}
	*writes = append(*writes, b.String())
}
