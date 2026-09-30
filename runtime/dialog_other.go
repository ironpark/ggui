//go:build (!darwin && !windows) || ios

package runtime

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// showDialog runs the zenity or kdialog program, whichever is on PATH.
func showDialog(k dialogKind, d FileDialog, _ uintptr) ([]string, error) { return execDialog(k, d) }

// execDialog runs the first dialog program found and returns the paths
// chosen, or ErrUnsupported where neither program is on PATH.
func execDialog(k dialogKind, d FileDialog) ([]string, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return runDialog(path, zenityArgs(k, d))
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return runDialog(path, kdialogArgs(k, d))
	}
	return nil, ErrUnsupported
}

// runDialog executes the program and splits its output into paths.
func runDialog(path string, args []string) ([]string, error) {
	cmd := exec.Command(path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return nil, ErrCanceled
		}
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return all(paths, nil)
}
