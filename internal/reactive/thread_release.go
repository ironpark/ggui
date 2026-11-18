//go:build !ggui_debug

package reactive

// The UI-goroutine check costs nothing unless the ggui_debug build tag is
// set; see thread_debug.go for what it does and why. Its fields are empty
// here, and come first in the structs holding them, as an empty field that
// ends a struct is padded to a word.

//lint:ignore U1000 only a ggui_debug build marks a runtime
type runtimeThread struct{}

//lint:ignore U1000 only a ggui_debug build remembers a signal's runtime
type valueThread struct{}

func madeHere() valueThread { return valueThread{} }

func (*valueThread) checkThread(string) {}

func MarkUIThread(*Runtime) {}

func UnmarkUIThread(*Runtime) {}

func CheckUIThread(string) {}
