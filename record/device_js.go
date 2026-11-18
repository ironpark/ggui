package record

import "errors"

// device brackets the drawing of a frame.
type device interface {
	frame(fn func() error) error
}

// openDevice fails: a page has no renderer to draw a tour with offscreen
// before it runs. Record a tour from a desktop build of the app.
func openDevice() (device, error) {
	return nil, errors.New("record: tours are drawn by a desktop build; the browser cannot draw one")
}
