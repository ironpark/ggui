package record

import (
	"bufio"
	"compress/lzw"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"time"
)

// result is what a recording wrote.
type result struct {
	frames   int
	duration time.Duration
	w, h     int
}

// pacer picks the frames drawn at drawn a second that are kept at out a
// second: the one on screen at each 1/out of a second.
type pacer struct {
	drawn, out int
	i, k       int // frames drawn and kept so far
}

func newPacer(drawn, out int) *pacer {
	if out <= 0 || out > drawn {
		out = drawn
	}
	return &pacer{drawn: drawn, out: out}
}

// next reports whether the frame drawn next is kept: it is when it is on
// screen at the time of the next frame kept, k/out.
func (p *pacer) next() bool {
	end := p.i + 1 // the frame drawn is on screen until end/drawn
	p.i++
	if p.k*p.drawn < end*p.out {
		p.k++
		return true
	}
	return false
}

// centis returns the time at which kept frame k shows, in the hundredths
// of a second a GIF counts in, rounded so that the delays add up to the
// time drawn.
func (p *pacer) centis(k int) int { return (k*200 + p.out) / (2 * p.out) }

// changed returns the smallest rectangle outside which a and b, opaque
// images of one size, are the same; empty when they are the same.
func changed(a, b *image.RGBA) image.Rectangle {
	w, h := a.Rect.Dx(), a.Rect.Dy()
	row := func(y int) []byte { return a.Pix[y*a.Stride : y*a.Stride+w*4] }
	rowB := func(y int) []byte { return b.Pix[y*b.Stride : y*b.Stride+w*4] }
	same := func(y int) bool { return string(row(y)) == string(rowB(y)) }
	y0 := 0
	for y0 < h && same(y0) {
		y0++
	}
	if y0 == h {
		return image.Rectangle{}
	}
	y1 := h
	for same(y1 - 1) {
		y1--
	}
	x0, x1 := w, 0
	for y := y0; y < y1; y++ {
		ra, rb := row(y), rowB(y)
		for x := 0; x < x0; x++ {
			if ra[x*4] != rb[x*4] || ra[x*4+1] != rb[x*4+1] || ra[x*4+2] != rb[x*4+2] {
				x0 = x
				break
			}
		}
		for x := w - 1; x >= x1; x-- {
			if ra[x*4] != rb[x*4] || ra[x*4+1] != rb[x*4+1] || ra[x*4+2] != rb[x*4+2] {
				x1 = x + 1
				break
			}
		}
	}
	return image.Rect(x0, y0, x1, y1)
}

// gifSink is the first pass of a GIF: it keeps the frames the pacer
// picks, scaled, counts their colors for the palette, and spools the part
// of each that changed to a file, as RGB, merging a frame that changed
// nothing into the one before. Memory holds two frames, however long the
// tour.
type gifSink struct {
	pace   *pacer
	scale  *scaler
	prev   *image.RGBA
	hist   *histogram
	spool  *os.File
	buf    *bufio.Writer
	frames []spooled
}

// spooled is a frame in the spool: the rectangle that changed, whose
// pixels follow the previous frame's, and how long it shows.
type spooled struct {
	rect   image.Rectangle
	centis int
}

func newGIFSink(w, h int, o *settings, drawnFPS int) (*gifSink, error) {
	f, err := os.CreateTemp("", "ggui-record-*.rgb")
	if err != nil {
		return nil, err
	}
	// A GIF counts time in hundredths of a second, and browsers slow a
	// frame shown for less than two down to ten: 50 frames a second is
	// as many as a GIF plays.
	fps := o.fps
	if fps <= 0 || fps > 50 {
		fps = min(drawnFPS, 50)
	}
	s := &gifSink{
		pace:  newPacer(drawnFPS, fps),
		scale: newScaler(w, h, o.width),
		hist:  new(histogram),
		spool: f,
		buf:   bufio.NewWriterSize(f, 1<<20),
	}
	return s, nil
}

// close removes the spool.
func (s *gifSink) close() {
	s.spool.Close()
	os.Remove(s.spool.Name())
}

func (s *gifSink) add(img *image.RGBA) error {
	if !s.pace.next() {
		return nil
	}
	k := s.pace.k - 1
	delay := s.pace.centis(k+1) - s.pace.centis(k)
	if s.scale != nil {
		img = s.scale.scale(img)
	}
	var rect image.Rectangle
	if s.prev == nil {
		s.prev = image.NewRGBA(img.Rect)
		rect = img.Rect
	} else {
		rect = changed(s.prev, img)
		if rect.Empty() {
			s.frames[len(s.frames)-1].centis += delay
			return nil
		}
	}
	s.hist.add(img, rect)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		p := img.PixOffset(rect.Min.X, y)
		for range rect.Dx() {
			if _, err := s.buf.Write(img.Pix[p : p+3]); err != nil {
				return err
			}
			p += 4
		}
		copy(s.prev.Pix[p-rect.Dx()*4:p], img.Pix[p-rect.Dx()*4:p])
	}
	s.frames = append(s.frames, spooled{rect: rect, centis: delay})
	return nil
}

