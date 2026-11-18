package record

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"io"
	"time"
)

// encode is the second pass: it chooses the palette and writes the GIF
// to w, each frame drawn over the frame before.
//
// At Optimize level 1, the default, a frame keeps only the pixels whose
// palette index changes, within the smallest rectangle around them; the
// others in it are written as a transparent index, which leaves the
// frame before showing, when the frame has an index to spare, and
// whichever of the two continues the LZW string being matched. A frame
// whose indices do not change at all, as when what changed maps to the
// same palette color, lengthens the one before. What the GIF shows is
// the same as at level 0, which writes each changed rectangle whole.
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
	e := &frameEncoder{
		g:      g,
		size:   size,
		screen: make([]byte, size.X*size.Y),
		level:  o.optimizeLevel(),
		slots:  1 << g.bits,
		npal:   len(pal),
	}
	if o.lossy > 0 {
		e.lossy = newNearness(pal, o.lossy)
	}
	in := bufio.NewReaderSize(s.spool, 1<<20)
	var rgb []byte
	res := result{w: size.X, h: size.Y}
	for k, f := range s.frames {
		n := f.rect.Dx() * f.rect.Dy()
		rgb = grow(rgb, 3*n)
		e.idx = grow(e.idx, n)
		if _, err := io.ReadFull(in, rgb); err != nil {
			return result{}, fmt.Errorf("record: reading the frames back: %w", err)
		}
		i := 0
		for y := f.rect.Min.Y; y < f.rect.Max.Y; y++ {
			for x := f.rect.Min.X; x < f.rect.Max.X; x++ {
				e.idx[i] = m.index(rgb[3*i], rgb[3*i+1], rgb[3*i+2], x, y)
				i++
			}
		}
		if err := e.frame(f.rect, f.centis, k == 0); err != nil {
			return result{}, err
		}
		res.duration += time.Duration(f.centis) * 10 * time.Millisecond
	}
	if err := e.flush(); err != nil {
		return result{}, err
	}
	res.frames = e.written
	return res, g.close()
}

// optimizeLevel returns the Optimize level asked for, 1 by default.
func (o *settings) optimizeLevel() int {
	if o.optimize == nil {
		return 1
	}
	return *o.optimize
}

// frameEncoder writes the frames of a GIF as they come, one behind: a
// frame is written when the next is known to change something, so that
// one that changes nothing lengthens it instead.
type frameEncoder struct {
	g      *gifWriter
	size   image.Point
	screen []byte // the index each pixel of the GIF shows so far
	level  int
	lossy  *nearness
	slots  int // the color table's size, 1<<bits
	npal   int // the colors in the palette

	idx, want, under, chosen []byte
	lzw                      lzwEncoder
	data                     bytes.Buffer
	pending                  bool
	rect                     image.Rectangle
	clear                    int
	centis                   int
	written                  int
}

// frame encodes the frame whose indices for rect are e.idx, shown for
// centis.
func (e *frameEncoder) frame(rect image.Rectangle, centis int, first bool) error {
	draw := rect
	clear := -1
	if !first && e.level > 0 {
		draw = e.needed(rect)
		if draw.Empty() {
			e.centis += centis
			return nil
		}
	}
	if err := e.flush(); err != nil {
		return err
	}
	n := draw.Dx() * draw.Dy()
	e.want = grow(e.want, n)
	e.under = grow(e.under, n)
	e.chosen = grow(e.chosen, n)
	i := 0
	for y := draw.Min.Y; y < draw.Max.Y; y++ {
		src := (y-rect.Min.Y)*rect.Dx() + draw.Min.X - rect.Min.X
		copy(e.want[i:i+draw.Dx()], e.idx[src:src+draw.Dx()])
		copy(e.under[i:i+draw.Dx()], e.screen[y*e.size.X+draw.Min.X:])
		i += draw.Dx()
	}
	if !first && e.level > 0 {
		clear = e.spareIndex()
	}
	fc := frameCodes{want: e.want, clear: clear, lossy: e.lossy, chosen: e.chosen}
	if clear >= 0 {
		fc.under = e.under
	}
	e.data.Reset()
	if err := e.lzw.encode(&e.data, max(e.g.bits, 2), fc); err != nil {
		return err
	}
	// What the frame shows now.
	i = 0
	for y := draw.Min.Y; y < draw.Max.Y; y++ {
		row := e.screen[y*e.size.X:]
		for x := draw.Min.X; x < draw.Max.X; x++ {
			if c := e.chosen[i]; int(c) != clear {
				row[x] = c
			}
			i++
		}
	}
	e.pending, e.rect, e.clear, e.centis = true, draw, clear, centis
	return nil
}

