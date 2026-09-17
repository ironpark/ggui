//go:build !darwin || ios

package ggui

// newAXPlatform returns nothing: there is no accessibility bridge on this
// platform yet, so App.Semantics publishes a tree every frame and nobody
// reads it. The bridge above does nothing at all when this is nil, which is
// what keeps the cost of the whole feature at zero here.
func newAXPlatform(*App) axPlatform { return nil }

// axAttach has nowhere to put the bridge, since there is no platform half
// to answer from.
func axAttach(*axBridge) {}
