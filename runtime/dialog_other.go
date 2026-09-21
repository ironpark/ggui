//go:build (!darwin && !windows) || ios

package runtime

// showDialog runs the zenity or kdialog program, whichever is on PATH.
func showDialog(k dialogKind, d FileDialog) ([]string, error) { return execDialog(k, d) }
