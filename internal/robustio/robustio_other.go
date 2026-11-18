//go:build !windows

package robustio

// Elsewhere an open file is renamed over and removed like any other.
func transient(error) bool { return false }
