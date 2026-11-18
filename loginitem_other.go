//go:build !(darwin && !ios) && !windows && !js

package ggui

import (
	"os"
	"path/filepath"
	"strings"
)

// bundleID is "": only macOS runs an app from a bundle.
func bundleID() string { return "" }

// userAutostartDir is the user's XDG autostart directory.
func userAutostartDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return autostartDir(os.Getenv, home), nil
}

func setOpenAtLogin(id, exe string, enabled bool) error {
	dir, err := userAutostartDir()
	if err != nil {
		return err
	}
	name := strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe))
	return setLoginFile(autostartPath(dir, id), autostartEntry(name, exe), enabled)
}

func openAtLogin(id string) (bool, error) {
	dir, err := userAutostartDir()
	if err != nil {
		return false, err
	}
	return loginFileExists(autostartPath(dir, id))
}
