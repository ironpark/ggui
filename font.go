package ggui

import (
	"fmt"
	"sync"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/reactive"
	"github.com/ironpark/ggui/internal/third_party/text"
)

// Font is a loaded TrueType or OpenType face, usable at any size. Glyphs
// it lacks are drawn from its fallbacks: the ones given with Fallback, or
// else the system fonts SystemFonts finds, so Korean, Japanese and Chinese
// text renders with the built-in Latin font on a machine that has a CJK
// font installed.
type Font struct {
	f *fontsys.Font // the engine's font; see internal/fontsys
}

// wrapFont is the Font that draws f, nil for nil. Each call makes a new
// Font, and styles compare fonts by pointer (see TextStyle.Equal), so
// a font handed out more than once is wrapped once and kept, as
// SystemFonts and the defaults are.
func wrapFont(f *fontsys.Font) *Font {
	if f == nil {
		return nil
	}
	return &Font{f: f}
}

// engine is f's font in the engine, nil for a nil Font.
func (f *Font) engine() *fontsys.Font {
	if f == nil {
		return nil
	}
	return f.f
}

// FontWeight is a CSS font weight, from 100 (thin) to 900 (black). Zero
// asks for nothing, so a TextStyle without one inherits it.
type FontWeight int

const (
	WeightThin       FontWeight = 100
	WeightExtraLight FontWeight = 200
	WeightLight      FontWeight = 300
	WeightRegular    FontWeight = 400
	WeightMedium     FontWeight = 500
	WeightSemibold   FontWeight = 600
	WeightBold       FontWeight = 700
	WeightExtraBold  FontWeight = 800
	WeightBlack      FontWeight = 900
)

// LoadFont parses TTF or OTF bytes.
func LoadFont(data []byte) (*Font, error) {
	f, err := fontsys.Load(data)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return wrapFont(f), nil
}

// LoadFontCollection parses a TTC or OTC collection into one Font per face.
func LoadFontCollection(data []byte) ([]*Font, error) {
	fs, err := fontsys.LoadCollection(data)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font collection: %w", err)
	}
	return wrapFonts(fs), nil
}

func wrapFonts(fs []*fontsys.Font) []*Font {
	fonts := make([]*Font, len(fs))
	for i, f := range fs {
		fonts[i] = wrapFont(f)
	}
	return fonts
}

// LoadFontFile loads the font file at path: a TTF or OTF, or the first
// face of a TTC or OTC collection. The file is read as the font draws,
// not into memory, and stays open: a font's glyphs are parsed once text
// needs them, and a color emoji font's bitmaps read as they are drawn.
func LoadFontFile(path string) (*Font, error) {
	f, err := fontsys.LoadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ggui: load font: %w", err)
	}
	return wrapFont(f), nil
}

// MustFont is LoadFont for embedded data that is known to be valid.
func MustFont(data []byte) *Font {
	f, err := LoadFont(data)
	if err != nil {
		panic(err)
	}
	return f
}

// WithWeight adds face as f's face for weight w: text whose style asks for
// a weight is drawn with the face nearest it, f itself among them. A
// variable font with a weight axis needs none, since it draws every weight
// on the axis.
//
//	inter := ggui.MustFont(interRegular).
//		WithWeight(ggui.WeightMedium, ggui.MustFont(interMedium)).
//		WithWeight(ggui.WeightBold, ggui.MustFont(interBold))
func (f *Font) WithWeight(w FontWeight, face *Font) *Font {
	reactive.CheckUIThread("Font.WithWeight")
	f.f.WithWeight(fontsys.Weight(w), face.engine())
	return f
}

// Fallback appends fonts to draw the glyphs f lacks, in order of
// preference. A Font with none set falls back to SystemFonts.
func (f *Font) Fallback(fonts ...*Font) *Font {
	reactive.CheckUIThread("Font.Fallback")
	fs := make([]*fontsys.Font, 0, len(fonts))
	for _, fb := range fonts {
		fs = append(fs, fb.engine())
	}
	f.f.AddFallbacks(fs...)
	return f
}

// NoFallback draws only f's own glyphs, with no system fonts behind it.
func (f *Font) NoFallback() *Font {
	reactive.CheckUIThread("Font.NoFallback")
	f.f.SetNoFallback()
	return f
}

// SystemFonts returns the fonts found on this machine that cover scripts
// the built-in font does not, mainly CJK, and then symbols interface fonts
// lack, such as the arrows, dingbats and technical symbols command-line
// programs print (❯ ⏵ ⏺ ⎿), found once by family in the index of the
// system's fonts that go-text's fontscan keeps. It is what every Font
// falls back to first; a glyph none of them has is then looked for in
// every font the system has. Call it to see what was found, and Fallback
// to choose differently. The GGUI_FONTS environment variable lists extra
// files, separated like PATH, tried first.
func SystemFonts() []*Font { return systemFonts() }

var systemFonts = sync.OnceValue(func() []*Font { return wrapFonts(fontsys.SystemFonts()) })

// systemUIFonts are the platform's interface fonts, loaded once, or nil
// for one it does not find; see fontsys.SystemUI.
var systemUIFonts = sync.OnceValues(func() (sans, mono *Font) {
	s, m := fontsys.SystemUI()
	return wrapFont(s), wrapFont(m)
})

