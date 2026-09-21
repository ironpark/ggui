//go:build !ggui_debug

package reactive

// Recording where every Computation was created costs a stack walk per Computation,
// so only the ggui_debug build does it; see origin_debug.go and ErrCycle.

func effectOrigin() string { return "" }
