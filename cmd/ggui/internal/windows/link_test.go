package windows

import (
	"debug/pe"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// linkProgram builds an empty program for windows/arch with the resources
// of testResources linked in from the main package's directory, as ggui
// build places them, and returns the program's path.
func linkProgram(t *testing.T, arch string, ldflags string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("cross-compiles a program for Windows")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hello\n\ngo 1.27\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644)
	obj, _, err := Object(testResources(t, arch))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := Place(dir, arch, obj); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "hello.exe")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOOS=windows", "GOARCH="+arch, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return exe
}

// The resources must survive the linker: Go's links the object's .rsrc
// section into the program's own, relocating each data entry, and the
// program must say where it is in its resource directory.
func TestLinked(t *testing.T) {
	for _, arch := range Arches {
		t.Run(arch, func(t *testing.T) {
			exe := linkProgram(t, arch, "-s -w -H=windowsgui")
			f, err := pe.Open(exe)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			if f.Machine != machine[arch].machine {
				t.Errorf("machine %#x", f.Machine)
			}
			var subsystem uint16
			var dirs [16]pe.DataDirectory
			switch oh := f.OptionalHeader.(type) {
			case *pe.OptionalHeader32:
				subsystem, dirs = oh.Subsystem, oh.DataDirectory
			case *pe.OptionalHeader64:
				subsystem, dirs = oh.Subsystem, oh.DataDirectory
			}
			if subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI {
				t.Errorf("subsystem %d, want %d, a GUI program that opens no console", subsystem, pe.IMAGE_SUBSYSTEM_WINDOWS_GUI)
			}
			s := f.Section(".rsrc")
			if s == nil {
				t.Fatal("the program has no .rsrc section")
			}
			if d := dirs[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE]; d.VirtualAddress != s.VirtualAddress || d.Size == 0 {
				t.Errorf("resource directory %+v; the .rsrc section is at %#x", d, s.VirtualAddress)
			}
			data, err := s.Data()
			if err != nil {
				t.Fatal(err)
			}
			// Each data entry now holds its data's address in the program.
			checkResources(t, walk(t, data[:s.VirtualSize], s.VirtualAddress))
		})
	}
}
