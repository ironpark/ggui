package main

import (
	"sync"

	"github.com/ironpark/ggui/ui/icons"
)

// previewGlyphs are the preview's decorative icons: the body of a 24 pixel
// outline SVG each, kept here since the bundled sets carry only the roles
// the controls need. They are tinted and scaled as the control icons are.
var previewGlyphs = map[string]string{
	"coffee":       `<path d="M4 8h12v8a4 4 0 0 1-4 4H8a4 4 0 0 1-4-4zM16 9h2a3 3 0 0 1 0 6h-2M7 3v2m4-2v2m4-2v2"/>`,
	"cart":         `<path d="M3 3h2l3 13h11l2-9H6M9 20h.01M18 20h.01"/>`,
	"wallet":       `<rect x="3" y="6" width="18" height="14" rx="2"/><path d="M3 9V5l14-2v3m0 7h4m-4 0v3h4"/>`,
	"car":          `<path d="m5 9 2-5h10l2 5M3 10h18v9H3zM6 14h2m8 0h2M5 19v2m14-2v2"/>`,
	"tv":           `<rect x="3" y="7" width="18" height="14" rx="2"/><path d="m8 2 4 5 4-5"/>`,
	"figma":        `<path d="M8 3h8v18H8zM8 9h8M8 15h8"/>`,
	"music":        `<path d="M9 18V5l11-2v13M9 8l11-2"/><circle cx="6" cy="18" r="3"/><circle cx="17" cy="16" r="3"/>`,
	"dashboard":    `<rect x="3" y="3" width="7" height="9" rx="1"/><rect x="14" y="3" width="7" height="5" rx="1"/><rect x="14" y="12" width="7" height="9" rx="1"/><rect x="3" y="16" width="7" height="5" rx="1"/>`,
	"transfer":     `<path d="M8 3 4 7l4 4M4 7h16M16 21l4-4-4-4M20 17H4"/>`,
	"trending":     `<path d="M16 7h6v6M22 7l-8.5 8.5-5-5L2 17"/>`,
	"bank":         `<path d="M3 22h18M6 18v-7M10 18v-7M14 18v-7M18 18v-7M12 2l8 5H4z"/>`,
	"pie":          `<path d="M21 12A9 9 0 0 0 12 3v9z"/><path d="M20.2 16A9 9 0 1 1 8 3.8"/>`,
	"target":       `<circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/>`,
	"report":       `<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7zM14 2v5h6M8 18v-2M12 18v-4M16 18v-6"/>`,
	"document":     `<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7zM14 2v5h6M16 13H8M16 17H8M10 9H8"/>`,
	"user":         `<path d="M19 21v-2a4 4 0 0 0-4-4H9a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/>`,
	"card":         `<rect x="2" y="5" width="20" height="14" rx="2"/><path d="M2 10h20"/>`,
	"bell":         `<path d="M10.3 21a2 2 0 0 0 3.4 0M6 8a6 6 0 0 1 12 0c0 4.5 1.4 6 2.7 7.3A1 1 0 0 1 20 17H4a1 1 0 0 1-.7-1.7C4.6 14 6 12.5 6 8"/>`,
	"shield":       `<path d="M20 13c0 5-3.5 7.5-8 9-4.5-1.5-8-4-8-9V5l8-3 8 3z"/>`,
	"brush":        `<path d="m14.6 12.9 6.1-6.1a2.1 2.1 0 0 0-3-3l-6.1 6.1M7 15a3 3 0 0 0-3 3c0 1.3-2 1.5-2 2 1 1.1 2.500 2 4 2a4 4 0 0 0 4-4 3 3 0 0 0-3-3"/>`,
	"help":         `<circle cx="12" cy="12" r="10"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 3-3 3M12 17h.01"/>`,
	"message":      `<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`,
	"book":         `<path d="M12 7v14M2 4h6a4 4 0 0 1 4 4 4 4 0 0 1 4-4h6v14h-7a3 3 0 0 0-3 3 3 3 0 0 0-3-3H2z"/>`,
	"activity":     `<path d="M22 12h-4l-3 9L9 3l-3 9H2"/>`,
	"gauge":        `<path d="m12 14 4-4M3.3 19a10 10 0 1 1 17.4 0"/>`,
	"calendar":     `<rect x="3" y="4" width="18" height="18" rx="2"/><path d="M8 2v4M16 2v4M3 10h18"/>`,
	"repeat":       `<path d="m17 2 4 4-4 4M3 11v-1a4 4 0 0 1 4-4h14M7 22l-4-4 4-4M21 13v1a4 4 0 0 1-4 4H3"/>`,
	"refresh":      `<path d="M3 12a9 9 0 0 1 15.7-6.3L21 8M21 3v5h-5M21 12a9 9 0 0 1-15.7 6.3L3 16M8 16H3v5"/>`,
	"lock":         `<rect x="3" y="11" width="18" height="11" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>`,
	"arrow-right":  `<path d="M5 12h14M12 5l7 7-7 7"/>`,
	"image":        `<rect x="3" y="3" width="18" height="18" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.100-3.100a2 2 0 0 0-2.800 0L6 21"/>`,
	"audio":        `<path d="M2 10v3M6 6v11M10 3v18M14 8v7M18 5v13M22 10v3"/>`,
	"brightness":   `<circle cx="12" cy="12" r="4"/><path d="M12 3v1M12 20v1M3 12h1M20 12h1M5.600 5.600l.7.7M17.700 17.700l.7.7M5.600 18.400l.7-.7M17.700 6.300l.7-.7"/>`,
	"thermometer":  `<path d="M14 4v10.500a4 4 0 1 1-4 0V4a2 2 0 0 1 4 0z"/>`,
	"volume":       `<path d="M11 5 6 9H2v6h4l5 4zM16 9a5 5 0 0 1 0 6M19.400 18.400a9 9 0 0 0 0-12.800"/>`,
	"timer":        `<path d="M10 2h4M12 14l3-3"/><circle cx="12" cy="14" r="8"/>`,
	"circle-plus":  `<circle cx="12" cy="12" r="10"/><path d="M8 12h8M12 8v8"/>`,
	"camera":       `<path d="M14.500 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3z"/><circle cx="12" cy="13" r="3"/>`,
	"cloud":        `<path d="M17.500 19H9a7 7 0 1 1 6.700-9h1.800a4.500 4.500 0 1 1 0 9z"/>`,
	"globe":        `<circle cx="12" cy="12" r="10"/><path d="M12 2a14.500 14.500 0 0 0 0 20 14.500 14.500 0 0 0 0-20M2 12h20"/>`,
	"circle-alert": `<circle cx="12" cy="12" r="10"/><path d="M12 8v4M12 16h.01"/>`,
}

var previewGlyphAssets sync.Map // name → *icons.SVG, parsed once

// glyph is the named preview icon, 16 pixels until sized. It panics for a
// name that previewGlyphs lacks or that does not parse: both are fixed
// source, which a test walks.
func glyph(name string) *icons.Widget {
	if asset, ok := previewGlyphAssets.Load(name); ok {
		return icons.New(asset.(*icons.SVG))
	}
	body, ok := previewGlyphs[name]
	if !ok {
		panic("create: no preview glyph " + name)
	}
	asset, err := icons.Parse([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
	if err != nil {
		panic("create: preview glyph " + name + ": " + err.Error())
	}
	previewGlyphAssets.Store(name, asset)
	return icons.New(asset)
}
