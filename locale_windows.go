package ggui

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetUserDefaultLocaleName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")

// systemLanguages is the user's preferred display languages, or, where
// Windows gives none, the user's locale.
func systemLanguages() []string {
	if langs, err := windows.GetUserPreferredUILanguages(windows.MUI_LANGUAGE_NAME); err == nil && len(langs) > 0 {
		return langs
	}
	if procGetUserDefaultLocaleName.Find() != nil {
		return nil
	}
	// LOCALE_NAME_MAX_LENGTH is 85 characters, the null included.
	var buf [85]uint16
	n, _, _ := procGetUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return nil
	}
	return []string{windows.UTF16ToString(buf[:])}
}