// needed returns the smallest rectangle within rect around the pixels
// whose index is not shown already, or near enough to it when lossy.
func (e *frameEncoder) needed(rect image.Rectangle) image.Rectangle {
	x0, y0, x1, y1 := rect.Max.X, rect.Max.Y, rect.Min.X, rect.Min.Y
	i := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		row := e.screen[y*e.size.X:]
		for x := rect.Min.X; x < rect.Max.X; x++ {
			p, s := e.idx[i], row[x]
			i++
			if p == s || (e.lossy != nil && e.lossy.ok[s][p]) {
				continue
			}
			x0, x1 = min(x0, x), max(x1, x+1)
			y0, y1 = min(y0, y), max(y1, y+1)
		}
	}
	if x0 >= x1 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// spareIndex returns an index of the color table no pixel of the frame
// may be written with, to be its transparent one, or -1 when it has none
// to spare.
func (e *frameEncoder) spareIndex() int {
	if e.npal < e.slots {
		return e.npal // past the palette: never a pixel's
	}
	var used [256]bool
	for _, p := range e.want {
		used[p] = true
	}
	if e.lossy != nil {
		for p := range 256 {
			if used[p] {
				for _, q := range e.lossy.near[p] {
					used[q] = true
				}
			}
		}
	}
	for i := e.slots - 1; i >= 0; i-- {
		if !used[i] {
			return i
		}
	}
	return -1
}

// flush writes the frame encoded last.
func (e *frameEncoder) flush() error {
	if !e.pending {
		return nil
	}
	e.pending = false
	e.written++
	return e.g.frame(e.rect, max(e.g.bits, 2), e.data.Bytes(), e.centis, e.clear)
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
	w    *bufio.Writer
	bits int // the palette holds 1<<bits colors
	err  error
}

func newGIFWriter(w io.Writer, size image.Point, pal color.Palette, loop int) *gifWriter {
	g := &gifWriter{w: bufio.NewWriter(w), bits: 1}
	for 1<<g.bits < len(pal) {
		g.bits++
	}
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

// frame writes rect's LZW data, of lit-bit literals, shown for centis
// hundredths of a second over what the frame before left; the index
// clear, unless -1, is transparent.
func (g *gifWriter) frame(rect image.Rectangle, lit int, data []byte, centis, clear int) error {
	b := g.w
	// Graphic control: disposal 1, leave the frame in place; the delay;
	// the transparent index, if any.
	flags := byte(1 << 2)
	if clear >= 0 {
		flags |= 1
	}
	b.Write([]byte{0x21, 0xf9, 0x04, flags})
	le16(b, min(centis, 0xffff))
	b.Write([]byte{byte(max(clear, 0)), 0})
	// Image descriptor: no local color table, not interlaced.
	b.WriteByte(0x2c)
	le16(b, rect.Min.X)
	le16(b, rect.Min.Y)
	le16(b, rect.Dx())
	le16(b, rect.Dy())
	b.WriteByte(0)
	b.WriteByte(byte(lit))
	// The data in sub-blocks of at most 255 bytes.
	for len(data) > 0 {
		n := min(len(data), 255)
		b.WriteByte(byte(n))
		if _, err := b.Write(data[:n]); err != nil && g.err == nil {
			g.err = err
		}
		data = data[n:]
	}
	b.WriteByte(0) // the end of the image's data
	return g.err
}

func (g *gifWriter) close() error {
	g.w.WriteByte(0x3b)
	if err := g.w.Flush(); err != nil {
		return err
	}
	return g.err
}
