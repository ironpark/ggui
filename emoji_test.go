package ggui

import (
	"os"
	"strings"
	"testing"

	"github.com/ironpark/ggui/internal/textedit"

	"github.com/ironpark/ggfx/text/v2"
)

func useTestEmoji(t *testing.T) *Font {
	t.Helper()
	data, err := os.ReadFile("testdata/emoji-subset.ttf")
	if err != nil {
		t.Fatal(err)
	}
	f := MustFont(data)
	w := activeWorld()
	old := w.emoji
	SetEmojiFont(f)
	t.Cleanup(func() { setEmoji(w, old) })
	return f
}
func TestEmojiSequencesStayInOneColorGlyph(t *testing.T) {
	t.Parallel()
	useTestEmoji(t)
	face := fallbackFont().face(20, 0)
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
	t.Parallel()
	useTestEmoji(t)
	face := fallbackFont().face(20, 0)
	ef := face.(*emojiFace)
	for _, s := range []string{"AV 123 # * © ♥︎", "plain text"} {
		if got, want := lineWidth(s, face), text.AdvanceAt(s, len(s), ef.Face); got != want {
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
		e := textedit.Editor{Text: source, Caret: len(source), Anchor: len(source)}
		e.Backspace(false)
		if e.Text != strings.Repeat(s, 2) {
			t.Fatalf("backspace broke %q: %q", s, e.Text)
		}
	}
}
func TestEmojiSwitchInvalidatesCachedMeasurement(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	for _, s := range []string{"👍🏽", "🇰🇷", "👩🏽‍💻", "👨‍👩‍👧‍👦", "1️⃣", "🏳️‍🌈", "❤️"} {
		t.Run(s, func(t *testing.T) {
			// A subtest runs on a goroutine of its own, whose settings are
			// its own.
			useTestEmoji(t)
			useFakeIME(t)
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
