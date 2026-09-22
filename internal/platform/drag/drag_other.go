//go:build !darwin || ios

// Package drag reports files being dragged over the window, before they
// are dropped. Ebitengine reports only the drop, so this listens on the
// platform's own drag session where it can.
package drag

// Install reports whether the platform can follow a drag. It cannot here:
// only the drop itself is reported, so a zone learns of a drag when it
// lands.
func Install() bool { return true }

// Position reports no drag on this platform.
func Position() (x, y float64, over bool) { return 0, 0, false }
