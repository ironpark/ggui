package robustio

import (
	"errors"
	"syscall"
)

// transient reports whether err is Windows refusing because a file is open
// elsewhere: access denied, which replacing an open file gives, or a
// sharing violation.
func transient(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	const (
		accessDenied     syscall.Errno = 5
		sharingViolation syscall.Errno = 32
	)
	return errno == accessDenied || errno == sharingViolation
}
