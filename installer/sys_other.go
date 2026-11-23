//go:build !windows

package installer

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// portableBackend is a non-Windows fallback that lets the installer UI and
// engine run and be tested on macOS and Linux.
type portableBackend struct{}

func defaultBackend() Backend { return portableBackend{} }

func (portableBackend) DefaultInstallDir(meta Metadata) string {
	if meta.InstallDir != "" {
		return filepath.Clean(expandWindowsEnv(meta.InstallDir, os.Getenv))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), meta.Name)
	}
	return filepath.Join(home, "Applications", meta.Name)
}

func (portableBackend) ExistingInstall(Metadata) (string, string, bool) { return "", "", false }

func (portableBackend) StartMenuDir(string) string { return "" }
func (portableBackend) DesktopDir(string) string   { return "" }

func (portableBackend) CreateShortcut(lnkPath, targetPath, _, _, _ string) error {
	if lnkPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(lnkPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(lnkPath, []byte(targetPath+"\n"), 0o644)
}

func (portableBackend) RemoveShortcut(lnkPath string) error {
	err := os.Remove(lnkPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (portableBackend) NotifyShell()                             {}
func (portableBackend) WriteUninstallEntry(UninstallEntry) error { return nil }
func (portableBackend) DeleteUninstallEntry(Metadata) error      { return nil }

func (portableBackend) Launch(exePath string) error {
	cmd := exec.Command(exePath)
	cmd.Dir = filepath.Dir(exePath)
	return cmd.Start()
}

func (portableBackend) SelfExe() (string, error) { return defaultSelfExe() }

func (portableBackend) RelaunchForUninstall(string, bool) (bool, error) { return false, nil }
func (portableBackend) ScheduleTempDelete(string)                       {}
