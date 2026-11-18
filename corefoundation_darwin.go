//go:build darwin && !ios

package ggui

import (
	"sync"

	"github.com/ebitengine/purego"
)

// coreFoundation is the little of CoreFoundation SystemLanguages and the
// login item call, reached through purego as the rest of the platform code
// is. Each of its functions may be called from any thread.
type coreFoundation struct {
	copyPreferredLanguages func() uintptr
	mainBundle             func() uintptr
	bundleIdentifier       func(bundle uintptr) uintptr
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
	purego.RegisterLibFunc(&cf.mainBundle, lib, "CFBundleGetMainBundle")
	purego.RegisterLibFunc(&cf.bundleIdentifier, lib, "CFBundleGetIdentifier")
	purego.RegisterLibFunc(&cf.arrayGetCount, lib, "CFArrayGetCount")
	purego.RegisterLibFunc(&cf.arrayGetValueAtIndex, lib, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&cf.stringGetCString, lib, "CFStringGetCString")
	purego.RegisterLibFunc(&cf.release, lib, "CFRelease")
	return cf
})

// string returns the CFString s as Go text, or "" for none or for one too
// long for the few names read here.
func (cf *coreFoundation) string(s uintptr) string {
	var buf [1024]byte
	if s == 0 || !cf.stringGetCString(s, &buf[0], len(buf), cfStringEncodingUTF8) {
		return ""
	}
	n := 0
	for n < len(buf) && buf[n] != 0 {
		n++
	}
	return string(buf[:n])
}

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
	for i := range cf.arrayGetCount(arr) {
		if s := cf.string(cf.arrayGetValueAtIndex(arr, i)); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// bundleID is the identifier in the Info.plist of the bundle the app runs
// from, or "" for an executable run outside one.
func bundleID() string {
	cf := loadCoreFoundation()
	if cf == nil {
		return ""
	}
	b := cf.mainBundle()
	if b == 0 {
		return ""
	}
	return cf.string(cf.bundleIdentifier(b))
}
