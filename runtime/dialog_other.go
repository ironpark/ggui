//go:build (!darwin && !windows) || ios

package runtime

import (
	"errors"

	"github.com/ironpark/ggui/internal/dialogexec"
)

// showDialog runs the zenity or kdialog program, whichever is on PATH.
func showDialog(k dialogKind, d FileDialog) ([]string, error) {
	filters := make([]dialogexec.Filter, len(d.Filters))
	for i, f := range d.Filters {
		filters[i] = dialogexec.Filter{Name: f.Name, Extensions: f.Extensions}
	}
	paths, err := dialogexec.Show(dialogexec.Request{
		Kind:      dialogexec.Kind(k),
		Title:     d.Title,
		Directory: d.Directory,
		FileName:  d.FileName,
		Filters:   filters,
	})
	switch {
	case errors.Is(err, dialogexec.ErrCanceled):
		return nil, ErrCanceled
	case errors.Is(err, dialogexec.ErrNoProgram):
		return nil, ErrUnsupported
	}
	return paths, err
}
