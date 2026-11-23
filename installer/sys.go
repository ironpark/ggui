package installer

import (
	"os"
	"strings"
)

// UninstallEntry holds the metadata recorded in Windows Add/Remove Programs.
type UninstallEntry struct {
	Meta              Metadata
	InstallDir        string
	ExecutablePath    string
	UninstallExePath  string
	EstimatedSizeKB   uint32
	UninstallCommand  string
	QuietUninstallCmd string
}

// Backend abstracts OS-specific installer actions (Windows Registry, COM
// shortcuts, shell notifications, and detached process launch) so the core
// engine and ggui wizard can be unit-tested on any host.
type Backend interface {
	DefaultInstallDir(meta Metadata) string
	ExistingInstall(meta Metadata) (dir, version string, ok bool)
	StartMenuDir(scope string) string
	DesktopDir(scope string) string
	CreateShortcut(lnkPath, targetPath, workDir, desc, iconPath string) error
	RemoveShortcut(lnkPath string) error
	NotifyShell()
	WriteUninstallEntry(entry UninstallEntry) error
	DeleteUninstallEntry(meta Metadata) error
	Launch(exePath string) error
	SelfExe() (string, error)
	RelaunchForUninstall(installDir string, silent bool) (bool, error)
	ScheduleTempDelete(selfExe string)
}

// expandWindowsEnv expands Windows-style %VAR% references using getenv.
func expandWindowsEnv(s string, getenv func(string) string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '%' {
			end := strings.IndexByte(s[i+1:], '%')
			if end > 0 {
				name := s[i+1 : i+1+end]
				if v := getenv(name); v != "" {
					b.WriteString(v)
					i += end + 2
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func defaultSelfExe() (string, error) {
	return os.Executable()
}
