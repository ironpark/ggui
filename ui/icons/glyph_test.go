package icons

import (
	"bytes"
	"testing"
)

func TestGlyphParsesItsSourceOnce(t *testing.T) {
	t.Parallel()
	g := &Glyph{Name: "dot", Source: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="4"/></svg>`}
	a, err := g.Parse()
	if err != nil || a == nil || g.SVG() != a {
		t.Fatalf("parsed %v, %v, then %v", a, err, g.SVG())
	}
	if w := g.Icon(); w.asset != a {
		t.Fatal("the widget does not draw the glyph's SVG")
	}
}

// A glyph kept as Data draws as the same icon kept as Source does.
func TestGlyphReadsParsedDataAsItsSource(t *testing.T) {
	t.Parallel()
	src := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M12 15V3"/><circle cx="12" cy="12" r="9"/></svg>`
	parsed := (&Glyph{Name: "source", Source: src}).SVG()
	data, err := parsed.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	read := (&Glyph{Name: "data", Data: string(data)}).SVG()
	if want, got := parsed.Image(48), read.Image(48); !bytes.Equal(want.Pix, got.Pix) {
		t.Fatal("the icon read from its data draws differently from its source")
	}
	if _, err := (&Glyph{Name: "bad", Data: "\x01nothing"}).Parse(); err == nil {
		t.Fatal("data that is no icon was read")
	}
}

func TestGlyphReportsASourceItCannotRead(t *testing.T) {
	t.Parallel()
	g := &Glyph{Name: "broken", Source: `<svg`}
	if _, err := g.Parse(); err == nil {
		t.Fatal("a broken source parsed")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("SVG returned for a broken source")
		}
	}()
	g.SVG()
}
