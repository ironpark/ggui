// Package ggtest lets an application's tests see what its widgets paint.
//
// A Probe lays a tree out, runs its input and reads its semantics with no
// renderer at all; what it paints is drawn only inside a frame of the
// renderer, which an App's window runs and a test does not. Main gives a
// package's tests one, with no window: the renderer comes up headless,
// with an OpenGL context of the test's own where the platform needs one
// (macOS), or a surfaceless EGL one on Linux. Render then paints a probe's
// frame and reads it back.
//
//	func TestMain(m *testing.M) { ggtest.Main(m) }
//
//	func TestTheBadgeIsRed(t *testing.T) {
//		p := ggui.NewProbe(badge(), ggui.Sz(80, 24))
//		defer p.Close()
//		img := ggtest.Render(p, ggui.Sz(80, 24), 1)
//		// img.RGBAAt(40, 12) ...
//	}
//
// The tests then all run in that one frame, on the main thread: a test
// under Main does not call t.Parallel. A pixel can differ very slightly
// from one GPU to another; compare with a tolerance, not byte for byte.
package ggtest

import (
	"image"
	"math"
	"testing"

	"github.com/ironpark/ggfx"

	"github.com/ironpark/ggui"
	"github.com/ironpark/ggui/internal/gfxtest"
)

// Main runs a package's tests inside one frame of the renderer, with no
// window, and exits with their code. Call it from TestMain.
func Main(m *testing.M) { gfxtest.MainInFrame(m) }

// MainHeadless runs a package's tests with no renderer, for tests that
// paint a great many frames and read none: the draws a frame defers are
// dropped as they pile up. Render is not for its tests.
func MainHeadless(m *testing.M) { gfxtest.MainHeadless(m) }

// Render runs one frame of p at size, in logical pixels, painted at
// scale device pixels to one, and returns what it painted: size times
// scale, rounded up, in device pixels, its alpha premultiplied as
// image.RGBA's is. It is to be called in a test run by Main.
func Render(p *ggui.Probe, size ggui.Size, scale float64) *image.RGBA {
	if scale <= 0 {
		scale = 1
	}
	w, h := int(math.Ceil(size.W*scale)), int(math.Ceil(size.H*scale))
	img := ggfx.NewImage(max(w, 1), max(h, 1))
	defer img.Deallocate()
	p.SetScale(scale)
	p.Resize(size)
	p.Draw(img)
	out := image.NewRGBA(img.Bounds())
	img.ReadPixels(out.Pix)
	return out
}

// RenderWindow reads what p's window shows, after the frames it has run.
// It is to be called in a test run by Main.
func RenderWindow(p *ggui.WindowProbe) *image.RGBA {
	screen := p.Screen()
	out := image.NewRGBA(screen.Bounds())
	screen.ReadPixels(out.Pix)
	return out
}
