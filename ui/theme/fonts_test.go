package theme

import (
	"testing"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/fonts/geist"
	"github.com/ironpark/ggui/fonts/gofont"
	"github.com/ironpark/ggui/fonts/inter"
)

func init() {
	RegisterFont("Inter", inter.Font)
	RegisterFont("Geist", geist.Font)
	RegisterFont("Geist Sans", geist.Font)
	RegisterFont("Geist Mono", geist.Mono)
	RegisterFont("Go", gofont.Font)
	RegisterFont("Go Sans", gofont.Font)
	RegisterFont("Go Mono", gofont.Mono)
}

// The font variables select registered families by name, the first one the
// list has, and leave the font alone when it names none.
func TestApplyCSSMapsFontVariablesToRegisteredFonts(t *testing.T) {
	t.Parallel()
	custom := gofont.Mono()
	RegisterFont("Test Grotesk", func() *ggui.Font { return custom })
	th, err := Default().ApplyCSS(`
		--font-sans: "Geist Variable", ui-sans-serif, system-ui;
		--font-heading: 'Unknown Display', test-grotesk, serif;
		--font-mono: var(--font-geist-mono), monospace;`)
	if err != nil {
		t.Fatal(err)
	}
	if th.Text.Font != geist.Font() {
		t.Error("--font-sans did not select Geist")
	}
	if th.Title.Font != custom || th.Heading.Font != custom {
		t.Error("--font-heading did not select the registered family for Title and Heading")
	}
	if th.Mono.Font != geist.Mono() {
		t.Error("--font-mono did not follow an undefined var() to the family it names")
	}
	if th.Resolve() != th {
		t.Error("the fonts did not survive Resolve")
	}
	plain, err := Default().ApplyCSS(`--font-sans: ui-sans-serif, system-ui, sans-serif; --font-mono: var(--font-sans);`)
	if err != nil {
		t.Fatal(err)
	}
	if plain.Text.Font != Default().Text.Font || plain.Mono.Font != Default().Mono.Font {
		t.Error("a list of system fonts replaced the theme's fonts")
	}
	via, err := Default().ApplyCSS(`--font-inter: Inter; --font-sans: var(--font-inter);`)
	if err != nil || via.Text.Font != inter.Font() {
		t.Errorf("--font-sans did not follow var() to a declared family: %v", err)
	}
	light, dark, err := FromCSS(`:root { --font-sans: Geist, sans-serif; } .dark { --font-mono: "Geist Mono"; }`)
	if err != nil || light.Text.Font != geist.Font() || dark.Text.Font != geist.Font() || dark.Mono.Font != geist.Mono() || light.Mono.Font == geist.Mono() {
		t.Errorf("FromCSS did not carry the fonts into both themes: %v", err)
	}
}

func TestRegisterFontNilUnregistersAndCustomConfigFont(t *testing.T) {
	t.Parallel()
	const family = "Temp Unregister Grotesk"
	custom := gofont.Mono()
	RegisterFont(family, func() *ggui.Font { return custom })
	if !hasFontNamed(family) || !hasConfigFont(FontFamily(family)) {
		t.Fatal("registered custom font was not detected")
	}
	if got := (Config{Font: FontFamily(family)}).Light().Text.Font; got != custom {
		t.Fatal("Config did not select custom registered font")
	}
	RegisterFont(family, nil)
	if hasFontNamed(family) || hasConfigFont(FontFamily(family)) || fontNamed(family) != nil {
		t.Fatal("RegisterFont(name, nil) did not remove the registration")
	}
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_ = (Config{Font: FontFamily(family)}).Light()
	}()
	if !panicked {
		t.Fatal("unregistered custom font did not panic")
	}
}
