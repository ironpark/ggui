// Copyright 2026 The Ebitengine Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package text

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"io"
	"math"

	"golang.org/x/image/tiff"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
)

// A font read through NewGoTextFaceSourceAt stays where it is, in a file
// or other io.ReaderAt, rather than in memory: the parser copies the
// tables it parses, and a large color emoji font's bitmaps, hundreds of
// megabytes of them, are read a glyph at a time as they are drawn.

// SourceOptions configures NewGoTextFaceSourceAt.
type SourceOptions struct {
	// Index is the face to read of a TTC or OTC collection; 0 for a font
	// that is not one.
	Index int

	// MetricsOnly reads only what a face's metrics, metadata, variation
	// axes and character coverage need, leaving out the glyph outlines,
	// bitmaps and the tables shaping reads. Such a source measures and
	// reports which runes it has, as a full one does, but draws nothing;
	// see NewLazyFace, which loads the full source when a glyph is needed.
	MetricsOnly bool
}

// tag reads a table tag from a font's table directory.
var (
	tagSbix = opentype.MustNewTag("sbix")
	tagTTCF = opentype.MustNewTag("ttcf")
)

// heavyTables are the tables a MetricsOnly source leaves out: glyph data,
// bitmaps, color glyphs, and what shaping reads. The horizontal and
// vertical metrics stay, since the parser reads the line metrics with them.
var heavyTables = []opentype.Tag{
	opentype.MustNewTag("glyf"), opentype.MustNewTag("loca"), opentype.MustNewTag("CFF "), opentype.MustNewTag("CFF2"),
	opentype.MustNewTag("gvar"), opentype.MustNewTag("HVAR"), opentype.MustNewTag("VVAR"), tagSbix, opentype.MustNewTag("CBDT"), opentype.MustNewTag("CBLC"),
	opentype.MustNewTag("EBDT"), opentype.MustNewTag("EBLC"), opentype.MustNewTag("bdat"), opentype.MustNewTag("bloc"),
	opentype.MustNewTag("SVG "), opentype.MustNewTag("COLR"), opentype.MustNewTag("CPAL"), opentype.MustNewTag("GSUB"),
	opentype.MustNewTag("GPOS"), opentype.MustNewTag("GDEF"), opentype.MustNewTag("morx"), opentype.MustNewTag("kerx"),
	opentype.MustNewTag("kern"), opentype.MustNewTag("ankr"), opentype.MustNewTag("trak"), opentype.MustNewTag("feat"),
	opentype.MustNewTag("post"),
}

// NewGoTextFaceSourceAt parses one face of the OpenType or TrueType font,
// or font collection, in the size bytes r holds, reading r only for the
// tables it needs. A face with an 'sbix' table, as Apple's color emoji
// font has, reads each glyph's bitmap from r as it is drawn, so r must stay
// readable for as long as the source is used. r is read at offsets only,
// so one r may serve several sources at once.
func NewGoTextFaceSourceAt(r io.ReaderAt, size int64, opts SourceOptions) (*GoTextFaceSource, error) {
	dir, err := faceDirectory(r, size, opts.Index)
	if err != nil {
		return nil, err
	}
	hide := []opentype.Tag{tagSbix}
	if opts.MetricsOnly {
		hide = heavyTables
	}
	patched := &patchedReaderAt{r: r}
	var sbixRecord *tableRecord
	for i := range dir.records {
		rec := &dir.records[i]
		if rec.tag == tagSbix {
			sbixRecord = rec
		}
		for _, t := range hide {
			if rec.tag == t {
				// A tag no parser asks for hides the table it names.
				patched.patch(rec.tagOffset, []byte{'~', byte('a' + i%26), byte('a' + i/26%26), '~'})
				break
			}
		}
	}
	res := io.NewSectionReader(patched, 0, size)
	var ld *opentype.Loader
	if dir.collection {
		lds, err := opentype.NewLoaders(res)
		if err != nil {
			return nil, err
		}
		if opts.Index >= len(lds) {
			return nil, fmt.Errorf("text: the collection has %d faces, not %d", len(lds), opts.Index+1)
		}
		ld = lds[opts.Index]
	} else if ld, err = opentype.NewLoader(res); err != nil {
		return nil, err
	}
	f, err := font.NewFont(ld)
	if err != nil {
		return nil, err
	}
	s := newGoTextFaceSource(font.NewFace(f), ld)
	if sbixRecord != nil && !opts.MetricsOnly {
		if sb, err := newLazySbix(r, *sbixRecord, ld); err == nil {
			s.sbix = sb
		}
	}
	return s, nil
}

