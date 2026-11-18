//go:build darwin && !ios

package ggui

import (
	"sync"

	"github.com/ebitengine/purego"
)

// coreFoundation is the little of CoreFoundation SystemLanguages calls,
// reached through purego as the rest of the platform code is. Each of its
// functions may be called from any thread.
type coreFoundation struct {
	copyPreferredLanguages func() uintptr
	arrayGetCount          func(arr uintptr) int
	arrayGetValueAtIndex   func(arr uintptr, i int) uintptr
	stringGetCString       func(s uintptr, buf *byte, size int, encoding uint32) bool
	release                func(cf uintptr)
}

// cfStringEncodingUTF8 is kCFStringEncodingUTF8.
const cfStringEncodingUTF8 = 0x08000100

// loadCoreFoundation opens the framework once, or reports nil when it
// cannot.
var loadCoreFoundation = sync.OnceValue(func() *coreFoundation {
	lib, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
	if err != nil {
		return nil
	}
	cf := &coreFoundation{}
	purego.RegisterLibFunc(&cf.copyPreferredLanguages, lib, "CFLocaleCopyPreferredLanguages")
	purego.RegisterLibFunc(&cf.arrayGetCount, lib, "CFArrayGetCount")
	purego.RegisterLibFunc(&cf.arrayGetValueAtIndex, lib, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&cf.stringGetCString, lib, "CFStringGetCString")
	purego.RegisterLibFunc(&cf.release, lib, "CFRelease")
	return cf
})

// systemLanguages is the list in Language & Region settings, as
// CFLocaleCopyPreferredLanguages gives it to this app.
func systemLanguages() []string {
	cf := loadCoreFoundation()
	if cf == nil {
		return nil
	}
	arr := cf.copyPreferredLanguages()
	if arr == 0 {
		return nil
	}
	defer cf.release(arr)
	var out []string
	// A language tag is a few dozen bytes at most.
	var buf [256]byte
	for i := range cf.arrayGetCount(arr) {
		s := cf.arrayGetValueAtIndex(arr, i)
		if s == 0 || !cf.stringGetCString(s, &buf[0], len(buf), cfStringEncodingUTF8) {
			continue
		}
		n := 0
		for n < len(buf) && buf[n] != 0 {
			n++
		}
		out = append(out, string(buf[:n]))
	}
	return out
}
