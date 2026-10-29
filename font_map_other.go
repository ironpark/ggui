//go:build !unix

package ggui

import "os"

// mapFontFile maps nothing here: the file is read where it is instead.
func mapFontFile(*os.File, int64) (mappedFont, bool) { return nil, false }
