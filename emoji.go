package ggui

import (
	"sync"

	"github.com/ironpark/ggui/internal/fontsys"
	"github.com/ironpark/ggui/internal/reactive"
)

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
