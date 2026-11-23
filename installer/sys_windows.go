//go:build windows

package installer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

type windowsBackend struct{}

func defaultBackend() Backend { return windowsBackend{} }

func uninstallKeyRoot(scope string) registry.Key {
	if scope == ScopeMachine {
		return registry.LOCAL_MACHINE
	}
	return registry.CURRENT_USER
}

func uninstallKeyPath(id string) string {
	return `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + id
}

func (windowsBackend) DefaultInstallDir(meta Metadata) string {
	if meta.InstallDir != "" {
		return filepath.Clean(expandWindowsEnv(meta.InstallDir, os.Getenv))
	}
	if meta.NormalizedScope() == ScopeMachine {
		base := os.Getenv("ProgramFiles")
		if base == "" {
			base = `C:\Program Files`
		}
		return filepath.Join(base, meta.Name)
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "AppData", "Local")
		} else {
			base = `C:\Users\Public`
		}
	}
	return filepath.Join(base, "Programs", meta.Name)
}

func (windowsBackend) ExistingInstall(meta Metadata) (dir, version string, ok bool) {
	k, err := registry.OpenKey(uninstallKeyRoot(meta.NormalizedScope()), uninstallKeyPath(meta.ID), registry.QUERY_VALUE)
	if err != nil {
		return "", "", false
	}
	defer k.Close()
	loc, _, err := k.GetStringValue("InstallLocation")
	if err != nil || strings.TrimSpace(loc) == "" {
		return "", "", false
	}
	ver, _, _ := k.GetStringValue("DisplayVersion")
	return loc, ver, true
}

func (windowsBackend) StartMenuDir(scope string) string {
	if scope == ScopeMachine {
		base := os.Getenv("ProgramData")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs")
	}
	base := os.Getenv("APPDATA")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, "AppData", "Roaming")
		}
	}
	return filepath.Join(base, "Microsoft", "Windows", "Start Menu", "Programs")
}

func (windowsBackend) DesktopDir(scope string) string {
	if scope == ScopeMachine {
		base := os.Getenv("PUBLIC")
		if base == "" {
			base = `C:\Users\Public`
		}
		return filepath.Join(base, "Desktop")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Desktop")
}

func (windowsBackend) WriteUninstallEntry(entry UninstallEntry) error {
	root := uninstallKeyRoot(entry.Meta.NormalizedScope())
	k, _, err := registry.CreateKey(root, uninstallKeyPath(entry.Meta.ID), registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("writing uninstall registry key: %w", err)
	}
	defer k.Close()

	setStr := func(name, val string) error {
		if val == "" {
			return nil
		}
		return k.SetStringValue(name, val)
	}
	if err := setStr("DisplayName", entry.Meta.Name); err != nil {
		return err
	}
	if err := setStr("DisplayVersion", entry.Meta.Version); err != nil {
		return err
	}
	if err := setStr("Publisher", entry.Meta.Publisher); err != nil {
		return err
	}
	if err := setStr("DisplayIcon", entry.ExecutablePath+",0"); err != nil {
		return err
	}
	if err := setStr("InstallLocation", entry.InstallDir); err != nil {
		return err
	}
	if err := setStr("UninstallString", entry.UninstallCommand); err != nil {
		return err
	}
	if err := setStr("QuietUninstallString", entry.QuietUninstallCmd); err != nil {
		return err
	}
	if err := setStr("URLInfoAbout", entry.Meta.Homepage); err != nil {
		return err
	}
	if err := k.SetDWordValue("EstimatedSize", entry.EstimatedSizeKB); err != nil {
		return err
	}
	if err := k.SetDWordValue("NoModify", 1); err != nil {
		return err
	}
	return k.SetDWordValue("NoRepair", 1)
}

func (windowsBackend) DeleteUninstallEntry(meta Metadata) error {
	err := registry.DeleteKey(uninstallKeyRoot(meta.NormalizedScope()), uninstallKeyPath(meta.ID))
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (windowsBackend) Launch(exePath string) error {
	cmd := exec.Command(exePath)
	cmd.Dir = filepath.Dir(exePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	return cmd.Start()
}

func (windowsBackend) SelfExe() (string, error) {
	return defaultSelfExe()
}

// RelaunchForUninstall copies the running uninstaller into %TEMP% and launches
// it with --from <installDir> --cleanup-self when the process currently lives
// inside installDir, so installDir is unlocked and can be removed completely.
func (windowsBackend) RelaunchForUninstall(installDir string, silent bool) (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, nil
	}
	absExe, err := filepath.Abs(exe)
	if err != nil {
		return false, nil
	}
	absDir, err := filepath.Abs(installDir)
	if err != nil {
		return false, nil
	}
	rel, err := filepath.Rel(absDir, absExe)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return false, nil
	}
	tmp, err := os.CreateTemp("", "ggui-uninstall-*.exe")
	if err != nil {
		return false, err
	}
	tmpPath := tmp.Name()
	in, err := os.Open(absExe)
	if err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return false, err
	}
	_, copyErr := io.Copy(tmp, in)
	in.Close()
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(tmpPath)
		if copyErr != nil {
			return false, copyErr
		}
		return false, closeErr
	}
	args := []string{"--uninstall", "--from", absDir, "--cleanup-self"}
	if silent {
		args = append(args, "--silent")
	}
	cmd := exec.Command(tmpPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		os.Remove(tmpPath)
		return false, err
	}
	return true, nil
}

func (windowsBackend) ScheduleTempDelete(selfExe string) {
	if selfExe == "" {
		return
	}
	cmd := exec.Command("cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP,
		CmdLine:       fmt.Sprintf(`cmd.exe /C "ping -n 3 127.0.0.1 >nul & del /f /q "%s""`, selfExe),
	}
	_ = cmd.Start()
}

func (windowsBackend) RemoveShortcut(lnkPath string) error {
	err := os.Remove(lnkPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

var (
	modOle32           = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize = modOle32.NewProc("CoUninitialize")
	procCoCreateInst   = modOle32.NewProc("CoCreateInstance")
	modShell32         = windows.NewLazySystemDLL("shell32.dll")
	procSHChangeNotify = modShell32.NewProc("SHChangeNotify")
)

const (
	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1
	shcneAssocChanged       = 0x08000000
	shcnfIdList             = 0x0000
)

var (
	clsidShellLink  = windows.GUID{Data1: 0x00021401, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIShellLinkW  = windows.GUID{Data1: 0x000214F9, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
	iidIPersistFile = windows.GUID{Data1: 0x0000010b, Data2: 0x0000, Data3: 0x0000, Data4: [8]byte{0xC0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x46}}
)

type comObj struct {
	vtbl *[32]uintptr
}

func (o *comObj) release() {
	if o != nil {
		syscall.SyscallN(o.vtbl[2], uintptr(unsafe.Pointer(o)))
	}
}

func (windowsBackend) NotifyShell() {
	procSHChangeNotify.Call(shcneAssocChanged, shcnfIdList, 0, 0)
}

// CreateShortcut writes a Windows Shell Link (.lnk) file using COM IShellLinkW
// and IPersistFile without cgo or PowerShell.
func (windowsBackend) CreateShortcut(lnkPath, targetPath, workDir, desc, iconPath string) error {
	if err := os.MkdirAll(filepath.Dir(lnkPath), 0o755); err != nil {
		return err
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	// S_OK (0) or S_FALSE (1) mean COM was initialized and must be uninitialized.
	// RPC_E_CHANGED_MODE (0x80010106) means the thread already has COM initialized.
	const rpcEChangedMode = 0x80010106
	if hr == 0 || hr == 1 {
		defer procCoUninitialize.Call()
	} else if uint32(hr) != rpcEChangedMode {
		return fmt.Errorf("CoInitializeEx failed: 0x%x", uint32(hr))
	}

	var sl *comObj
	hr, _, _ = procCoCreateInst.Call(
		uintptr(unsafe.Pointer(&clsidShellLink)),
		0,
		clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidIShellLinkW)),
		uintptr(unsafe.Pointer(&sl)),
	)
	if int32(hr) < 0 || sl == nil {
		return fmt.Errorf("CoCreateInstance(ShellLink) failed: 0x%x", uint32(hr))
	}
	defer sl.release()

	callStr := func(slot int, s string) error {
		p, err := windows.UTF16PtrFromString(s)
		if err != nil {
			return err
		}
		r, _, _ := syscall.SyscallN(sl.vtbl[slot], uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(p)))
		if int32(r) < 0 {
			return fmt.Errorf("IShellLinkW[%d] failed: 0x%x", slot, uint32(r))
		}
		return nil
	}

	// IShellLinkW: SetPath=20, SetWorkingDirectory=9, SetDescription=7, SetIconLocation=17.
	if err := callStr(20, targetPath); err != nil {
		return err
	}
	if workDir != "" {
		if err := callStr(9, workDir); err != nil {
			return err
		}
	}
	if desc != "" {
		if err := callStr(7, desc); err != nil {
			return err
		}
	}
	if iconPath != "" {
		p, err := windows.UTF16PtrFromString(iconPath)
		if err != nil {
			return err
		}
		r, _, _ := syscall.SyscallN(sl.vtbl[17], uintptr(unsafe.Pointer(sl)), uintptr(unsafe.Pointer(p)), 0)
		if int32(r) < 0 {
			return fmt.Errorf("IShellLinkW.SetIconLocation failed: 0x%x", uint32(r))
		}
	}

	var pf *comObj
	r, _, _ := syscall.SyscallN(
		sl.vtbl[0], // QueryInterface
		uintptr(unsafe.Pointer(sl)),
		uintptr(unsafe.Pointer(&iidIPersistFile)),
		uintptr(unsafe.Pointer(&pf)),
	)
	if int32(r) < 0 || pf == nil {
		return fmt.Errorf("QueryInterface(IPersistFile) failed: 0x%x", uint32(r))
	}
	defer pf.release()

	lnkPtr, err := windows.UTF16PtrFromString(lnkPath)
	if err != nil {
		return err
	}
	// IPersistFile::Save is vtable slot 6.
	r, _, _ = syscall.SyscallN(pf.vtbl[6], uintptr(unsafe.Pointer(pf)), uintptr(unsafe.Pointer(lnkPtr)), 1)
	if int32(r) < 0 {
		return fmt.Errorf("IPersistFile.Save(%q) failed: 0x%x", lnkPath, uint32(r))
	}
	return nil
}
