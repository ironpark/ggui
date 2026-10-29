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
	files map[string]*os.File
}{files: map[string]*os.File{}}

// openFontFile opens the font file at path for reading at offsets, once.
func openFontFile(path string) (io.ReaderAt, int64, error) {
	openFontFiles.Lock()
	defer openFontFiles.Unlock()
	f, ok := openFontFiles.files[path]
	if !ok {
		var err error
		if f, err = os.Open(path); err != nil {
			return nil, 0, err
		}
		openFontFiles.files[path] = f
	}
	st, err := f.Stat()
	if err != nil {
		return nil, 0, err
	}
	return f, st.Size(), nil
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
