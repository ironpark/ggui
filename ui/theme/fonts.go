package theme

import (
	"strings"
	"sync"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/geist"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/fonts/inter"
)

// fontFamilies are the fonts RegisterFont knows, by familyKey of their name.
var fontFamilies sync.Map // string -> func() *ggui.Font

func init() {
	RegisterFont("Inter", inter.Font)
	RegisterFont("Geist", geist.Font)
	RegisterFont("Geist Sans", geist.Font)
	RegisterFont("Geist Mono", geist.Mono)
	RegisterFont("Go", gofont.Font)
	RegisterFont("Go Sans", gofont.Font)
	RegisterFont("Go Mono", gofont.Mono)
}

// RegisterFont names a font family for ApplyCSS and FromCSS, so that a
// stylesheet's --font-sans, --font-heading or --font-mono naming it selects
// font's result:
//
//	theme.RegisterFont("Figtree", figtree.Font)
//	t, err := theme.Default().ApplyCSS(`--font-sans: "Figtree", sans-serif;`)
//
// The name is matched without regard to case, quotes, a trailing "Variable"
// or whether its words are joined by spaces or hyphens. font is called when
// a stylesheet first asks for the family and should return the same Font
// each time, since text styles compare fonts by identity. Inter, Geist,
// Geist Mono, Go and Go Mono are registered already; registering a name
// again replaces it.
func RegisterFont(family string, font func() *ggui.Font) {
	fontFamilies.Store(familyKey(family), font)
}

// familyKey is a CSS font family name as fontFamilies keys it.
func familyKey(name string) string {
	name = strings.ToLower(strings.Trim(strings.TrimSpace(name), `"'`))
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' }), " ")
	return strings.TrimSuffix(name, " variable")
}

// fontNamed is the registered font of the family, nil for one not registered.
func fontNamed(family string) *ggui.Font {
	if f, ok := fontFamilies.Load(familyKey(family)); ok {
		return f.(func() *ggui.Font)()
	}
	return nil
}

// cssFonts maps shadcn/ui's font variables to the text styles they set.
var cssFonts = map[string]func(*Theme) []*ggui.TextStyle{
	"font-sans":    func(t *Theme) []*ggui.TextStyle { return []*ggui.TextStyle{&t.Text} },
	"font-heading": func(t *Theme) []*ggui.TextStyle { return []*ggui.TextStyle{&t.Title, &t.Heading} },
	"font-mono":    func(t *Theme) []*ggui.TextStyle { return []*ggui.TextStyle{&t.Mono} },
}

// cssFont is the first registered family the font variable name lists, nil
// when it lists none. A var() of a variable the declarations lack, as a
// framework's own --font-geist-sans is, names the family after it.
func cssFont(vars map[string]string, name string) *ggui.Font {
	return fontIn(vars, vars[name], 0)
}

// fontIn is the first registered family in a font-family list, following
// each var() in it.
func fontIn(vars map[string]string, list string, depth int) *ggui.Font {
	for len(list) > 0 && depth < 8 {
		// One family ends at the next comma outside parentheses.
		end, nest := len(list), 0
		for i, r := range list {
			if r == '(' {
				nest++
			} else if r == ')' {
				nest--
			} else if r == ',' && nest == 0 {
				end = i
				break
			}
		}
		family := strings.TrimSpace(list[:end])
		list = strings.TrimPrefix(list[end:], ",")
		inner, isVar := strings.CutPrefix(family, "var(")
		if !isVar {
			if f := fontNamed(family); f != nil {
				return f
			}
			continue
		}
		ref, fallback, _ := strings.Cut(strings.TrimSuffix(inner, ")"), ",")
		ref = strings.TrimPrefix(strings.TrimSpace(ref), "--")
		if value, ok := vars[ref]; ok {
			if f := fontIn(vars, value, depth+1); f != nil {
				return f
			}
		} else if f := fontNamed(strings.TrimPrefix(ref, "font-")); f != nil {
			return f
		} else if f := fontIn(vars, fallback, depth+1); f != nil {
			return f
		}
	}
	return nil
}
