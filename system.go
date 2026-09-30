package ggui

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/ironpark/ggfx"
)

// Appearance is the light or dark look of what the platform draws for the
// app: title bars, and on some desktops scroll bars and menus.
type Appearance uint8

// The appearances.
const (
	// AppearanceSystem follows the desktop's setting.
	AppearanceSystem Appearance = iota
	AppearanceLight
	AppearanceDark
)

// appearance is what SetAppearance asked for, applied when Run starts if
// it was asked for before.
var appearance atomic.Uint32

// SetAppearance sets the look of the chrome the platform draws around the
// app's windows. It does not change the app's own theme: uitheme.Set does
// that, and calls this to match. It may be called from any goroutine, and
// before Run.
func SetAppearance(a Appearance) {
	if Appearance(appearance.Swap(uint32(a))) == a {
		return
	}
	if appRunning.Load() {
		applyAppearance()
	}
}

// applyAppearance hands the appearance to the engine, which needs a
// running app to change a window.
func applyAppearance() {
	mode := ggfx.ColorModeUnknown
	switch Appearance(appearance.Load()) {
	case AppearanceLight:
		mode = ggfx.ColorModeLight
	case AppearanceDark:
		mode = ggfx.ColorModeDark
	}
	ggfx.SetPreferredColorMode(mode)
}

var (
	systemDark      = State(false)
	systemDarkWatch sync.Once
)

// systemDarkPoll is how often SystemDark asks the platform, which reports
// no change as an event.
const systemDarkPoll = time.Second

// SystemDark follows whether the desktop is set to a dark appearance, for
// an app that follows it:
//
//	uitheme.Bind(ggui.SystemDark(), uitheme.Dark(), uitheme.Default())
//
// It is read while an App runs, about once a second, so a change shows
// within a second. Before Run, under a Probe and where the platform cannot
// tell, it is false.
func SystemDark() Readable[bool] {
	systemDarkWatch.Do(func() { go watchSystemDark() })
	return systemDark
}

// watchSystemDark polls the platform for the life of the process and posts
// each change to whichever window is running.
func watchSystemDark() {
	last := false
	for ; ; time.Sleep(systemDarkPoll) {
		if !appRunning.Load() {
			continue
		}
		dark := ggfx.SystemColorMode() == ggfx.ColorModeDark
		if dark == last {
			continue
		}
		if a := runningApp.Load(); a != nil {
			last = dark
			a.post(func() { systemDark.Set(dark) })
		}
	}
}
