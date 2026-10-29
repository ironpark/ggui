//go:build unix

package ggui

import (
	"os"

	"golang.org/x/sys/unix"
)

// mapFontFile maps f, of size bytes, into memory. The mapping is private
// and writable, so that a write, which nothing should make, changes a
// private copy of a page rather than faulting; untouched, its pages stay
// the file's.
func mapFontFile(f *os.File, size int64) (mappedFont, bool) {
	if size <= 0 || int64(int(size)) != size {
		return nil, false
	}
	b, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE)
	if err != nil {
		return nil, false
	}
	return b, true
}
