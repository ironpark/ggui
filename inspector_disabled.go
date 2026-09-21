//go:build !ggui_inspector

package ggui

// The inspector is a development tool: 1,800 lines of tree, panel and
// overlay painting, plus its own monospaced font, that a shipped app never
// runs. Build with -tags ggui_inspector to get it; without the tag this
// stub takes its place, App.Inspector cannot turn it on, and none of it
// reaches the binary. See inspector.go for the real one.

// inspectorEnabled reports whether this build contains the inspector.
const inspectorEnabled = false

// inspector is the no-op stand-in. Its methods mirror the ones App calls,
// so app.go needs no build tag of its own.
type inspector struct {
	closed bool
}

func (in *inspector) hide()                            {}
func (in *inspector) release()                         {}
func (in *inspector) reset()                           {}
func (in *inspector) apply(InspectorOptions)           {}
func (in *inspector) paint(*Canvas)                    {}
func (in *inspector) input(frameInput) bool            { return false }
func (in *inspector) cursor(Point) (CursorShape, bool) { return 0, false }
