package ggui

import (
	"os"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
)

func useTestEmoji(t *testing.T) *Font {
	t.Helper()
	data, err := os.ReadFile("fonts/notoemoji/NotoColorEmoji.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := MustFont(data)
	old, set := emojiFont, emojiFontSet
	SetEmojiFont(f)
	t.Cleanup(func() { emojiFont, emojiFontSet = old, set; fontGeneration++; requestLayout() })
	return f
}
func TestEmojiSequencesStayInOneColorGlyph(t *testing.T) {
	useTestEmoji(t)
	face := fallbackFont().face(20)
	for _, s := range []string{"👍", "👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️", "🫩"} {
		t.Run(s, func(t *testing.T) {
			for run, f := range textRuns(s, face) {
				if run != s {
					t.Fatalf("split sequence: %q", run)
				}
				glyphs := text.AppendLazyGlyphs(nil, run, f, nil)
				if len(glyphs) != 1 || !glyphs[0].Colored() {
					t.Fatalf("%q: expected one color glyph, got %d", s, len(glyphs))
				}
			}
			if lineWidth(s, face) <= 0 {
				t.Fatal("zero width")
			}
		})
	}
}
func TestEmojiPresentationAndTextShaping(t *testing.T) {
	useTestEmoji(t)
	face := fallbackFont().face(20)
	ef := face.(*emojiFace)
	for _, s := range []string{"AV 123 # * © ♥︎", "plain text"} {
		if got, want := lineWidth(s, face), text.Advance(s, ef.Face); got != want {
			t.Fatalf("ordinary text width changed: %q %v != %v", s, got, want)
		}
	}
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "1️⃣", "🏳️‍🌈", "❤️"} {
		source := strings.Repeat(s, 3)
		if got := wrapText(source, face, 1); len(got) != 3 {
			t.Fatalf("split emoji clusters: %q", got)
		} else {
			for _, line := range got {
				if line != s {
					t.Fatalf("broken cluster: %q", line)
				}
			}
		}
		spans := wrapSpans(source, face, 1)
		if len(spans) != 3 {
			t.Fatalf("editor wrap: %+v", spans)
		}
		for _, span := range spans {
			if source[span.start:span.end] != s {
				t.Fatalf("editor broke cluster: %+v", span)
			}
		}
		e := textEditor{text: source, caret: len(source), anchor: len(source)}
		e.backspace(false)
		if e.text != strings.Repeat(s, 2) {
			t.Fatalf("backspace broke %q: %q", s, e.text)
		}
	}
}
func TestEmojiSwitchInvalidatesCachedMeasurement(t *testing.T) {
	useTestEmoji(t)
	w := Text("👩🏽‍💻")
	c := Cached(w)
	constraints := Loose(Sz(1000, 100))
	env := Env{}
	c.Layout(constraints, env)
	old := w.wrapped.generation
	SetEmojiFont(nil)
	c.Layout(constraints, env)
	if w.wrapped.generation == old {
		t.Fatal("cached text did not remeasure after font switch")
	}
}

func TestEmojiPointerCaretAndBackspace(t *testing.T) {
	useTestEmoji(t)
	useFakeIME(t)
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️"} {
		t.Run(s, func(t *testing.T) {
			value := State(s)
			w := TextInput(value)
			w.Layout(Tight(Sz(300, 30)), rootEnv())
			for x := 0.0; x <= w.advance(s)+5; x += .5 {
				i := w.indexInLine(lineSpan{0, len(s)}, x)
				if i != 0 && i != len(s) {
					t.Fatalf("click at x=%v placed caret inside emoji at byte %d/%d", x, i, len(s))
				}
			}
			w.rect = Rct(Point{}, Sz(300, 30))
			w.HandlePointer(PointerEvent{Kind: PointerDown, Button: MouseButtonLeft, Pos: Pt(299, 10)})
			w.HandleKey(KeyEvent{Kind: KeyPress, Key: KeyBackspace})
			if Untrack(value.Get) != "" {
				t.Fatalf("backspace left emoji fragments: %q", Untrack(value.Get))
			}
		})
	}
}
