//go:build !windows

package runtime

// The file dialog of a desktop with no toolkit to ask: zenity on GTK,
// kdialog on KDE, whichever is on PATH. Both print the chosen paths on
// standard output and exit with one when the user cancels. The command
// lines are built here, apart from the code in dialog_other.go that runs
// them, so they are tested on macOS as well.

import (
	"path/filepath"
	"strings"
)

// zenityArgs is the command line for zenity --file-selection.
func zenityArgs(k dialogKind, d FileDialog) []string {
	args := []string{"--file-selection"}
	if d.Title != "" {
		args = append(args, "--title="+d.Title)
	}
	switch k {
	case kindOpenMultiple:
		args = append(args, "--multiple", "--separator=\n")
	case kindFolder:
		args = append(args, "--directory")
	case kindSave:
		args = append(args, "--save", "--confirm-overwrite")
	}
	if start := startPath(d); start != "" {
		args = append(args, "--filename="+start)
	}
	if k != kindFolder {
		for _, f := range d.Filters {
			args = append(args, "--file-filter="+f.Name+" | "+globs(f, " "))
		}
	}
	return args
}

// kdialogArgs is the command line for kdialog's file dialogs: the verb,
// the start path, the filter where the dialog takes one, then the flags.
func kdialogArgs(k dialogKind, d FileDialog) []string {
	start := startPath(d)
	var filter string
	if k != kindFolder && len(d.Filters) > 0 {
		groups := make([]string, len(d.Filters))
		for i, f := range d.Filters {
			groups[i] = f.Name + " (" + globs(f, " ") + ")"
		}
		filter = strings.Join(groups, "\n")
	}
	var args []string
	switch k {
	case kindOpen, kindOpenMultiple:
		args = []string{"--getopenfilename", start}
	case kindFolder:
		args = []string{"--getexistingdirectory", start}
	case kindSave:
		args = []string{"--getsavefilename", start}
	}
	if filter != "" {
		args = append(args, filter)
	}
	if k == kindOpenMultiple {
		args = append(args, "--multiple", "--separate-output")
	}
	if d.Title != "" {
		args = append(args, "--title", d.Title)
	}
	return args
}

// startPath is where a dialog begins: the directory, the proposed file
// inside it, or nothing. A trailing separator is what tells both programs
// that a bare directory is a place rather than a name.
func startPath(d FileDialog) string {
	switch {
	case d.Directory != "" && d.FileName != "":
		return filepath.Join(d.Directory, d.FileName)
	case d.Directory != "":
		return strings.TrimRight(d.Directory, string(filepath.Separator)) + string(filepath.Separator)
	case d.FileName != "":
		return d.FileName
	}
	return ""
}
