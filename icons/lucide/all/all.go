// Package all lists every Lucide icon, for looking one up by a name known
// only at run time: an icon picker, a name read from a file. Importing it
// links every icon into the program, about 2055 of them; a
// program that names its icons in its code needs only the lucide package.
package all

import (
	"sort"

	"github.com/ironpark/ggui/ui/icons"
)

// Glyphs is every icon in name order. The slice is shared: do not change it.
func Glyphs() []*icons.Glyph { return glyphs[:] }

// Lookup is the icon the library calls name, "arrow-down", or nil.
func Lookup(name string) *icons.Glyph {
	i := sort.Search(len(glyphs), func(i int) bool { return glyphs[i].Name >= name })
	if i < len(glyphs) && glyphs[i].Name == name {
		return glyphs[i]
	}
	return nil
}

// Names is every icon's name, in order.
func Names() []string {
	names := make([]string, len(glyphs))
	for i, g := range glyphs {
		names[i] = g.Name
	}
	return names
}
