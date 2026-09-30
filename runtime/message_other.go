//go:build (!darwin && !windows && !js) || ios

package runtime

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
)

// showMessage runs the zenity or kdialog program, whichever is on PATH.
func showMessage(m Message, _ uintptr) (int, error) { return execMessage(m) }

// execMessage runs the first dialog program found and returns the button
// pressed, or ErrUnsupported where neither program is on PATH.
func execMessage(m Message) (int, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return runMessage(path, zenityMessageArgs(m), m.buttons(), zenityAnswer)
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return runMessage(path, kdialogMessageArgs(m), m.buttons(), kdialogAnswer)
	}
	return 0, ErrUnsupported
}

// runMessage runs the program and turns its exit code and output into the
// index of the button pressed.
func runMessage(path string, args, buttons []string, answer func(code int, out string, buttons []string) (int, error)) (int, error) {
	cmd := exec.Command(path, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	code := 0
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return 0, err
		}
		code = exit.ExitCode()
	}
	return answer(code, strings.TrimSpace(out.String()), buttons)
}
