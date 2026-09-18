// Package notoemoji supplies an optional, embedded Noto Color Emoji font.
// Enable it for portable color emoji in Text and TextInput, including WASM.
package notoemoji

import (
	_ "embed"
	"sync"

	"github.com/ironpark/ggui"
)

//go:embed NotoColorEmoji.ttf
var data []byte

var load = sync.OnceValue(func() *ggui.Font { return ggui.MustFont(data) })

// Font returns the shared font. Treat it as immutable.
func Font() *ggui.Font { return load() }

// Enable selects the embedded font for emoji without replacing the text font.
func Enable() { ggui.SetEmojiFont(Font()) }
