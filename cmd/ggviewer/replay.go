//go:build (unix && !ios && !android) || windows

package main

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"

	"github.com/ironpark/ggfx"
	ggfxremote "github.com/ironpark/ggfx/remote"
	"github.com/ironpark/ggwd"
	"github.com/ironpark/ggwd/remote"
)

var background = color.RGBA{0x1f, 0x1f, 0x23, 0xff}

// service frees the apps that went away and replays the shown app's
// batches. It runs inside a frame: a background frame, which comes
// whatever the window is doing, or the window's.
func (v *session) service(windowFrame bool) {
	v.mu.Lock()
	gone := v.gone
	v.gone = nil
	a := v.cur
	v.mu.Unlock()
	for _, g := range gone {
		v.freeApp(g)
	}
	v.replayPending(a, windowFrame)
	for _, p := range v.dumps {
		if err := v.dump(p); err != nil {
			log.Printf("dumping the window: %v", err)
		}
		v.events.send(event{Type: evDumped, Path: p})
	}
	v.dumps = nil
}

// freeApp frees one gone app: its replayer and its shared memory.
func (v *session) freeApp(g *app) {
	if r := v.replayers[g]; r != nil {
		images, shaders := r.Live()
		if err := r.Dispose(); err != nil {
			log.Printf("freeing app %d: %v", g.pid, err)
		}
		if v.opts.debug {
			li, ls := r.Live()
			log.Printf("app %d freed: %d images and %d shaders, %d and %d left", g.pid, images, shaders, li, ls)
		}
		delete(v.replayers, g)
	}
	g.mu.Lock()
	for _, b := range append(g.oldShm, g.shm) {
		if b != nil {
			_ = remote.Unmap(b)
		}
	}
	g.shm, g.oldShm, g.pending = nil, nil, nil
	g.mu.Unlock()
}

// replayPending replays the shown app's queued batches, if this frame should
// show them.
func (v *session) replayPending(a *app, windowFrame bool) {
	if a == nil || !(windowFrame || !v.opts.windowFramesOnly) {
		return
	}
	a.mu.Lock()
	pending, old := a.pending, a.oldShm
	a.pending, a.oldShm = nil, nil
	a.mu.Unlock()
	r := v.replayers[a]
	if r == nil && len(pending) > 0 {
		r = ggfxremote.NewReplayer()
		v.replayers[a] = r
	}
	for _, p := range pending {
		if !v.replay(a, r, p.shm, p.batch) {
			break
		}
	}
	for _, b := range old {
		_ = remote.Unmap(b)
	}
}

// replay replays one batch of a's into the screen image and acks it. It
// reports false when the app is dropped for a bad batch.
func (v *session) replay(a *app, r *ggfxremote.Replayer, shm []byte, b *remote.Batch) bool {
	drop := func(err error) bool {
		log.Printf("app %d's batch %d: %v; dropping the app", a.pid, b.Seq, err)
		_ = a.conn.Close()
		return false
	}
	if b.Offset < 0 || b.Length < 0 || b.Offset+b.Length > int64(len(shm)) {
		return drop(errors.New("outside its shared memory"))
	}
	batch := shm[b.Offset : b.Offset+b.Length]
	w, h, err := ggfxremote.ScreenSize(batch)
	if err != nil {
		return drop(err)
	}
	screen := v.screenFor(w, h)
	if b.Read != nil {
		return v.replayRead(a, r, batch, screen, b)
	}
	res, err := r.Replay(batch, screen)
	if err != nil {
		return drop(err)
	}
	_ = a.conn.Send(&remote.Ack{Seq: b.Seq})
	v.notePresented(a, res)
	return true
}

// screenFor returns the screen image for a frame of w by h, reallocating
// v.screen as the size changes.
func (v *session) screenFor(w, h int) *ggfx.Image {
	if w <= 0 || h <= 0 {
		return nil
	}
	if v.screen == nil || v.screen.Bounds().Size() != image.Pt(w, h) {
		if v.screen != nil {
			v.screen.Deallocate()
		}
		v.screen = ggfx.NewImageWithOptions(image.Rect(0, 0, w, h), &ggfx.NewImageOptions{Unmanaged: true})
	}
	return v.screen
}

// replayRead answers a batch's read of the screen's pixels.
func (v *session) replayRead(a *app, r *ggfxremote.Replayer, batch []byte, screen *ggfx.Image, b *remote.Batch) bool {
	dsts := make([][]byte, len(b.Read.Regions))
	for k, rect := range b.Read.Regions {
		dsts[k] = make([]byte, 4*max(rect.Dx(), 0)*max(rect.Dy(), 0))
	}
	reply := &remote.Pixels{Seq: b.Seq, Pixels: dsts}
	if err := r.ReadPixels(batch, screen, b.Read.Image, b.Read.Regions, dsts); err != nil {
		reply.Err = err.Error()
	}
	_ = a.conn.Send(reply)
	return true
}

