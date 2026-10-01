//go:build !windows && !js

package runtime

import "os/exec"

// explorerSelect is only reached on Windows.
func explorerSelect(path string) *exec.Cmd { return exec.Command("explorer", "/select,"+path) }
