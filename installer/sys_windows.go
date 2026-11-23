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

func envKeyRootAndPath(scope string) (registry.Key, string) {
	if normalizeEnvScope(scope) == ScopeMachine {
		return registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	}
	return registry.CURRENT_USER, `Environment`
}

func (windowsBackend) GetEnv(scope, name string) (string, bool, error) {
	n, err := validateEnvName(name)
	if err != nil {
		return "", false, err
	}
	root, path := envKeyRootAndPath(scope)
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("opening environment key: %w", err)
	}
	defer k.Close()
	val, _, err := k.GetStringValue(n)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading environment variable %s: %w", n, err)
	}
	return val, true, nil
}

func (windowsBackend) SetEnv(scope, name, value string) error {
	n, err := validateEnvName(name)
	if err != nil {
		return err
	}
	root, path := envKeyRootAndPath(scope)
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening environment key: %w", err)
	}
	defer k.Close()
	if strings.Contains(value, "%") {
		err = k.SetExpandStringValue(n, value)
	} else {
		err = k.SetStringValue(n, value)
	}
	if err != nil {
		return fmt.Errorf("writing environment variable %s: %w", n, err)
	}
	broadcastEnvironmentChange()
	return nil
}

func (windowsBackend) DeleteEnv(scope, name string) error {
	n, err := validateEnvName(name)
	if err != nil {
		return err
	}
	root, path := envKeyRootAndPath(scope)
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening environment key: %w", err)
	}
	defer k.Close()
	err = k.DeleteValue(n)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("deleting environment variable %s: %w", n, err)
	}
	broadcastEnvironmentChange()
	return nil
}

func (windowsBackend) AddToPath(scope, dir string) error {
	return updateWindowsPath(scope, dir, true)
}

func (windowsBackend) RemoveFromPath(scope, dir string) error {
	return updateWindowsPath(scope, dir, false)
}

func updateWindowsPath(scope, dir string, add bool) error {
	root, path := envKeyRootAndPath(scope)
	k, _, err := registry.CreateKey(root, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer k.Close()

	cur, _, err := k.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("reading Path: %w", err)
	}
	next, changed := modifyPathList(cur, dir, add)
	if !changed {
		return nil
	}
	if err := k.SetExpandStringValue("Path", next); err != nil {
		return fmt.Errorf("writing Path: %w", err)
	}
	broadcastEnvironmentChange()
	return nil
}

func registryRootKey(root RegistryRoot) (registry.Key, error) {
	nr, err := normalizeRegistryRoot(root)
	if err != nil {
		return 0, err
	}
	switch nr {
	case RegistryHKCU:
		return registry.CURRENT_USER, nil
	case RegistryHKLM:
		return registry.LOCAL_MACHINE, nil
	case RegistryHKCR:
		return registry.CLASSES_ROOT, nil
	default:
		return 0, fmt.Errorf("installer: unsupported registry root %q", root)
	}
}

