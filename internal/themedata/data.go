// Package themedata holds the vendored shadcn/ui semantic color tokens.
package themedata

import _ "embed"

//go:embed shadcn.json
var JSON []byte
