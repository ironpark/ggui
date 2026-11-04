package font

import (
	"testing"

	ot "github.com/ironpark/ggui/internal/third_party/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/third_party/typesetting/font/opentype/tables"
)

// The glyph names read at the first ask are those the table holds, and the
// metrics are read with the font.
func TestPostNamesAreReadWhenAsked(t *testing.T) {
	for name, b := range glyfFonts(t) {
		f, ld := loadGlyfFont(t, b)
		raw, err := ld.RawTable(ot.MustNewTag("post"))
		if err != nil {
			continue
		}
		pst, _, err := tables.ParsePost(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want, err := newPost(pst)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if f.post.underlinePosition != want.underlinePosition || f.post.underlineThickness != want.underlineThickness || f.post.isFixedPitch != want.isFixedPitch {
			t.Fatalf("%s: metrics %+v, want %+v", name, f.post, want)
		}
		named := 0
		for gid := range GID(min(f.nGlyphs, 2000)) {
			got, w := f.post.names.glyphName(gid), ""
			if want.names != nil {
				w = want.names.glyphName(gid)
			}
			if got != w {
				t.Fatalf("%s: glyph %d named %q, want %q", name, gid, got, w)
			}
			if got != "" {
				named++
			}
		}
		t.Logf("%s: %d glyphs named", name, named)
	}
}
