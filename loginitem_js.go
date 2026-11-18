package ggui

import (
	"errors"
	"fmt"
)

// bundleID is "": only macOS runs an app from a bundle.
func bundleID() string { return "" }

// errNoLoginItem is what the login item calls return in a browser, which
// opens no page at login.
var errNoLoginItem = fmt.Errorf("ggui: open at login in a browser: %w", errors.ErrUnsupported)

func setOpenAtLogin(string, string, []string, bool) error { return errNoLoginItem }

func openAtLogin(string) (bool, error) { return false, errNoLoginItem }
