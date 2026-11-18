// Package headless brings the renderer up with no window: what a test
// that reads pixels runs in (see ggtest.Main), and what package record
// draws a tour with. The renderer then draws to images alone, which are
// read back inside a frame begun with host.BeginFrame.
package headless

import (
	"fmt"
	"runtime"

	"github.com/ironpark/ggfx/host"
)

func init() {
	// The renderer is initialized on the main thread, which init runs on.
	runtime.LockOSThread()
}

// Start initializes the renderer with no window, on the calling
// goroutine, which is the main one: OpenGL draws in a context of its own
// where the platform has one to make (a CGL context on macOS, a
// surfaceless EGL one on Linux), and the other drivers need none. It can
// run once in a process, since the renderer initializes once. stop waits
// for the queued commands to run and lets the renderer and the context go.
func Start() (stop func(), err error) {
	if err := host.Init(nil); err != nil {
		return nil, err
	}
	var closeContext func()
	stop = func() {
		host.Terminate()
		if closeContext != nil {
			closeContext()
		}
	}
	if err := newCanvasSurface(); err != nil {
		stop()
		return nil, err
	}
	if host.CurrentDriver().Library == host.GraphicsLibraryOpenGL && runtime.GOOS != "js" {
		c, err := newGLContext()
		if err != nil {
			stop()
			return nil, fmt.Errorf("OpenGL needs a context: %w", err)
		}
		if closer, ok := any(c).(interface{ Close() }); ok {
			closeContext = closer.Close
		}
		host.SetGLContext(c)
	}
	return stop, nil
}

// Running reports whether the renderer was initialized already, by a
// window system's loop, a test's Main or Start.
func Running() bool {
	// Before Init the library reads as Auto, though ggfx documents
	// Unknown; neither is a driver.
	lib := host.CurrentDriver().Library
	return lib != host.GraphicsLibraryAuto && lib != host.GraphicsLibraryUnknown
}
