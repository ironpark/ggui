//go:build darwin && !ios

package ggui

import (
	"os"
	"path/filepath"
)

// launchAgentsDir is the user's ~/Library/LaunchAgents.
func launchAgentsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents"), nil
}

func setOpenAtLogin(id, exe string, args []string, enabled bool) error {
	dir, err := launchAgentsDir()
	if err != nil {
		return err
	}
	return setLoginFile(launchAgentPath(dir, id), launchAgent(id, exe, args...), enabled)
}

func openAtLogin(id string) (bool, error) {
	dir, err := launchAgentsDir()
	if err != nil {
		return false, err
	}
	return loginFileExists(launchAgentPath(dir, id))
}
