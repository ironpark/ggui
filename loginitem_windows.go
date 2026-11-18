package ggui

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// The keys of the user's startup programs: Run holds the command of each,
// by name, and StartupApproved\Run what Task Manager's Startup apps list
// turned off, by the same name.
const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// bundleID is "": only macOS runs an app from a bundle.
func bundleID() string { return "" }

func setOpenAtLogin(id, exe string, enabled bool) error {
	if enabled {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
		if err != nil {
			return err
		}
		err = k.SetStringValue(id, runCommand(exe))
		if cerr := k.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	} else if err := deleteValue(runKey, id); err != nil {
		return err
	}
	// The user asking the app again settles what Task Manager was told:
	// an entry it turned off would otherwise stay off.
	return deleteValue(approvedKey, id)
}

func openAtLogin(id string) (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, _, err = k.GetStringValue(id)
	if cerr := k.Close(); err == nil {
		err = cerr
	}
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !turnedOff(id), nil
}

// turnedOff reports whether Task Manager turned the startup program id
// off: its StartupApproved value's first byte is odd.
func turnedOff(id string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	v, _, err := k.GetBinaryValue(id)
	// A key opened to be read has nothing to lose in closing.
	_ = k.Close()
	return err == nil && len(v) > 0 && v[0]&1 != 0
}

// deleteValue removes the value name from the user's key at path, which
// may be missing already, as may the key.
func deleteValue(path, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = k.DeleteValue(name)
	if cerr := k.Close(); err == nil {
		err = cerr
	}
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}
