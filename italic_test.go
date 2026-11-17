package ggui

import "testing"

// Text styled Italic is set in its font's italic: the one WithItalic gave
// it, inherited from an ancestor's style; a font with none stays upright,
// and an italic styled Italic again stays itself.
func TestItalicTextTakesItsFontsItalic(t *testing.T) {
	upright, italic := wrapFont(builtinFont().f), wrapFont(builtinFont().f)
	upright.WithItalic(italic)
	st := TextStyle{Font: upright}.Merge(TextStyle{Italic: true})
	if got := st.resolved().Font; got != italic {
		t.Fatalf("italic text is set in %p, not the italic %p", got, italic)
	}
	if got := st.resolved().resolved().Font; got != italic {
		t.Fatalf("resolved twice, it is set in %p", got)
	}
	if st.Merge(TextStyle{}).Italic != true {
		t.Fatal("a descendant's style lost the italic")
	}
	if (TextStyle{Font: upright}).Equal(st) {
		t.Fatal("italic text draws the same as upright")
	}
	plain := wrapFont(builtinFont().f)
	if got := (TextStyle{Font: plain, Italic: true}).resolved().Font; got != plain {
		t.Fatalf("a font with no italic is set in %p", got)
	}
	if sans, _ := systemUIFonts(); sans != nil {
		if i, _ := systemItalicFonts(); i != nil && sans.italicFace() != i {
			t.Fatal("the interface font's italic is not the system's")
		}
	}
}