// SystemItalicFonts returns the italic faces of the platform's interface
// fonts, with every weight it has of them, loaded once: SF Pro Italic and
// SF Mono Italic on macOS, Segoe UI Italic and Cascadia Mono Italic or
// Consolas Italic on Windows, and what fontconfig matches for sans-serif
// and monospace in italic elsewhere. Either is nil where the platform has
// no italic of that font, and fontconfig's match for an italic it does not
// have, an upright face, is not taken for one. Like any Font they fall back
// to SystemFonts. Text has no italic style of its own; a widget that draws
// italics, such as a terminal, chooses these for them.
func SystemItalicFonts() (sans, mono *Font) { return systemItalicFonts() }

var systemItalicFonts = sync.OnceValues(func() (sans, mono *Font) {
	s, m := fontsys.SystemItalic()
	return wrapFont(s), wrapFont(m)
})

// DefaultTextSize is the size Text uses until Size is set.
const DefaultTextSize = 14

// builtinFont is Go Regular, parsed on first use so a program that draws
// no text never pays for it. It is the floor under every default: the font
// of a Probe, and of an App where no system font is found.
var builtinFont = sync.OnceValue(func() *Font { return wrapFont(fontsys.Builtin()) })

// defaultMono stands for the default monospaced font, which is chosen
// where it draws: see DefaultMonoFont.
var defaultMono = wrapFont(fontsys.Alias(func() *fontsys.Font { return monoFont().f }))

// DefaultMonoFont returns a Font that draws in the default monospaced font
// of whatever App or Probe it draws in: the one SetDefaultMonoFont set, or
// in an App the platform's own, such as SF Mono on macOS, or else the
// default text font. The theme's Mono style uses it.
func DefaultMonoFont() *Font { return defaultMono }

// SetDefaultFont replaces the font Text uses when none is set. An App
// draws in the platform's interface font, SF Pro on macOS, Segoe UI on
// Windows and fontconfig's sans-serif elsewhere, and a Probe, a browser, or
// an App on a machine without one, in Go Regular, which covers Latin, Greek
// and Cyrillic. fonts/gofont adds Go Medium and Go Bold for the same look
// everywhere.
//
// Like SetEnv, it sets the font of the App or Probe whose frames run on
// this goroutine, or, before any has, the one every App and Probe made on
// it uses until given its own. Nil goes back to that inherited font, or
// to the default above.
func SetDefaultFont(f *Font) {
	reactive.CheckUIThread("SetDefaultFont")
	w := activeWorld()
	w.font = f
	w.fontGen++
}

// SetDefaultMonoFont replaces the font DefaultMonoFont draws in, in the
// same scope SetDefaultFont sets the text font in. Nil goes back to the
// default.
func SetDefaultMonoFont(f *Font) {
	reactive.CheckUIThread("SetDefaultMonoFont")
	w := activeWorld()
	w.mono = f
	w.fontGen++
}

// fallbackFont returns the font Text uses when none is set.
func fallbackFont() *Font {
	native := false
	for w := range activeWorld().chain() {
		if w.font != nil {
			return w.font
		}
		native = native || w.nativeFonts
	}
	if native {
		if f, _ := systemUIFonts(); f != nil {
			return f
		}
	}
	return builtinFont()
}

// monoFont returns the font DefaultMonoFont draws in.
func monoFont() *Font {
	native := false
	for w := range activeWorld().chain() {
		if w.mono != nil {
			return w.mono
		}
		native = native || w.nativeFonts
	}
	if native {
		if _, f := systemUIFonts(); f != nil {
			return f
		}
	}
	return fallbackFont()
}

// face is f's face at size and weight, beside the emoji font the active
// world chose.
func (f *Font) face(size float64, weight FontWeight) text.Face {
	return f.f.Face(size, fontsys.Weight(weight), activeEmoji)
}

// fontGen changes whenever a measurement made in the active world with the
// fonts it resolves may no longer hold: a Font changed (fontsys.Generation
// counts those, which reach every world that measures with it), or the
// active world or one it inherits from chose another default or emoji
// font. Every part only grows, so their sum changes whenever one does.
// CachedWidget and Text read it to decide whether a measurement still
// holds.
func fontGen() uint64 {
	g := fontsys.Generation()
	for w := range activeWorld().chain() {
		g += w.fontGen
	}
	return g
}

// activeEmoji is the active world's emoji choice: what SetEmojiFont chose,
// nil font included, or nil when it was never called and the system font
// is used.
func activeEmoji() *fontsys.EmojiChoice {
	for w := range activeWorld().chain() {
		if w.emoji != nil {
			return w.emoji
		}
	}
	return nil
}

// SetEmojiFont selects a color emoji font for Text and TextInput. It does not
// replace their Latin/CJK fonts. Use the fonts/notoemoji module for a portable embedded font,
// or LoadFont for another CBDT, sbix, COLRv0 or OpenType SVG font. Nil disables
// emoji substitution; SetEmojiFont(SystemEmojiFont()) restores system rendering.
// Like SetEnv, call on the UI thread or before creating the app: it sets the
// emoji font of the App or Probe whose frames run on this goroutine, or,
// before any has, the one every App and Probe made on it uses until given
// its own.
func SetEmojiFont(f *Font) {
	reactive.CheckUIThread("SetEmojiFont")
	setEmoji(activeWorld(), &fontsys.EmojiChoice{Font: f.engine()})
}

// setEmoji makes c w's emoji choice, nil to inherit its parent's.
func setEmoji(w *world, c *fontsys.EmojiChoice) {
	w.emoji = c
	w.fontGen++
}

// SystemEmojiFont finds the platform color emoji font once, or returns nil.
// Browsers have no filesystem font access; load a remote or embedded font there.
func SystemEmojiFont() *Font { return systemEmojiFont() }

var systemEmojiFont = sync.OnceValue(func() *Font { return wrapFont(fontsys.SystemEmoji()) })
