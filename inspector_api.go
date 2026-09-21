package ggui

// The inspector's public surface. It stays in every build so that
// Config.Inspector, App.Inspector and App.SetInspector compile and keep
// their signatures whether or not the ggui_inspector tag is set; the
// implementation behind them lives in the tagged inspector*.go files.

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
