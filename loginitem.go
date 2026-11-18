package ggui

import (
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/ironpark/ggui/internal/robustio"
)

// SetOpenAtLogin has the system open the app when the user logs in, or
// stops it doing so. It registers the executable running now, so call it
// from the installed app, and again after the app moves. It may be called
// from any goroutine, before Run too.
//
// macOS is given a LaunchAgent, ~/Library/LaunchAgents/<id>.plist, run at
// load; from the next login it lists the app under Login Items in System
// Settings, where the user can turn it off. Windows is given a value in the
// user's HKCU\Software\Microsoft\Windows\CurrentVersion\Run key, which
// Task Manager's Startup apps list shows. Linux and the BSDs are given an
// XDG autostart entry, $XDG_CONFIG_HOME/autostart/<id>.desktop. The browser
// has nothing of the kind, and there it returns an error that wraps
// errors.ErrUnsupported.
//
// The registration is named for the app's id, as AppID gives it: the
// bundle identifier on macOS when the app runs from a bundle, else the id
// ggui build linked in, else the id given to App.SingleInstance, else the
// executable's name. Keep it the same from one release to the next, so
// that a later version finds what an earlier one registered.
func SetOpenAtLogin(enabled bool) error {
	exe, err := loginExecutable()
	if err != nil {
		return err
	}
	return setOpenAtLogin(loginID(exe), exe, enabled)
}

// OpenAtLogin reports whether the app is registered to open at login, as
// SetOpenAtLogin registers it. On Windows an entry the user turned off in
// Task Manager counts as not registered.
func OpenAtLogin() (bool, error) {
	exe, err := loginExecutable()
	if err != nil {
		return false, err
	}
	return openAtLogin(loginID(exe))
}

// instanceName is the id App.SingleInstance last claimed: the app's id
// where neither a bundle identifier nor a linked one is.
var instanceName atomic.Pointer[string]

// loginExecutable is the path of the running executable, links resolved:
// inside an app bundle, its Contents/MacOS/<name>.
func loginExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe, nil
}

// loginID is the name the login item of the app running exe is kept under:
// the app's id, made a single file name.
func loginID(exe string) string {
	id, _ := appIDOf(exe)
	return placementDirName(id)
}

// launchAgentPath is where the LaunchAgent named id lives in dir, the
// user's ~/Library/LaunchAgents.
func launchAgentPath(dir, id string) string { return filepath.Join(dir, id+".plist") }

// launchAgent is the property list of a LaunchAgent, labelled id, that
// runs exe when it is loaded, which a user's agents are at login.
func launchAgent(id, exe string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>`)
	xmlText(&b, id)
	b.WriteString(`</string>
	<key>ProgramArguments</key>
	<array>
		<string>`)
	xmlText(&b, exe)
	b.WriteString(`</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`)
	return []byte(b.String())
}

// xmlText writes s to b escaped as XML character data.
func xmlText(b *strings.Builder, s string) {
	// Writing to a strings.Builder does not fail.
	_ = xml.EscapeText(b, []byte(s))
}

// autostartDir is the XDG autostart directory, given the environment:
// $XDG_CONFIG_HOME/autostart, or ~/.config/autostart where that is unset
// or not absolute, as the specification says to treat it.
func autostartDir(getenv func(string) string, home string) string {
	config := getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "autostart")
}

// autostartPath is where the autostart entry named id lives in dir.
func autostartPath(dir, id string) string { return filepath.Join(dir, id+".desktop") }

// autostartEntry is the desktop entry that starts exe at login, shown to
// the user as name.
func autostartEntry(name, exe string) []byte {
	return []byte("[Desktop Entry]\n" +
		"Type=Application\n" +
		"Name=" + desktopString(name) + "\n" +
		"Exec=" + desktopString(execArg(exe)) + "\n" +
		"X-GNOME-Autostart-enabled=true\n")
}

// execArg quotes path as one argument of a desktop entry's Exec key: in
// double quotes, with the characters the specification reserves inside
// them escaped, and a percent sign doubled so it is not a field code.
func execArg(path string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		switch r {
		case '"', '`', '$', '\\':
			b.WriteByte('\\')
		case '%':
			b.WriteByte('%')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// desktopString escapes s as a desktop entry's string value, whose
// backslashes and line breaks are escaped before any rule of the key's own
// applies.
func desktopString(s string) string {
	return strings.NewReplacer(`\`, `\\`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s)
}

// runCommand is the data of the Run value that starts exe: its path in
// quotes, as Windows reads a path that may hold a space.
func runCommand(exe string) string { return `"` + exe + `"` }

// setLoginFile writes data to path when enabled, or removes path when not,
// a file already gone included.
func setLoginFile(path string, data []byte, enabled bool) error {
	if !enabled {
		if err := robustio.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	return writeFileReplacing(path, data)
}

// loginFileExists reports whether the login item at path is there.
func loginFileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	}
	return false, err
}

// writeFileReplacing writes data to path through a file beside it, renamed
// over path once complete, so that a reader never sees half of it, and
// makes the directory if it is missing.
func writeFileReplacing(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(f.Name(), 0o644)
	}
	if err == nil {
		err = robustio.Rename(f.Name(), path)
	}
	if err != nil {
		if rerr := os.Remove(f.Name()); rerr != nil && !errors.Is(rerr, fs.ErrNotExist) {
			err = errors.Join(err, rerr)
		}
	}
	return err
}