// notePresented records the app's first presented frame, which takes the
// place of what the overlay said while it was coming.
func (v *session) notePresented(a *app, res ggfxremote.Result) {
	if !res.Presented || a.shown {
		return
	}
	a.shown = true
	v.mu.Lock()
	v.ov = overlay{}
	v.ovSerial++
	v.mu.Unlock()
	v.events.send(event{Type: evFirstFrame, PID: a.pid})
}

// compose draws the app's last frame and the overlay to dst.
func (v *session) compose(dst *ggfx.Image) {
	db := dst.Bounds()
	if v.screen == nil {
		dst.Fill(background)
	} else if sb := v.screen.Bounds(); sb.Size() == db.Size() {
		dst.DrawImage(v.screen, &ggfx.DrawImageOptions{Blend: ggfx.BlendCopy})
	} else {
		// The app has not caught up with a resize: letterbox its last frame.
		dst.Fill(background)
		s := min(float64(db.Dx())/float64(sb.Dx()), float64(db.Dy())/float64(sb.Dy()))
		op := &ggfx.DrawImageOptions{Filter: ggfx.FilterLinear}
		op.GeoM.Scale(s, s)
		op.GeoM.Translate((float64(db.Dx())-s*float64(sb.Dx()))/2, (float64(db.Dy())-s*float64(sb.Dy()))/2)
		dst.DrawImage(v.screen, op)
	}
	v.updateOverlay(db.Dx(), db.Dy())
	if v.ovImage != nil {
		op := &ggfx.DrawImageOptions{}
		op.GeoM.Translate(float64(v.ovLayout.rect.Min.X), float64(v.ovLayout.rect.Min.Y))
		dst.DrawImage(v.ovImage, op)
	}
}

// updateOverlay draws the overlay again if it or the window's size
// changed since it was drawn.
func (v *session) updateOverlay(w, h int) {
	v.mu.Lock()
	ov, serial := v.ov, v.ovSerial
	v.mu.Unlock()
	key := [4]float64{float64(serial), float64(w), float64(h), v.scale}
	if key == v.ovKey {
		return
	}
	v.ovKey = key
	img, layout := ov.render(w, h, v.scale)
	v.mu.Lock()
	if v.ovSerial == serial {
		v.ov.scroll = ov.scroll
	}
	v.mu.Unlock()
	v.ovLayout = layout
	if img == nil {
		if v.ovImage != nil {
			v.ovImage.Deallocate()
			v.ovImage = nil
		}
		return
	}
	if v.ovImage == nil || v.ovImage.Bounds().Size() != img.Rect.Size() {
		if v.ovImage != nil {
			v.ovImage.Deallocate()
		}
		v.ovImage = ggfx.NewImage(img.Rect.Dx(), img.Rect.Dy())
	}
	v.ovImage.WritePixels(img.Pix)
}

// scrollOverlay scrolls the overlay's output when the pointer is over it,
// and reports whether it did.
func (v *session) scrollOverlay(ev ggwd.ScrollEvent) bool {
	l := v.ovLayout
	p := image.Pt(int(v.mouse[0]*v.scale), int(v.mouse[1]*v.scale))
	if !l.scrollable() || !p.In(l.rect) || l.lineHeight == 0 {
		return false
	}
	v.scrolled -= ev.Y * ev.Line * v.scale / float64(l.lineHeight)
	n := int(v.scrolled)
	if n != 0 {
		v.scrolled -= float64(n)
		v.mu.Lock()
		v.ov.scroll = max(v.ov.scroll+n, 0)
		v.ovSerial++
		v.mu.Unlock()
		if w := v.win.Load(); w != nil {
			w.RequestFrame()
		}
	}
	return true
}

// dump writes what the window shows to a PNG at path.
func (v *session) dump(path string) error {
	size := v.pixels
	if size.X <= 0 || size.Y <= 0 {
		if v.screen == nil {
			return errors.New("the window has no size yet")
		}
		size = v.screen.Bounds().Size()
	}
	img := ggfx.NewImage(size.X, size.Y)
	defer img.Deallocate()
	v.compose(img)
	rgba := image.NewRGBA(image.Rectangle{Max: size})
	img.ReadPixels(rgba.Pix)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, rgba); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