// encode is the second pass: it chooses the palette and writes the GIF
// to w, each frame the rectangle that changed, drawn over the frame
// before.
func (s *gifSink) encode(w io.Writer, o *settings) (result, error) {
	if len(s.frames) == 0 {
		return result{}, errNoFrames
	}
	if err := s.buf.Flush(); err != nil {
		return result{}, err
	}
	if _, err := s.spool.Seek(0, io.SeekStart); err != nil {
		return result{}, err
	}
	pal := s.hist.palette(o.colors)
	m := newMapper(pal, o.dither)
	size := s.prev.Rect.Size()
	g := newGIFWriter(w, size, pal, o.loop)
	in := bufio.NewReaderSize(s.spool, 1<<20)
	var rgb, idx []byte
	res := result{frames: len(s.frames), w: size.X, h: size.Y}
	for _, f := range s.frames {
		n := f.rect.Dx() * f.rect.Dy()
		rgb = grow(rgb, 3*n)
		idx = grow(idx, n)
		if _, err := io.ReadFull(in, rgb); err != nil {
			return result{}, fmt.Errorf("record: reading the frames back: %w", err)
		}
		i := 0
		for y := f.rect.Min.Y; y < f.rect.Max.Y; y++ {
			for x := f.rect.Min.X; x < f.rect.Max.X; x++ {
				idx[i] = m.index(rgb[3*i], rgb[3*i+1], rgb[3*i+2], x, y)
				i++
			}
		}
		g.frame(f.rect, idx, f.centis)
		res.duration += time.Duration(f.centis) * 10 * time.Millisecond
	}
	return res, g.close()
}

func grow(b []byte, n int) []byte {
	if cap(b) < n {
		return make([]byte, n)
	}
	return b[:n]
}

// gifWriter writes a GIF a frame at a time, as image/gif's encoder, which
// takes every frame at once, cannot: the frames of a long tour need not
// all be in memory. All frames share the global palette.
type gifWriter struct {
	w     *bufio.Writer
	bits  int // the palette holds 1<<bits colors
	block blockWriter
	err   error
}

func newGIFWriter(w io.Writer, size image.Point, pal color.Palette, loop int) *gifWriter {
	g := &gifWriter{w: bufio.NewWriter(w), bits: 1}
	for 1<<g.bits < len(pal) {
		g.bits++
	}
	g.block.w = g.w
	b := g.w
	b.WriteString("GIF89a")
	le16(b, size.X)
	le16(b, size.Y)
	// A global color table of 1<<bits colors, 8 bits a channel.
	b.WriteByte(0x80 | 0x70 | byte(g.bits-1))
	b.WriteByte(0) // background color index
	b.WriteByte(0) // pixel aspect ratio
	for i := range 1 << g.bits {
		var r, gr, bl uint32
		if i < len(pal) {
			r, gr, bl, _ = pal[i].RGBA()
		}
		b.Write([]byte{byte(r >> 8), byte(gr >> 8), byte(bl >> 8)})
	}
	// The NETSCAPE2.0 extension counts the times a GIF plays after the
	// first, 0 for ever; a GIF without one plays once.
	if loop != 1 {
		b.Write([]byte{0x21, 0xff, 0x0b})
		b.WriteString("NETSCAPE2.0")
		b.Write([]byte{0x03, 0x01})
		le16(b, max(loop-1, 0))
		b.WriteByte(0)
	}
	return g
}

func le16(b *bufio.Writer, v int) {
	b.Write([]byte{byte(v), byte(v >> 8)})
}

// frame writes the indices of rect, a row at a time, shown for centis
// hundredths of a second, over what the frame before left.
func (g *gifWriter) frame(rect image.Rectangle, idx []byte, centis int) {
	if g.err != nil {
		return
	}
	b := g.w
	// Graphic control: disposal 1, leave the frame in place; the delay;
	// no transparent color.
	b.Write([]byte{0x21, 0xf9, 0x04, 1 << 2})
	le16(b, min(centis, 0xffff))
	b.Write([]byte{0, 0})
	// Image descriptor: no local color table, not interlaced.
	b.WriteByte(0x2c)
	le16(b, rect.Min.X)
	le16(b, rect.Min.Y)
	le16(b, rect.Dx())
	le16(b, rect.Dy())
	b.WriteByte(0)
	lit := max(g.bits, 2)
	b.WriteByte(byte(lit))
	lz := lzw.NewWriter(&g.block, lzw.LSB, lit)
	if _, err := lz.Write(idx); err != nil {
		g.err = err
		return
	}
	if err := lz.Close(); err != nil {
		g.err = err
		return
	}
	g.block.flush()
	b.WriteByte(0) // the end of the image's data
}

func (g *gifWriter) close() error {
	g.w.WriteByte(0x3b)
	if err := g.w.Flush(); err != nil {
		return err
	}
	return errors.Join(g.err, g.block.err)
}

// blockWriter cuts the LZW stream into the sub-blocks of at most 255
// bytes a GIF holds it in.
type blockWriter struct {
	w   *bufio.Writer
	buf [255]byte
	n   int
	err error
}

func (b *blockWriter) Write(p []byte) (int, error) {
	for _, c := range p {
		b.buf[b.n] = c
		b.n++
		if b.n == len(b.buf) {
			b.flush()
		}
	}
	return len(p), b.err
}

func (b *blockWriter) flush() {
	if b.n == 0 {
		return
	}
	b.w.WriteByte(byte(b.n))
	if _, err := b.w.Write(b.buf[:b.n]); err != nil && b.err == nil {
		b.err = err
	}
	b.n = 0
}
