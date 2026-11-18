package windows

import (
	"os/exec"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32              = windows.NewLazySystemDLL("user32.dll")
	procLoadImageW      = user32.NewProc("LoadImageW")
	procDestroyIcn      = user32.NewProc("DestroyIcon")
	procExtractIconExW  = windows.NewLazySystemDLL("shell32.dll").NewProc("ExtractIconExW")
	extractCountOfIcons = ^uintptr(0) // nIconIndex -1: how many icons the file has
)

// Windows itself must take what was linked: start the program, which it
// refuses to with a manifest it cannot parse; load its icon at the
// largest and smallest sizes; and read its version information as
// Explorer does.
func TestWindowsLoads(t *testing.T) {
	exe := linkProgram(t, runtime.GOARCH, "-s -w -H=windowsgui")
	if out, err := exec.Command(exe).CombinedOutput(); err != nil {
		t.Fatalf("running the program: %v %s", err, out)
	}
	// Explorer's view: the shell counts the program's icons.
	name, _ := windows.UTF16PtrFromString(exe)
	if n, _, _ := procExtractIconExW.Call(uintptr(unsafe.Pointer(name)), extractCountOfIcons, 0, 0, 0); n != 1 {
		t.Errorf("the shell finds %d icons in the program, want 1", n)
	}

	h, err := windows.LoadLibraryEx(exe, 0, windows.LOAD_LIBRARY_AS_DATAFILE|windows.LOAD_LIBRARY_AS_IMAGE_RESOURCE)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.FreeLibrary(h)
	const imageIcon = 1
	for _, size := range []uintptr{256, 48, 16} {
		icon, _, err := procLoadImageW.Call(uintptr(h), 1, imageIcon, size, size, 0)
		if icon == 0 {
			t.Errorf("LoadImage of icon 1 at %d pixels: %v", size, err)
			continue
		}
		procDestroyIcn.Call(icon)
	}
	if _, err := windows.FindResource(h, windows.ResourceID(1), windows.RT_MANIFEST); err != nil {
		t.Errorf("manifest: %v", err)
	}

	n, err := windows.GetFileVersionInfoSize(exe, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, n)
	if err := windows.GetFileVersionInfo(exe, 0, n, unsafe.Pointer(&buf[0])); err != nil {
		t.Fatal(err)
	}
	var p *uint16
	var l uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\040904B0\FileDescription`, unsafe.Pointer(&p), &l); err != nil {
		t.Fatal(err)
	}
	if got := windows.UTF16PtrToString(p); got != "Hello & Goodbye" {
		t.Errorf("FileDescription = %q", got)
	}
	var fixed *windows.VS_FIXEDFILEINFO
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &l); err != nil {
		t.Fatal(err)
	}
	if fixed.FileVersionMS != 1<<16|2 || fixed.FileVersionLS != 3<<16 {
		t.Errorf("file version %#x %#x", fixed.FileVersionMS, fixed.FileVersionLS)
	}
}
