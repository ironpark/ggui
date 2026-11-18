//go:build !(darwin && !ios) && !windows && !js

package ggui

import "os"

// systemLanguages reads the POSIX environment, where Linux and the BSDs
// keep the user's languages.
func systemLanguages() []string { return posixLanguages(os.Getenv) }
