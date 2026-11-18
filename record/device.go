//go:build !js

package record

import (
	"fmt"
	"sync"

	"github.com/ironpark/ggfx/host"

	"github.com/ironpark/ggui/internal/headless"
)

// device brackets the drawing of a frame: the renderer draws and reads
// pixels back only inside a frame of its own.
type device interface {
	frame(fn func() error) error
}

var (
	deviceMu sync.Mutex
	started  bool // the renderer is the one openDevice started
)

// openDevice returns the device a tour draws with. A process with no
// renderer yet gets one with no window, which stays up for the rest of the
// process, since the renderer initializes once; each frame of the tour is
// then a frame of its own. A renderer something else brought up is
// already inside a frame wherever a tour can be drawn, as a test run by
// ggtest.Main is: the tour draws in that one.
func openDevice() (device, error) {
	deviceMu.Lock()
	defer deviceMu.Unlock()
	if started {
		return ownFrames{}, nil
	}
	if headless.Running() {
		return currentFrame{}, nil
	}
	if _, err := headless.Start(); err != nil {
		return nil, fmt.Errorf("record: starting the renderer: %w", err)
	}
	started = true
	return ownFrames{}, nil
}

// ownFrames begins and ends a frame of the renderer around each draw.
type ownFrames struct{}

func (ownFrames) frame(fn func() error) error {
	f, err := host.BeginFrame()
	if err != nil {
		return err
	}
	ferr := fn()
	if err := f.End(); err != nil && ferr == nil {
		ferr = err
	}
	// The image and glyph caches expire by flushes: one a frame keeps
	// a long tour's in check.
	if _, err := host.Flush(false); err != nil && ferr == nil {
		ferr = err
	}
	return ferr
}

// currentFrame draws in the frame the caller is in.
type currentFrame struct{}

func (currentFrame) frame(fn func() error) error { return fn() }
