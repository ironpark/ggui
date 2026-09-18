//go:build !ggui_debug

package ggui

// Recording where every effect was created costs a stack walk per effect,
// so only the ggui_debug build does it; see origin_debug.go and ErrCycle.

func effectOrigin() string { return "" }
