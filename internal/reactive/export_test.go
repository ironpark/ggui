package reactive

// Recording reports whether a layout recorder is installed. Only the
// tests ask; the frame loop takes the recorder itself with Recorder.
func Recording() bool { return current().measuring != nil }
