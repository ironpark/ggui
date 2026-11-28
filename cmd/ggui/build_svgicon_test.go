package main

import (
	"bytes"
	"debug/pe"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/scaffold"
	"github.com/ironpark/ggui/cmd/ggui/internal/svgicon"
)

const circleSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
  <circle cx="32" cy="32" r="24" fill="#d33"/>
</svg>
`

// An SVG app.icon is drawn by the svgx of a ggui project's module, at
// 1024×1024 on transparency, and made into the program's icon, with
// nothing left in the project.
func TestSVGIcon(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go mod tidy and builds the drawing program and the app")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(root, "go.mod")); err != nil || !strings.HasPrefix(string(data), "module "+scaffold.GGUI+"\n") {
		t.Skipf("not inside a ggui checkout: %s", root)
	}
	buildEnv(t)
	dir := filepath.Join(t.TempDir(), "hello")
	if _, err := scaffold.Init(t.Context(), scaffold.Options{Dir: dir, Name: "Tiny", Replace: root, Log: io.Discard}); err != nil {
		t.Skipf("a ggui project against %s needs the modules ggui requires, from the module cache or the network: %v", root, err)
	}
	os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(circleSVG), 0o644)
	before := listDir(t, dir)
	mods := map[string][]byte{}
	for _, name := range []string{"go.mod", "go.sum"} {
		mods[name], _ = os.ReadFile(filepath.Join(dir, name))
	}

	out := filepath.Join(t.TempDir(), "icon.png")
	if err := svgicon.Draw(t.Context(), svgicon.Options{Dir: dir, SVG: filepath.Join(dir, "icon.svg"), Name: "icon.svg", Out: out}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 1024 || b.Dy() != 1024 {
		t.Fatalf("%v, want 1024×1024", b)
	}
	// The circle fills 48 of 64 units: its centre is opaque red, the
	// corners clear.
	for _, p := range [][2]int{{0, 0}, {1023, 0}, {0, 1023}, {1023, 1023}, {100, 100}} {
		if _, _, _, a := img.At(p[0], p[1]).RGBA(); a != 0 {
			t.Errorf("%v has alpha %d, want transparent", p, a)
		}
	}
	if r, g, b, a := img.At(512, 512).RGBA(); a != 0xffff || r>>8 != 0xdd || g>>8 != 0x33 || b>>8 != 0x33 {
		t.Errorf("centre is %x %x %x %x, want opaque #d33", r>>8, g>>8, b>>8, a>>8)
	}
	opaque := 0
	for y := 0; y < 1024; y += 8 {
		for x := 0; x < 1024; x += 8 {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0xffff {
				opaque++
			}
		}
	}
	// πr², r = 384 pixels, in samples of 8×8: about 7238.
	if opaque < 6900 || opaque > 7600 {
		t.Errorf("%d opaque samples, want about 7238", opaque)
	}

	// The whole build, cross-compiling: the icon has every size.
	toml := filepath.Join(dir, config.FileName)
	data, _ := os.ReadFile(toml)
	data = bytes.Replace(data, []byte(`# icon = "icon.png"`), []byte(`icon = "icon.svg"`), 1)
	os.WriteFile(toml, data, 0o644)
	exe, code, said := buildWindows(t, dir)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, said)
	}
	if !strings.Contains(said, "Drew icon.svg at 1024×1024") {
		t.Errorf("the drawing was not said:\n%s", said)
	}
	if info := readExe(t, exe); info.subsystem != pe.IMAGE_SUBSYSTEM_WINDOWS_GUI || info.types[3] != 9 || info.types[14] != 1 {
		t.Errorf("resources by type: %v", info.types)
	}
	if after := listDir(t, dir); strings.Join(after, " ") != strings.Join(before, " ") {
		t.Errorf("the project's files were %v, and are %v after", before, after)
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		if now, _ := os.ReadFile(filepath.Join(dir, name)); !bytes.Equal(now, mods[name]) {
			t.Errorf("%s changed:\n%s", name, now)
		}
	}
}

// A module that does not require svgx is told an SVG icon needs a ggui
// project, without building anything.
func TestSVGIconWithoutSVGX(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	buildEnv(t)
	dir := tiny(t)
	os.WriteFile(filepath.Join(dir, "icon.svg"), []byte(circleSVG), 0o644)
	os.WriteFile(filepath.Join(dir, config.FileName), []byte("[app]\nname = \"Tiny\"\nid = \"com.example.tiny\"\nicon = \"icon.svg\"\n"), 0o644)
	_, code, said := buildWindows(t, dir)
	if code == 0 || !strings.Contains(said, "does not require github.com/ironpark/svgx: an SVG icon needs a ggui project") {
		t.Errorf("exit %d: %s", code, said)
	}
}

// listDir lists the files and directories below dir, as relative paths.
func listDir(t *testing.T, dir string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		names = append(names, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return names
}
