//go:build !darwin && !linux && !windows

package watch

import "errors"

func newSource(func(string) bool) (source, error) {
	return nil, errors.New("no file change notifications on this system")
}
