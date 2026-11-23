//go:build !darwin && !linux && !windows

package fswatch

import "errors"

func newSource(bool, func(string) bool) (source, error) {
	return nil, errors.New("no file change notifications on this system")
}
