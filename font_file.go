package ggui

import (
	"encoding/binary"
	"io"
	"os"
	"sync"

	"github.com/ironpark/ggui/internal/text"
)

// Fonts of files are read where they are rather than into memory: the
// faces' metrics and coverage when loaded, their glyphs once text needs
// them, and a color emoji font's bitmaps a glyph at a time as they draw.
// What the platform ships runs to hundreds of megabytes, most of it never
// drawn.

// fontFile is one face to load: a file, and its face within a collection.
type fontFile struct {
	path  string
	index int
}

// openFontFiles are the font files open for reading, by path. They stay
// open while the program runs, since their fonts read them as they draw.
var openFontFiles = struct {
	sync.Mutex
	files map[string]openFile
}{files: map[string]openFile{}}

// openFile is a font file open for reading at offsets, and its size.
type openFile struct {
	r    io.ReaderAt
	size int64
}

// openFontFile opens the font file at path for reading at offsets, once.
// Where it can, it maps the file into memory: its pages are then the
// file's, read as they are touched and dropped under pressure, and the
// tables lent from them never count as the program's memory, as copies on
// the heap would.
func openFontFile(path string) (io.ReaderAt, int64, error) {
	openFontFiles.Lock()
	defer openFontFiles.Unlock()
	if o, ok := openFontFiles.files[path]; ok {
		return o.r, o.size, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	o := openFile{r: f, size: st.Size()}
	if m, ok := mapFontFile(f, o.size); ok {
		f.Close() // the mapping outlives it
		o.r = m
	}
	openFontFiles.files[path] = o
	return o.r, o.size, nil
}

// mappedFont is a font file mapped into memory, for the program's life.
type mappedFont []byte

// ReadAt implements io.ReaderAt.
func (m mappedFont) ReadAt(b []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(m)) {
		return 0, io.EOF
	}
	n := copy(b, m[off:])
	if n < len(b) {
		return n, io.EOF
	}
	return n, nil
}

// Slice lends the n bytes at off, capped so that appending to them cannot
// reach the bytes after.
func (m mappedFont) Slice(off, n int64) ([]byte, bool) {
	if off < 0 || n < 0 || off+n > int64(len(m)) {
		return nil, false
	}
	return m[off : off+n : off+n], true
}

// faceCount is how many faces the font file r holds: a collection's, or
// one.
func faceCount(r io.ReaderAt) (int, error) {
	head := make([]byte, 12)
	if _, err := r.ReadAt(head, 0); err != nil {
		return 0, err
	}
	if string(head[:4]) != "ttcf" {
		return 1, nil
	}
	return int(binary.BigEndian.Uint32(head[8:])), nil
}

// indexedFont is face file.index of the font file at file.path, of weight
// w as the system's index has it: its light source read when f is first
// asked about, and the rest when it first draws. A family's faces are
// many, and most are never chosen.
func indexedFont(file fontFile, w FontWeight) *Font {
	f := &Font{weight: w}
	f.lightLoad = func() *text.GoTextFaceSource {
		if r, size, err := openFontFile(file.path); err == nil {
			if light, err := text.NewGoTextFaceSourceAt(r, size, text.SourceOptions{Index: file.index, MetricsOnly: true}); err == nil {
				return light
			}
		}
		// The file is gone or changed since it was indexed. The built-in
		// font stands in rather than leave f with no source.
		return builtinFont().source()
	}
	f.load = func() *text.GoTextFaceSource {
		light := f.lightSource()
		if r, size, err := openFontFile(file.path); err == nil {
			if src, err := text.NewGoTextFaceSourceAt(r, size, text.SourceOptions{Index: file.index}); err == nil {
				return src
			}
		}
		return light // draws as little as the light source, as a font that failed to load
	}
	return f
}

// fileFont is face index of the font file r holds, of size bytes: its
// light source read now, and the rest when it first draws.
func fileFont(r io.ReaderAt, size int64, index int) (*Font, error) {
	light, err := text.NewGoTextFaceSourceAt(r, size, text.SourceOptions{Index: index, MetricsOnly: true})
	if err != nil {
		return nil, err
	}
	f := &Font{light: light, weight: FontWeight(light.Metadata().Weight)}
	f.load = func() *text.GoTextFaceSource {
		src, err := text.NewGoTextFaceSourceAt(r, size, text.SourceOptions{Index: index})
		if err != nil {
			return light // draws nothing, as a font that failed to load
		}
		return src
	}
	return f, nil
}
