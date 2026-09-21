package runtime

// The file dialog of a desktop with no toolkit to ask: zenity on GTK,
// kdialog on KDE, whichever is on PATH. Both print the chosen paths on
// standard output and exit with one when the user cancels. The command
// lines are built apart from the code that runs them, so they are tested
// on every platform.

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

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

// globs is a filter's patterns joined by sep: "*.png *.jpg", or "*" for a
// filter that admits everything.
func globs(f FileFilter, sep string) string {
	if len(f.Extensions) == 0 {
		return "*"
	}
	patterns := make([]string, len(f.Extensions))
	for i, ext := range f.Extensions {
		patterns[i] = "*." + ext
	}
	return strings.Join(patterns, sep)
}

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