// faceDir is a face's table directory.
type faceDir struct {
	collection bool
	records    []tableRecord
}

// tableRecord is a table directory entry.
type tableRecord struct {
	tag            opentype.Tag
	tagOffset      int64 // where the record's tag is in the file
	offset, length uint32
}

// faceDirectory reads the table directory of face index of the font or
// collection r holds.
func faceDirectory(r io.ReaderAt, size int64, index int) (faceDir, error) {
	var dir faceDir
	head := make([]byte, 12)
	if _, err := r.ReadAt(head, 0); err != nil {
		return dir, fmt.Errorf("text: reading the font header: %w", err)
	}
	start := int64(0)
	if opentype.Tag(binary.BigEndian.Uint32(head)) == tagTTCF {
		dir.collection = true
		n := int(binary.BigEndian.Uint32(head[8:]))
		if index < 0 || index >= n {
			return dir, fmt.Errorf("text: the collection has %d faces, not %d", n, index+1)
		}
		off := make([]byte, 4)
		if _, err := r.ReadAt(off, 12+4*int64(index)); err != nil {
			return dir, fmt.Errorf("text: reading the collection: %w", err)
		}
		start = int64(binary.BigEndian.Uint32(off))
		if _, err := r.ReadAt(head, start); err != nil {
			return dir, fmt.Errorf("text: reading a face header: %w", err)
		}
	}
	n := int(binary.BigEndian.Uint16(head[4:]))
	recs := make([]byte, 16*n)
	if start+12+int64(len(recs)) > size {
		return dir, errors.New("text: the table directory runs past the end of the font")
	}
	if _, err := r.ReadAt(recs, start+12); err != nil {
		return dir, fmt.Errorf("text: reading the table directory: %w", err)
	}
	for i := range n {
		b := recs[16*i:]
		dir.records = append(dir.records, tableRecord{
			tag:       opentype.Tag(binary.BigEndian.Uint32(b)),
			tagOffset: start + 12 + 16*int64(i),
			offset:    binary.BigEndian.Uint32(b[8:]),
			length:    binary.BigEndian.Uint32(b[12:]),
		})
	}
	return dir, nil
}

// patchedReaderAt is r with some of its bytes replaced.
type patchedReaderAt struct {
	r       io.ReaderAt
	patches []bytePatch
}

type bytePatch struct {
	at   int64
	data []byte
}

func (p *patchedReaderAt) patch(at int64, data []byte) {
	p.patches = append(p.patches, bytePatch{at, data})
}

func (p *patchedReaderAt) ReadAt(b []byte, off int64) (int, error) {
	n, err := p.r.ReadAt(b, off)
	for _, pt := range p.patches {
		lo, hi := max(pt.at, off), min(pt.at+int64(len(pt.data)), off+int64(n))
		if lo < hi {
			copy(b[lo-off:hi-off], pt.data[lo-pt.at:hi-pt.at])
		}
	}
	return n, err
}

// lazySbix is an 'sbix' table read a glyph at a time. It answers what the
// parser's own table would: the strikes' sizes, and a glyph's extents and
// bitmap at a strike, chosen as the parser chooses one.
type lazySbix struct {
	r         io.ReaderAt
	offset    int64 // the table's, in r
	length    int64
	numGlyphs int
	strikes   []lazyStrike
	sizes     []font.BitmapSize
	upem      uint16
}

