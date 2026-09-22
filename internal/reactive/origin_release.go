//go:build !ggui_debug

package reactive

// Recording where every effect was created costs a stack walk per effect,
// so only the ggui_debug build does it; see origin_debug.go and ErrCycle.

// Debug reports whether this build records effect origins.
const Debug = false

func effectOrigin() string { return "" }
