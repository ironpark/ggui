//go:build !ggui_debug

package reactive

// The UI-goroutine check costs nothing unless the ggui_debug build tag is
// set; see thread_debug.go for what it does and why.

func MarkUIThread() {}

func UnmarkUIThread() {}

func CheckUIThread(op string) {}