type lazyStrike struct {
	ppem   uint16
	offset int64 // from the table's start
}

func newLazySbix(r io.ReaderAt, rec tableRecord, ld *opentype.Loader) (*lazySbix, error) {
	sb := &lazySbix{r: r, offset: int64(rec.offset), length: int64(rec.length)}
	raw, err := ld.RawTable(opentype.MustNewTag("maxp"))
	if err != nil {
		return nil, err
	}
	maxp, _, err := tables.ParseMaxp(raw)
	if err != nil {
		return nil, err
	}
	sb.numGlyphs = int(maxp.NumGlyphs)
	head := make([]byte, 8)
	if err := sb.read(head, 0); err != nil {
		return nil, err
	}
	n := int(binary.BigEndian.Uint32(head[4:]))
	offs := make([]byte, 4*n)
	if err := sb.read(offs, 8); err != nil {
		return nil, err
	}
	for i := range n {
		off := int64(binary.BigEndian.Uint32(offs[4*i:]))
		ppem := make([]byte, 2)
		if err := sb.read(ppem, off); err != nil {
			return nil, err
		}
		sb.strikes = append(sb.strikes, lazyStrike{ppem: binary.BigEndian.Uint16(ppem), offset: off})
	}

	// The strikes' sizes, as the parser reports them for an 'sbix' table.
	upem, avgWidth := uint16(1), uint16(1)
	if raw, err := ld.RawTable(opentype.MustNewTag("head")); err == nil && len(raw) >= 20 {
		upem = binary.BigEndian.Uint16(raw[18:])
	}
	if raw, err := ld.RawTable(opentype.MustNewTag("OS/2")); err == nil && len(raw) >= 4 && upem != 0 {
		avgWidth = uint16(int16(binary.BigEndian.Uint16(raw[2:])))
	} else {
		upem = 1
	}
	sb.upem = upem
	if raw, err := ld.RawTable(opentype.MustNewTag("hhea")); err == nil {
		if hhea, _, err := tables.ParseHhea(raw); err == nil {
			for _, st := range sb.strikes {
				if st.ppem == 0 {
					continue
				}
				bs := font.BitmapSize{XPpem: st.ppem, YPpem: st.ppem}
				bs.Height = uint16(uint32(uint16(hhea.Ascender-hhea.Descender+hhea.LineGap)) * uint32(st.ppem) / uint32(max(upem, 1)))
				bs.Width = uint16((uint32(avgWidth)*uint32(st.ppem) + uint32(upem/2)) / uint32(max(upem, 1)))
				sb.sizes = append(sb.sizes, bs)
			}
		}
	}
	return sb, nil
}

func (sb *lazySbix) read(b []byte, off int64) error {
	if off < 0 || off+int64(len(b)) > sb.length {
		return errors.New("text: 'sbix' data runs past the table")
	}
	_, err := sb.r.ReadAt(b, sb.offset+off)
	return err
}

// strike chooses the strike for a resolution as the parser does: the
// smallest not smaller than it, or else the largest; the largest for none.
func (sb *lazySbix) strike(xPpem, yPpem uint16) *lazyStrike {
	if len(sb.strikes) == 0 {
		return nil
	}
	request := max(xPpem, yPpem)
	if request == 0 {
		request = math.MaxUint16
	}
	best := 0
	bestPpem := sb.strikes[0].ppem
	for i, s := range sb.strikes {
		if request <= s.ppem && s.ppem < bestPpem || request > bestPpem && s.ppem > bestPpem {
			best, bestPpem = i, s.ppem
		}
	}
	return &sb.strikes[best]
}

// sbixGlyph is a glyph's data in a strike.
type sbixGlyph struct {
	originX, originY int16
	graphic          opentype.Tag
	data             []byte
}

