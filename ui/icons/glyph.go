package icons

import (
	"fmt"
	"sync"
)

// Glyph is one icon of a library, read the first time it is asked for. A
// library declares one variable for each of its icons, and a program is
// linked with only those it names, so a whole catalog costs a binary nothing
// until an icon is used. Looking an icon up by a name known only at run time
// needs every one, which a library offers in a package of its own for that
// reason.
//
// A glyph holds its icon as Data or as Source. Data is the icon already
// parsed, in the form SVG.MarshalBinary writes: smaller than the source and
// read without parsing, which is how a generated library keeps its icons.
// Source is the SVG document itself, the simple way to declare one's own:
//
//	var Logo = &icons.Glyph{Name: "logo", Source: `<svg viewBox="0 0 24 24">…</svg>`}
type Glyph struct {
	Name   string // as the library spells it, "arrow-down"
	Data   string // the parsed icon, read if not empty
	Source string // the SVG document, parsed otherwise

	once sync.Once
	svg  *SVG
	err  error
}

// Parse is the glyph's SVG, shared by every use, or why it could not be
// read.
func (g *Glyph) Parse() (*SVG, error) {
	g.once.Do(func() {
		if g.Data != "" {
			g.svg, g.err = ParseBinary([]byte(g.Data))
		} else {
			g.svg, g.err = Parse([]byte(g.Source))
		}
		if g.err != nil {
			g.err = fmt.Errorf("icon %q: %w", g.Name, g.err)
		}
	})
	return g.svg, g.err
}

// SVG is the glyph's SVG, for an icons.Map or a custom Set. A glyph that
// cannot be read panics: a library's are checked when it is generated.
func (g *Glyph) SVG() *SVG {
	s, err := g.Parse()
	if err != nil {
		panic(err)
	}
	return s
}

// Icon is a widget drawing the glyph, whatever icon set the theme has.
func (g *Glyph) Icon() *Widget { return New(g.SVG()) }
