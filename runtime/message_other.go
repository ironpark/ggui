//go:build (!darwin && !windows && !js) || ios

package runtime

// showMessage runs the zenity or kdialog program, whichever is on PATH.
func showMessage(m Message, _ uintptr) (int, error) { return execMessage(m) }