var (
	tagDupe = opentype.MustNewTag("dupe")
	tagPNG  = opentype.MustNewTag("png ")
	tagTIFF = opentype.MustNewTag("tiff")
	tagJPG  = opentype.MustNewTag("jpg ")
)

// glyph reads gid's data in st, following 'dupe' glyphs as the parser does.
func (sb *lazySbix) glyph(st *lazyStrike, gid font.GID) (sbixGlyph, bool) {
	for depth := 0; depth <= 8; depth++ {
		if int(gid) >= sb.numGlyphs {
			return sbixGlyph{}, false
		}
		offs := make([]byte, 8)
		if sb.read(offs, st.offset+4+4*int64(gid)) != nil {
			return sbixGlyph{}, false
		}
		start, end := int64(binary.BigEndian.Uint32(offs)), int64(binary.BigEndian.Uint32(offs[4:]))
		if end-start < 8 {
			return sbixGlyph{}, false
		}
		b := make([]byte, end-start)
		if sb.read(b, st.offset+start) != nil {
			return sbixGlyph{}, false
		}
		g := sbixGlyph{
			originX: int16(binary.BigEndian.Uint16(b)),
			originY: int16(binary.BigEndian.Uint16(b[2:])),
			graphic: opentype.Tag(binary.BigEndian.Uint32(b[4:])),
			data:    b[8:],
		}
		if g.graphic != tagDupe {
			return g, true
		}
		if len(g.data) < 2 {
			return sbixGlyph{}, false
		}
		gid = font.GID(binary.BigEndian.Uint16(g.data))
	}
	return sbixGlyph{}, false
}

// config is a glyph bitmap's size and format.
func (g sbixGlyph) config() (width, height int, format font.BitmapFormat, err error) {
	switch g.graphic {
	case tagPNG:
		c, err := png.DecodeConfig(bytes.NewReader(g.data))
		return c.Width, c.Height, font.PNG, err
	case tagTIFF:
		c, err := tiff.DecodeConfig(bytes.NewReader(g.data))
		return c.Width, c.Height, font.TIFF, err
	case tagJPG:
		c, err := jpeg.DecodeConfig(bytes.NewReader(g.data))
		return c.Width, c.Height, font.JPG, err
	}
	return 0, 0, 0, fmt.Errorf("text: unsupported graphic type in sbix table: %s", g.graphic)
}

// extents is gid's extents at the strike for a resolution, in font units.
func (sb *lazySbix) extents(gid font.GID, xPpem, yPpem uint16) (font.GlyphExtents, bool) {
	st := sb.strike(xPpem, yPpem)
	if st == nil || st.ppem == 0 {
		return font.GlyphExtents{}, false
	}
	g, ok := sb.glyph(st, gid)
	if !ok {
		return font.GlyphExtents{}, false
	}
	w, h, _, err := g.config()
	if err != nil {
		return font.GlyphExtents{}, false
	}
	scale := float32(sb.upem) / float32(st.ppem)
	return font.GlyphExtents{
		XBearing: float32(g.originX) * scale,
		YBearing: (float32(h) + float32(g.originY)) * scale,
		Width:    float32(w) * scale,
		Height:   -float32(h) * scale,
	}, true
}

// bitmap is gid's bitmap at the strike for a resolution.
func (sb *lazySbix) bitmap(gid font.GID, xPpem, yPpem uint16) (font.GlyphBitmap, bool) {
	st := sb.strike(xPpem, yPpem)
	if st == nil {
		return font.GlyphBitmap{}, false
	}
	g, ok := sb.glyph(st, gid)
	if !ok {
		return font.GlyphBitmap{}, false
	}
	out := font.GlyphBitmap{Data: g.data}
	var err error
	if out.Width, out.Height, out.Format, err = g.config(); err != nil {
		return font.GlyphBitmap{}, false
	}
	return out, true
}
