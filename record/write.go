package record

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"time"
)

// WriteGIF draws the tour and writes it to path as an animated GIF: one
// palette of up to 256 colors chosen for the whole recording, and each
// frame after the first only the rectangle that changed, so a tour that
// mostly holds still makes a small file. Frames that change nothing are
// merged into the one before. It is drawn in two passes, the frames
// spooled to a temporary file between them, so memory does not grow with
// the tour's length. Call it from the main goroutine (see the package
// documentation).
func (t Tour) WriteGIF(path string, opts ...Option) error {
	_, err := t.writeGIF(path, resolve(opts))
	return err
}

func (t Tour) writeGIF(path string, o *settings) (res result, err error) {
	w, h := t.pixels()
	s, err := newGIFSink(w, h, o, t.fps())
	if err != nil {
		return result{}, err
	}
	defer s.close()
	if err := t.play(o, s.add); err != nil {
		return result{}, err
	}
	f, err := os.Create(path)
	if err != nil {
		return result{}, err
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = cerr
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	return s.encode(f, o)
}

// WritePNGs draws the tour and writes its frames to dir, which it
// creates, as 0000.png, 0001.png and on, for a tool such as ffmpeg to make
// a video or a GIF of. The options FPS, Width and Background apply; the
// frames are as many a second as FPS keeps, or the tour's FPS. Call it
// from the main goroutine (see the package documentation).
func (t Tour) WritePNGs(dir string, opts ...Option) error {
	_, err := t.writePNGs(dir, resolve(opts))
	return err
}

func (t Tour) writePNGs(dir string, o *settings) (result, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result{}, err
	}
	w, h := t.pixels()
	pace := newPacer(t.fps(), o.fps)
	sc := newScaler(w, h, o.width)
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	res := result{w: w, h: h}
	if sc != nil {
		res.w, res.h = sc.dw, sc.dh
	}
	err := t.play(o, func(img *image.RGBA) error {
		if !pace.next() {
			return nil
		}
		if sc != nil {
			img = sc.scale(img)
		}
		name := filepath.Join(dir, fmt.Sprintf("%04d.png", res.frames))
		f, err := os.Create(name)
		if err != nil {
			return err
		}
		err = enc.Encode(f, img)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		res.frames++
		return err
	})
	if err != nil {
		return result{}, err
	}
	if res.frames == 0 {
		return result{}, errNoFrames
	}
	res.duration = time.Duration(res.frames) * time.Second / time.Duration(pace.out)
	return res, nil
}

// errNoFrames is returned for a tour of which no frame was kept.
var errNoFrames = errors.New("record: no frames were kept")
