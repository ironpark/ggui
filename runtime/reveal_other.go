//go:build !windows && !js

package runtime

import "os/exec"

// explorerSelect is only reached on Windows; it is here so that Reveal's
// switch builds everywhere.
func explorerSelect(string) *exec.Cmd { panic("unreachable") }
