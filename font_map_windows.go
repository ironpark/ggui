//go:build windows

package ggui

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapFontFile maps f, of size bytes, into memory, as font_map_unix.go
// does: a copy-on-write view, so that a write, which nothing should make,
// changes a private copy of a page rather than faulting; untouched, its
// pages stay the file's. The view outlives the mapping's handle.
func mapFontFile(f *os.File, size int64) (mappedFont, bool) {
	if size <= 0 || int64(int(size)) != size {
		return nil, false
	}
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_WRITECOPY, uint32(uint64(size)>>32), uint32(size), nil)
	if err != nil {
		return nil, false
	}
	defer windows.CloseHandle(h)
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_COPY, 0, 0, uintptr(size))
	if err != nil {
		return nil, false
	}
	// The view is the system's memory, not the Go heap's: its address is
	// taken as a pointer as it is, which no collector moves or frees.
	base := *(*unsafe.Pointer)(unsafe.Pointer(&addr))
	return unsafe.Slice((*byte)(base), int(size)), true
}