func (windowsBackend) RegistryKeyExists(root RegistryRoot, keyPath string) (bool, error) {
	r, err := registryRootKey(root)
	if err != nil {
		return false, err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(r, kp, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	k.Close()
	return true, nil
}

func (windowsBackend) GetRegistryValue(root RegistryRoot, keyPath, name string) (RegistryValue, bool, error) {
	r, err := registryRootKey(root)
	if err != nil {
		return RegistryValue{}, false, err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return RegistryValue{}, false, err
	}
	k, err := registry.OpenKey(r, kp, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return RegistryValue{}, false, nil
	}
	if err != nil {
		return RegistryValue{}, false, fmt.Errorf("opening registry key %s\\%s: %w", root, kp, err)
	}
	defer k.Close()

	_, valType, err := k.GetValue(name, nil)
	if errors.Is(err, registry.ErrNotExist) {
		return RegistryValue{}, false, nil
	}
	if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
		return RegistryValue{}, false, fmt.Errorf("reading registry value %s\\%s\\%s: %w", root, kp, name, err)
	}

	switch valType {
	case registry.SZ:
		s, _, err := k.GetStringValue(name)
		return RegistryValue{Type: RegSZ, String: s}, err == nil, err
	case registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		return RegistryValue{Type: RegExpandSZ, String: s}, err == nil, err
	case registry.DWORD:
		v, _, err := k.GetIntegerValue(name)
		return RegistryValue{Type: RegDWORD, DWORD: uint32(v)}, err == nil, err
	case registry.QWORD:
		v, _, err := k.GetIntegerValue(name)
		return RegistryValue{Type: RegQWORD, QWORD: v}, err == nil, err
	case registry.BINARY:
		b, _, err := k.GetBinaryValue(name)
		return RegistryValue{Type: RegBinary, Binary: b}, err == nil, err
	case registry.MULTI_SZ:
		ss, _, err := k.GetStringsValue(name)
		return RegistryValue{Type: RegMultiSZ, Multi: ss}, err == nil, err
	default:
		b, _, err := k.GetBinaryValue(name)
		return RegistryValue{Type: RegBinary, Binary: b}, err == nil, err
	}
}

func (windowsBackend) SetRegistryValue(root RegistryRoot, keyPath, name string, val RegistryValue) error {
	r, err := registryRootKey(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	v, err := normalizeRegistryValue(val)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(r, kp, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("creating registry key %s\\%s: %w", root, kp, err)
	}
	defer k.Close()

	switch v.Type {
	case RegSZ:
		err = k.SetStringValue(name, v.String)
	case RegExpandSZ:
		err = k.SetExpandStringValue(name, v.String)
	case RegDWORD:
		err = k.SetDWordValue(name, v.DWORD)
	case RegQWORD:
		err = k.SetQWordValue(name, v.QWORD)
	case RegBinary:
		err = k.SetBinaryValue(name, v.Binary)
	case RegMultiSZ:
		err = k.SetStringsValue(name, v.Multi)
	}
	if err != nil {
		return fmt.Errorf("writing registry value %s\\%s\\%s: %w", root, kp, name, err)
	}
	return nil
}

func (windowsBackend) DeleteRegistryValue(root RegistryRoot, keyPath, name string) error {
	r, err := registryRootKey(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(r, kp, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening registry key %s\\%s: %w", root, kp, err)
	}
	defer k.Close()
	err = k.DeleteValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func (windowsBackend) DeleteRegistryKey(root RegistryRoot, keyPath string, onlyIfEmpty bool) error {
	r, err := registryRootKey(root)
	if err != nil {
		return err
	}
	kp, err := normalizeRegistryKey(keyPath)
	if err != nil {
		return err
	}
	if onlyIfEmpty {
		k, err := registry.OpenKey(r, kp, registry.QUERY_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		ki, statErr := k.Stat()
		k.Close()
		if statErr != nil {
			return statErr
		}
		if ki.ValueCount > 0 || ki.SubKeyCount > 0 {
			return nil
		}
		err = registry.DeleteKey(r, kp)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	return deleteRegistryTree(r, kp)
}

func deleteRegistryTree(r registry.Key, kp string) error {
	k, err := registry.OpenKey(r, kp, registry.ENUMERATE_SUB_KEYS)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	subs, readErr := k.ReadSubKeyNames(0)
	k.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	for _, sub := range subs {
		if err := deleteRegistryTree(r, kp+`\`+sub); err != nil {
			return err
		}
	}
	err = registry.DeleteKey(r, kp)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	return err
}

func broadcastEnvironmentChange() {
	envPtr, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	const (
		hwndBroadcast     = 0xFFFF
		wmSettingChange   = 0x001A
		smtoAbortIfHung   = 0x0002
		broadcastTimeoutM = 1000
	)
	var result uintptr
	procSendMessageTimeoutW.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(envPtr)),
		smtoAbortIfHung,
		broadcastTimeoutM,
		uintptr(unsafe.Pointer(&result)),
	)
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
	modOle32                = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx      = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize      = modOle32.NewProc("CoUninitialize")
	procCoCreateInst        = modOle32.NewProc("CoCreateInstance")
	modShell32              = windows.NewLazySystemDLL("shell32.dll")
	procSHChangeNotify      = modShell32.NewProc("SHChangeNotify")
	modUser32               = windows.NewLazySystemDLL("user32.dll")
	procSendMessageTimeoutW = modUser32.NewProc("SendMessageTimeoutW")
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
