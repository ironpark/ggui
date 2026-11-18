package record

import (
	"bufio"
	"image"
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
