// Package dialogexec is the file dialog of a desktop with no toolkit to
// ask: zenity on GTK, kdialog on KDE, whichever is on PATH. Both print the
// chosen paths on standard output and exit with one when the user cancels.
// The command lines are built apart from the code that runs them, so they
// are tested on every platform.
package dialogexec

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// Kind is which dialog to show.
type Kind uint8

const (
	Open Kind = iota
	OpenMultiple
	Folder
	Save
)

// Filter is one entry of the type menu; no extensions admits every file.
type Filter struct {
	Name       string
	Extensions []string
}

// Request is what a dialog is asked.
type Request struct {
	Kind      Kind
	Title     string
	Directory string
	FileName  string
	Filters   []Filter
}

// ErrCanceled is the program exiting with one, which is a cancel.
var ErrCanceled = errors.New("canceled")

// ErrNoProgram is neither program being on PATH.
var ErrNoProgram = errors.New("no dialog program found")

// Show runs the first dialog program found and returns the paths chosen.
func Show(r Request) ([]string, error) {
	if path, err := exec.LookPath("zenity"); err == nil {
		return run(path, ZenityArgs(r))
	}
	if path, err := exec.LookPath("kdialog"); err == nil {
		return run(path, KdialogArgs(r))
	}
	return nil, ErrNoProgram
}

// run executes the program and splits its output into paths.
func run(path string, args []string) ([]string, error) {
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
	if len(paths) == 0 {
		return nil, ErrCanceled
	}
	return paths, nil
}

// globs is a filter's patterns joined by sep: "*.png *.jpg", or "*" for a
// filter that admits everything.
func globs(f Filter, sep string) string {
	if len(f.Extensions) == 0 {
		return "*"
	}
	patterns := make([]string, len(f.Extensions))
	for i, ext := range f.Extensions {
		patterns[i] = "*." + ext
	}
	return strings.Join(patterns, sep)
}

// ZenityArgs is the command line for zenity --file-selection.
func ZenityArgs(r Request) []string {
	args := []string{"--file-selection"}
	if r.Title != "" {
		args = append(args, "--title="+r.Title)
	}
	switch r.Kind {
	case OpenMultiple:
		args = append(args, "--multiple", "--separator=\n")
	case Folder:
		args = append(args, "--directory")
	case Save:
		args = append(args, "--save", "--confirm-overwrite")
	}
	if start := startPath(r); start != "" {
		args = append(args, "--filename="+start)
	}
	if r.Kind != Folder {
		for _, f := range r.Filters {
			args = append(args, "--file-filter="+f.Name+" | "+globs(f, " "))
		}
	}
	return args
}

// KdialogArgs is the command line for kdialog's file dialogs.
func KdialogArgs(r Request) []string {
	var args []string
	start := startPath(r)
	switch r.Kind {
	case Open:
		args = append(args, "--getopenfilename", start)
	case OpenMultiple:
		args = append(args, "--getopenfilename", start, "--multiple", "--separate-output")
	case Folder:
		args = append(args, "--getexistingdirectory", start)
	case Save:
		args = append(args, "--getsavefilename", start)
	}
	if r.Kind != Folder && len(r.Filters) > 0 {
		var groups []string
		for _, f := range r.Filters {
			groups = append(groups, f.Name+" ("+globs(f, " ")+")")
		}
		// The filter argument follows the start directory in kdialog's
		// positional order, so it must come before the multiple flags.
		args = append(args[:2], append([]string{strings.Join(groups, "\n")}, args[2:]...)...)
	}
	if r.Title != "" {
		args = append(args, "--title", r.Title)
	}
	return args
}

// startPath is where a dialog begins: the directory, the proposed file
// inside it, or nothing. A trailing separator is what tells both programs
// that a bare directory is a place rather than a name.
func startPath(r Request) string {
	switch {
	case r.Directory != "" && r.FileName != "":
		return filepath.Join(r.Directory, r.FileName)
	case r.Directory != "":
		return strings.TrimRight(r.Directory, string(filepath.Separator)) + string(filepath.Separator)
	case r.FileName != "":
		return r.FileName
	}
	return ""
}
