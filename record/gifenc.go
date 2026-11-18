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

	idx, want, under []byte
	lzw              lzwEncoder
	parts            [3]encoded // the frame whole, and in two
	pending          *encoded   // the frame to write next
	centis           int        // how long it shows
	written          int
}

// encoded is a part of a frame, encoded.
type encoded struct {
	rect   image.Rectangle
	clear  int
	data   bytes.Buffer
	chosen []byte
}

// splitLead is how long the first part of a frame split in two shows,
// in hundredths of a second: browsers show a frame of less than 2 for
// 10, which would hold the animation up.
const splitLead = 2

// frame encodes the frame whose indices for rect are e.idx, shown for
// centis.
func (e *frameEncoder) frame(rect image.Rectangle, centis int, first bool) error {
	draw := rect
	opt := !first && e.level > 0
	if opt {
		draw = e.needed(rect, rect)
		if draw.Empty() {
			e.centis += centis
			return nil
		}
	}
	if err := e.flush(); err != nil {
		return err
	}
	clear := -1
	if opt {
		clear = e.spareIndex(rect, draw)
	}
	whole := &e.parts[0]
	if err := e.encode(whole, rect, draw, clear); err != nil {
		return err
	}
	if opt && e.level >= 2 && centis >= 2*splitLead {
		// Two rectangles far apart, as the cursor and a control it
		// changed, can cost less than the one around both: the first
		// shows for splitLead, the second for the rest.
		if a, b, ok := e.split(rect, draw); ok {
			pa, pb := &e.parts[1], &e.parts[2]
			if err := e.encode(pa, rect, a, clear); err != nil {
				return err
			}
			if err := e.encode(pb, rect, b, clear); err != nil {
				return err
			}
			const overhead = 20 // a graphic control, an image descriptor and their ends
			if pa.data.Len()+pb.data.Len()+overhead < whole.data.Len() {
				e.show(pa)
				e.pending, e.centis = pa, splitLead
				if err := e.flush(); err != nil {
					return err
				}
				e.show(pb)
				e.pending, e.centis = pb, centis-splitLead
				return nil
			}
		}
	}
	e.show(whole)
	e.pending, e.centis = whole, centis
	return nil
}

// encode encodes the part draw of the frame whose indices for rect are
// e.idx into p.
func (e *frameEncoder) encode(p *encoded, rect, draw image.Rectangle, clear int) error {
	n := draw.Dx() * draw.Dy()
	e.want = grow(e.want, n)
	e.under = grow(e.under, n)
	p.chosen = grow(p.chosen, n)
	i := 0
	for y := draw.Min.Y; y < draw.Max.Y; y++ {
		src := (y-rect.Min.Y)*rect.Dx() + draw.Min.X - rect.Min.X
		copy(e.want[i:i+draw.Dx()], e.idx[src:src+draw.Dx()])
		copy(e.under[i:i+draw.Dx()], e.screen[y*e.size.X+draw.Min.X:])
		i += draw.Dx()
	}
	fc := frameCodes{want: e.want, clear: clear, lossy: e.lossy, chosen: p.chosen}
	if clear >= 0 {
		fc.under = e.under
	}
	p.rect, p.clear = draw, clear
	p.data.Reset()
	return e.lzw.encode(&p.data, max(e.g.bits, 2), fc)
}

// show puts what p shows on the screen.
func (e *frameEncoder) show(p *encoded) {
	i := 0
	for y := p.rect.Min.Y; y < p.rect.Max.Y; y++ {
		row := e.screen[y*e.size.X:]
		for x := p.rect.Min.X; x < p.rect.Max.X; x++ {
			if c := p.chosen[i]; int(c) != p.clear {
				row[x] = c
			}
			i++
		}
	}
}

// needs reports whether the pixel at x, y, whose index for rect is in
// e.idx, is not shown already, nor near enough to it when lossy.
func (e *frameEncoder) needs(rect image.Rectangle, x, y int) bool {
	p := e.idx[(y-rect.Min.Y)*rect.Dx()+x-rect.Min.X]
	s := e.screen[y*e.size.X+x]
	return p != s && (e.lossy == nil || !e.lossy.ok[s][p])
}

// needed returns the smallest rectangle within in around the pixels
// that need drawing of the frame whose indices for rect are e.idx.
func (e *frameEncoder) needed(rect, in image.Rectangle) image.Rectangle {
	x0, y0, x1, y1 := in.Max.X, in.Max.Y, in.Min.X, in.Min.Y
	for y := in.Min.Y; y < in.Max.Y; y++ {
		for x := in.Min.X; x < in.Max.X; x++ {
			if e.needs(rect, x, y) {
				x0, x1 = min(x0, x), max(x1, x+1)
				y0, y1 = min(y0, y), max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// split cuts draw in two across the widest band of rows, or of columns,
// with nothing to draw, each part cut to what it needs, when the two are
// smaller than draw by a quarter.
func (e *frameEncoder) split(rect, draw image.Rectangle) (a, b image.Rectangle, ok bool) {
	band := func(lo, hi int, empty func(int) bool) (int, int) {
		start, n, run := -1, 0, -1
		for v := lo; v < hi; v++ {
			if !empty(v) {
				run = -1
				continue
			}
			if run < 0 {
				run = v
			}
			if v-run+1 > n {
				start, n = run, v-run+1
			}
		}
		return start, n
	}
	rows := func(y int) bool {
		for x := draw.Min.X; x < draw.Max.X; x++ {
			if e.needs(rect, x, y) {
				return false
			}
		}
		return true
	}
	cols := func(x int) bool {
		for y := draw.Min.Y; y < draw.Max.Y; y++ {
			if e.needs(rect, x, y) {
				return false
			}
		}
		return true
	}
	area := func(r image.Rectangle) int { return r.Dx() * r.Dy() }
	best := area(draw) * 3 / 4
	if y, n := band(draw.Min.Y, draw.Max.Y, rows); n > 0 {
		ta := e.needed(rect, image.Rect(draw.Min.X, draw.Min.Y, draw.Max.X, y))
		tb := e.needed(rect, image.Rect(draw.Min.X, y+n, draw.Max.X, draw.Max.Y))
		if s := area(ta) + area(tb); s < best {
			a, b, ok, best = ta, tb, true, s
		}
	}
	if x, n := band(draw.Min.X, draw.Max.X, cols); n > 0 {
		ta := e.needed(rect, image.Rect(draw.Min.X, draw.Min.Y, x, draw.Max.Y))
		tb := e.needed(rect, image.Rect(x+n, draw.Min.Y, draw.Max.X, draw.Max.Y))
		if s := area(ta) + area(tb); s < best {
			a, b, ok = ta, tb, true
		}
	}
	return a, b, ok
}

// spareIndex returns an index of the color table no pixel of the frame
// may be written with, to be its transparent one, or -1 when it has none
// to spare.
func (e *frameEncoder) spareIndex(rect, draw image.Rectangle) int {
	if e.npal < e.slots {
		return e.npal // past the palette: never a pixel's
	}
	var used [256]bool
	for y := draw.Min.Y; y < draw.Max.Y; y++ {
		row := e.idx[(y-rect.Min.Y)*rect.Dx():]
		for x := draw.Min.X; x < draw.Max.X; x++ {
			used[row[x-rect.Min.X]] = true
		}
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
	p := e.pending
	if p == nil {
		return nil
	}
	e.pending = nil
	e.written++
	return e.g.frame(p.rect, max(e.g.bits, 2), p.data.Bytes(), e.centis, p.clear)
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
