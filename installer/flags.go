package installer

import (
	"fmt"
	"strings"
)

// CLIOptions holds command-line options accepted by an installer binary.
// Both POSIX (--silent, --dir) and classic Windows (/S, /D=...) forms work.
type CLIOptions struct {
	Silent       bool
	Uninstall    bool
	InstallDir   string
	FromDir      string // internal: install dir passed to temp uninstaller copy
	CleanupSelf  bool   // internal: schedule removal of temp uninstaller copy on exit
	NoDesktop    bool
	NoStartMenu  bool
	NoLaunch     bool
	DesktopSet   bool
	StartMenuSet bool
	LaunchSet    bool
	ShowHelp     bool
}

// ParseArgs parses command-line arguments passed to the installer binary.
func ParseArgs(args []string) (CLIOptions, error) {
	var o CLIOptions
	for i := 0; i < len(args); i++ {
		a := args[i]
		lower := strings.ToLower(a)
		switch {
		case lower == "-h" || lower == "--help" || lower == "/?":
			o.ShowHelp = true
		case lower == "--silent" || lower == "-s" || lower == "/s" || lower == "/quiet" || lower == "/qn":
			o.Silent = true
		case lower == "--uninstall" || lower == "/uninstall":
			o.Uninstall = true
		case lower == "--cleanup-self":
			o.CleanupSelf = true
		case lower == "--no-desktop-shortcut" || lower == "--no-desktop":
			o.NoDesktop = true
			o.DesktopSet = true
		case lower == "--no-start-menu" || lower == "--no-startmenu":
			o.NoStartMenu = true
			o.StartMenuSet = true
		case lower == "--no-launch":
			o.NoLaunch = true
			o.LaunchSet = true
		case lower == "--dir" || lower == "-d":
			if i+1 >= len(args) {
				return o, fmt.Errorf("flag %s requires a directory path", a)
			}
			i++
			o.InstallDir = strings.Trim(args[i], `"`)
		case strings.HasPrefix(lower, "--dir="):
			o.InstallDir = strings.Trim(a[len("--dir="):], `"`)
		case strings.HasPrefix(lower, "/d="):
			// Classic NSIS /D=C:\Path form (may include spaces in trailing args).
			o.InstallDir = strings.Trim(a[len("/D="):], `"`)
		case lower == "--from":
			if i+1 >= len(args) {
				return o, fmt.Errorf("flag %s requires a directory path", a)
			}
			i++
			o.FromDir = strings.Trim(args[i], `"`)
		case strings.HasPrefix(lower, "--from="):
			o.FromDir = strings.Trim(a[len("--from="):], `"`)
		default:
			return o, fmt.Errorf("unknown installer option %q (use --help for usage)", a)
		}
	}
	return o, nil
}
