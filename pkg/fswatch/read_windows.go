package fswatch

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// readFile reads the named file without keeping others from deleting or
// renaming it meanwhile: os.Open does not share deletion, so a file the
// watcher reads the moment it changed could not be replaced by the
// program still saving it until the read is over.
func readFile(name string) ([]byte, error) {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	h, err := syscall.CreateFile(p, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	f := os.NewFile(uintptr(h), name)
	defer f.Close()
	return io.ReadAll(f)
}

// transient reports whether err is Windows refusing because a file is open
// elsewhere: access denied, which a file being replaced gives, or a
// sharing violation, which one open without sharing reads gives. Both
// pass in milliseconds, so the watcher looks again.
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
