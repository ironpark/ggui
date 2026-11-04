package fontsys

import (
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestPickerLooksInTheChainThenTheSystem(t *testing.T) {
	t.Parallel()
	f := emojiChain(t)
	p := f.Picker(400)
	if p.Len() != 2 {
		t.Fatalf("chain of %d fonts, want the font and its fallback", p.Len())
	}
	if i, ok := p.Pick('a'); !ok || i != 0 {
		t.Fatalf("'a' picked %d, %v: want the font itself", i, ok)
	}
	if i, ok := p.Pick('👍'); !ok || i != 1 {
		t.Fatalf("👍 picked %d, %v: want the fallback that has it", i, ok)
	}
	// A font with fallbacks of its own looks in no system font.
	if _, ok := p.Pick('한'); ok || p.Len() != 2 {
		t.Fatalf("a chain with its own fallbacks found 한 among the system's: %d fonts", p.Len())
	}

	bare := mustLoad(t, goregular.TTF)
	bare.SetNoFallback()
	if _, ok := bare.Picker(400).Pick('👍'); ok {
		t.Fatal("a font with no fallback picked a rune it lacks")
	}
	if bare.DrawsEmoji() || !f.DrawsEmoji() {
		t.Fatal("DrawsEmoji does not follow NoFallback")
	}
}

// A rune only a system font outside SystemFonts has adds that font to the
// end of the chain, once, as Face's fallback chain finds it after the
// chain's own fonts.
func TestPickerAddsASystemFontOnce(t *testing.T) {
	t.Parallel()
	p := mustLoad(t, goregular.TTF).Picker(400)
	own := p.Len()
	var r rune
	for _, c := range []rune{'𓀀', 'ᚠ', 'Ꭰ', 'ⴰ', 'ሀ', 'ก', 'ა', 'अ'} {
		if i, ok := p.Pick(c); ok && i >= own {
			r = c
			break
		}
	}
	if r == 0 {
		t.Skip("no system font has a rune that SystemFonts lack")
	}
	if p.Len() != own+1 {
		t.Fatalf("%c added %d fonts, want one", r, p.Len()-own)
	}
	if i, _ := p.Pick(r); i != own || p.Len() != own+1 {
		t.Fatalf("%c picked again at %d in a chain of %d: want the same font, not added again", r, i, p.Len())
	}
	if i, _ := p.Pick('a'); i != 0 {
		t.Fatalf("'a' picked %d after a system font joined: want the font itself", i)
	}
}

// Pickers on goroutines of their own read the cmaps of the fonts they
// share; run with -race.
func TestPickersShareFontsAcrossGoroutines(t *testing.T) {
	t.Parallel()
	f := mustLoad(t, goregular.TTF)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := f.Picker(400)
			for r := rune(0x20); r < 0x2000; r++ {
				p.Pick(r)
			}
		}()
	}
	wg.Wait()
}
