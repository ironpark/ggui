//go:build !windows

package shared

import "os"

func open(name string) (*os.File, error) { return os.Open(name) }
