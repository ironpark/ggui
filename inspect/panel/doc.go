// Package panel is the widget inspector's panel: an Overlay in the shape
// of a browser's element panel, showing the frames of the inspect package
// that App.OnInspect hands out. Importing it registers it, so that
// App.Inspector and the key Config.Inspector binds turn it on:
//
//	import _ "github.com/ironpark/ggui/inspect/panel"
//
// It ships only in builds tagged ggui_inspector; without the tag the
// package is empty and App.Inspector does nothing, so a release binary
// carries none of it. App.OnInspect needs neither the import nor the tag.
package panel
