package a11y

// Mode says when an App talks to the platform's accessibility
// API. The zero value waits for an assistive technology to attach and
// builds nothing at all until one does, so a frame costs nothing in the
// usual case where none is running.
type Mode uint8

const (
	// Auto turns the bridge on while an assistive technology
	// is attached, and off again when it detaches.
	Auto Mode = iota
	// Off never speaks to the platform, whatever is running.
	Off
	// Always keeps the bridge on, so that a tool which does
	// not announce itself -- Accessibility Inspector is one -- still sees
	// the tree. It costs a little work every frame.
	Always
)
