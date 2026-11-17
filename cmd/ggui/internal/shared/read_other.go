//go:build !windows

package shared

import "os"

// Elsewhere deleting or renaming over an open file is never refused.
func readFile(name string) ([]byte, error) { return os.ReadFile(name) }
