package ggui

import "github.com/ironpark/ggui/inspect"

// The inspector's public surface. It stays in every build so that
// Config.Inspector, App.Inspector and App.SetInspector compile and keep
// their signatures whether or not the ggui_inspector tag is set; the
// implementation behind them lives in the tagged inspector*.go files.

// OnInspect registers fn to receive every painted frame's inspect.Frame:
// the widgets as painted and the accessibility tree beside them, as the
// inspector's own panel sees them. It is for a viewer of one's own, such
// as one in another process fed over a connection. The frame is valid
// until fn returns and no longer, since the next frame reuses its buffers;
// a viewer that keeps it copies it, and one that leaves the process calls
// DescribeAll first. Like App.Inspector, this does nothing in a build
// without the ggui_inspector tag.
func (a *App) OnInspect(fn func(*inspect.Frame)) { a.insp.observe(fn) }

// InspectorDock is the edge the inspector's panel is docked to.
type InspectorDock uint8

const (
	InspectorBottom InspectorDock = iota // tree beside details on wide panels
	InspectorRight                       // tree above details
)

// InspectorOptions configures the inspector. The toolbar changes the same
// settings while it is open; dragging the panel edge adjusts its size.
// The zero value docks bottom and highlights only the selected widget.
type InspectorOptions struct {
	Dock         InspectorDock
	ShowOutlines bool
}
