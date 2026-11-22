package windows

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"encoding/xml"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf16"
)

// resKey names a resource: its type, ID and language.
type resKey struct{ typ, id, lang uint16 }

// walk reads a .rsrc section's directory tree as Windows does, from the
// root through types, IDs and languages to the data entries, and returns
// each resource's data. A data entry holds its data's address, which is
// rva, the section's address, more than its offset in sect: 0 in an
// object file, before the linker relocates it.
func walk(t *testing.T, sect []byte, rva uint32) map[resKey][]byte {
	t.Helper()
	le := binary.LittleEndian
	out := map[resKey][]byte{}
	var dir func(at uint32, depth int, path []uint16)
	dir = func(at uint32, depth int, path []uint16) {
		if int(at)+16 > len(sect) {
			t.Fatalf("directory at %#x runs past the section", at)
		}
		named, ids := le.Uint16(sect[at+12:]), le.Uint16(sect[at+14:])
		if named != 0 {
			t.Errorf("directory at %#x has %d named entries", at, named)
		}
		prev := -1
		for i := range uint32(ids) {
			e := at + 16 + 8*i
			id, to := le.Uint32(sect[e:]), le.Uint32(sect[e+4:])
			if id&0x80000000 != 0 || int(id) <= prev {
				t.Errorf("directory at %#x: entry %d has ID %#x after %d; IDs must ascend", at, i, id, prev)
			}
			prev = int(id)
			p := append(slices.Clone(path), uint16(id))
			switch {
			case to&0x80000000 != 0 && depth < 2:
				dir(to&^0x80000000, depth+1, p)
			case to&0x80000000 == 0 && depth == 2:
				addr, size := le.Uint32(sect[to:]), le.Uint32(sect[to+4:])
				off := addr - rva
				if addr < rva || int(off)+int(size) > len(sect) {
					t.Fatalf("resource %v: data at %#x, %d bytes, outside the section at %#x", p, addr, size, rva)
				}
				out[resKey{p[0], p[1], p[2]}] = sect[off : off+size]
			default:
				t.Fatalf("resource tree is not three levels deep at %v", p)
			}
		}
	}
	dir(0, 0, nil)
	return out
}

// pngFile writes a size×size icon, a disc on transparency.
func pngFile(t *testing.T, size int) string {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			if dx, dy := x-size/2, y-size/2; dx*dx+dy*dy < size*size/6 {
				img.SetNRGBA(x, y, color.NRGBA{30, 90, 220, 255})
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	path := filepath.Join(t.TempDir(), "icon.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func testResources(t *testing.T, arch string) Resources {
	return Resources{
		Arch: arch, Name: "Hello & Goodbye", Executable: "hello", ID: "com.example.hello",
		Version: "1.2.3", Company: "Example Ltd", Copyright: "© 2026 Example Ltd",
		Icon: pngFile(t, 512),
	}
}

// checkResources checks what a program or object carries: every icon
// image the group names, the version information and the manifest.
func checkResources(t *testing.T, res map[resKey][]byte) {
	t.Helper()
	le := binary.LittleEndian
	group, ok := res[resKey{rtGroupIcon, 1, langENUS}]
	if !ok {
		t.Fatalf("no icon group 1; resources %v", keys(res))
	}
	n := int(le.Uint16(group[4:]))
	if n != 9 || len(group) != 6+14*n {
		t.Fatalf("icon group has %d entries in %d bytes, want the 9 sizes", n, len(group))
	}
	icons := 0
	for k := range res {
		if k.typ == rtIcon {
			icons++
		}
	}
	if icons != n {
		t.Errorf("%d icons for a group of %d", icons, n)
	}
	for i := range n {
		e := group[6+14*i:]
		id := le.Uint16(e[12:])
		data, ok := res[resKey{rtIcon, id, langENUS}]
		if !ok || len(data) != int(le.Uint32(e[8:])) {
			t.Errorf("group entry %d names icon %d of %d bytes; found %v, %d bytes", i, id, le.Uint32(e[8:]), ok, len(data))
			continue
		}
		size := int(e[0])
		if size == 0 {
			if _, err := png.Decode(bytes.NewReader(data)); err != nil {
				t.Errorf("256-pixel icon: %v", err)
			}
		} else if int(le.Uint32(data[4:])) != size || int(le.Uint32(data[8:])) != 2*size {
			t.Errorf("icon %d: bitmap header says %d×%d, want %d×%d", id, le.Uint32(data[4:]), le.Uint32(data[8:]), size, 2*size)
		}
	}

	v := readVersion(t, res[resKey{rtVersion, 1, langENUS}])
	for k, want := range map[string]string{
		"FileDescription":  "Hello & Goodbye",
		"ProductName":      "Hello & Goodbye",
		"CompanyName":      "Example Ltd",
		"LegalCopyright":   "© 2026 Example Ltd",
		"FileVersion":      "1.2.3",
		"ProductVersion":   "1.2.3",
		"InternalName":     "hello",
		"OriginalFilename": "Hello & Goodbye.exe",
	} {
		if v.strings[k] != want {
			t.Errorf("version %s = %q, want %q", k, v.strings[k], want)
		}
	}
	if v.fileMS != 1<<16|2 || v.fileLS != 3<<16 || v.translation != [2]uint16{0x0409, 1200} {
		t.Errorf("version = %+v", v)
	}

	m := res[resKey{rtManifest, 1, langENUS}]
	if err := xml.Unmarshal(m, new(struct{})); err != nil {
		t.Errorf("manifest is not XML: %v", err)
	}
	for _, want := range []string{`name="com.example.hello" version="1.2.3.0"`, "PerMonitorV2", "Microsoft.Windows.Common-Controls", ">UTF-8<", "longPathAware"} {
		if !bytes.Contains(m, []byte(want)) {
			t.Errorf("manifest lacks %s", want)
		}
	}
}

func keys(m map[resKey][]byte) []resKey {
	var ks []resKey
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

type versionRead struct {
	fileMS, fileLS uint32
	strings        map[string]string
	translation    [2]uint16
}

// readVersion parses a VS_VERSIONINFO resource block by block, checking
// each block's length and alignment.
func readVersion(t *testing.T, data []byte) versionRead {
	t.Helper()
	le := binary.LittleEndian
	v := versionRead{strings: map[string]string{}}
	var read func(b []byte, path string)
	read = func(b []byte, path string) {
		length, valueLen, typ := int(le.Uint16(b)), int(le.Uint16(b[2:])), le.Uint16(b[4:])
		if length > len(b) || length < 6 {
			t.Fatalf("%s: block of %d bytes in %d", path, length, len(b))
		}
		b = b[:length]
		var key []uint16
		i := 6
		for ; le.Uint16(b[i:]) != 0; i += 2 {
			key = append(key, le.Uint16(b[i:]))
		}
		name := string(utf16.Decode(key))
		path += "/" + name
		i = int(align(uint32(i+2), 4))
		valueBytes := valueLen
		if typ == textValue {
			valueBytes *= 2
		}
		value := b[i : i+valueBytes]
		switch {
		case path == "/VS_VERSION_INFO":
			if valueLen != 52 || le.Uint32(value) != 0xFEEF04BD {
				t.Fatalf("fixed file info: %d bytes, signature %#x", valueLen, le.Uint32(value))
			}
			v.fileMS, v.fileLS = le.Uint32(value[8:]), le.Uint32(value[12:])
		case strings.HasPrefix(path, "/VS_VERSION_INFO/StringFileInfo/040904B0/"):
			u := make([]uint16, valueLen-1)
			for j := range u {
				u[j] = le.Uint16(value[2*j:])
			}
			if le.Uint16(value[2*len(u):]) != 0 {
				t.Errorf("%s is not NUL-terminated", path)
			}
			v.strings[name] = string(utf16.Decode(u))
		case path == "/VS_VERSION_INFO/VarFileInfo/Translation":
			v.translation = [2]uint16{le.Uint16(value), le.Uint16(value[2:])}
		}
		for i = int(align(uint32(i+valueBytes), 4)); i < length; {
			n := int(le.Uint16(b[i:]))
			read(b[i:], path)
			i = int(align(uint32(i+n), 4))
		}
	}
	read(data, "")
	return v
}

func TestObject(t *testing.T) {
	for _, arch := range Arches {
		t.Run(arch, func(t *testing.T) {
			obj, warnings, err := Object(testResources(t, arch))
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 0 {
				t.Errorf("warnings for a 512-pixel icon: %q", warnings)
			}
			f, err := pe.NewFile(bytes.NewReader(obj))
			if err != nil {
				t.Fatal(err)
			}
			if f.Machine != machine[arch].machine || len(f.Sections) != 1 || f.OptionalHeader != nil {
				t.Fatalf("object: machine %#x, %d sections, optional header %v", f.Machine, len(f.Sections), f.OptionalHeader)
			}
			s := f.Sections[0]
			if s.Name != ".rsrc" || s.Characteristics != pe.IMAGE_SCN_CNT_INITIALIZED_DATA|pe.IMAGE_SCN_MEM_READ {
				t.Errorf("section %q, characteristics %#x", s.Name, s.Characteristics)
			}
			// One symbol, the section's, with its auxiliary record.
			if len(f.COFFSymbols) != 2 || f.COFFSymbols[0].StorageClass != 3 || f.COFFSymbols[0].SectionNumber != 1 ||
				f.COFFSymbols[0].NumberOfAuxSymbols != 1 {
				t.Errorf("symbols = %+v", f.COFFSymbols)
			}
			if name, _ := f.COFFSymbols[0].FullName(f.StringTable); name != ".rsrc" {
				t.Errorf("symbol %q", name)
			}
			sect, err := s.Data()
			if err != nil {
				t.Fatal(err)
			}
			res := walk(t, sect, 0)
			checkResources(t, res)
			// A relocation for each resource, of the type that adds the
			// section's address, at its data entry's OffsetToData.
			if len(s.Relocs) != len(res) {
				t.Errorf("%d relocations for %d resources", len(s.Relocs), len(res))
			}
			for _, r := range s.Relocs {
				if r.Type != machine[arch].reloc || r.SymbolTableIndex != 0 {
					t.Errorf("relocation %+v", r)
				}
				at := binary.LittleEndian.Uint32(sect[r.VirtualAddress:])
				size := binary.LittleEndian.Uint32(sect[r.VirtualAddress+4:])
				found := false
				for _, d := range res {
					found = found || len(d) == int(size) && int(at)+len(d) <= len(sect) && bytes.Equal(sect[at:int(at)+len(d)], d)
				}
				if !found {
					t.Errorf("relocation at %#x is not at a data entry", r.VirtualAddress)
				}
			}
		})
	}
	if _, _, err := Object(Resources{Arch: "mips", Name: "x", Version: "1"}); err == nil {
		t.Error("an object for mips was made")
	}
}

func TestObjectWithoutIcon(t *testing.T) {
	r := testResources(t, "amd64")
	r.Icon, r.Description = "", "Says hello"
	obj, _, err := Object(r)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := pe.NewFile(bytes.NewReader(obj))
	sect, _ := f.Sections[0].Data()
	res := walk(t, sect, 0)
	if len(res) != 2 {
		t.Errorf("resources without an icon: %v, want the version and the manifest", keys(res))
	}
	if v := readVersion(t, res[resKey{rtVersion, 1, langENUS}]); v.strings["FileDescription"] != "Says hello" {
		t.Errorf("description = %q", v.strings["FileDescription"])
	}

	r.Icon = pngFile(t, 128)
	if _, warnings, err := Object(r); err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "[256]") {
		t.Errorf("a 128-pixel icon: %v, %q", err, warnings)
	}
}

func TestManifestReplaced(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, "app.manifest")
	text := `<?xml version="1.0" encoding="UTF-8"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges>
    <requestedExecutionLevel level="requireAdministrator"/>
  </requestedPrivileges></security></trustInfo>
</assembly>`
	os.WriteFile(own, []byte(text), 0o644)
	r := testResources(t, "arm64")
	r.Manifest = own
	obj, _, err := Object(r)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := pe.NewFile(bytes.NewReader(obj))
	sect, _ := f.Sections[0].Data()
	if got := walk(t, sect, 0)[resKey{rtManifest, 1, langENUS}]; string(got) != text {
		t.Errorf("manifest = %s", got)
	}

	for name, bad := range map[string]string{
		"broken.manifest": "<assembly xmlns=\"urn:schemas-microsoft-com:asm.v1\">\n<trustInfo>\n</assembly>",
		"html.manifest":   "<html></html>",
		"empty.manifest":  "",
	} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte(bad), 0o644)
		if _, err := ReadManifest(p); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for in, want := range map[string][4]uint16{"1": {1}, "1.2.3": {1, 2, 3}, "65535.0.0.7": {65535, 0, 0, 7}} {
		if got, err := ParseVersion(in); err != nil || got != want {
			t.Errorf("ParseVersion(%q) = %v, %v", in, got, err)
		}
	}
	for _, in := range []string{"", "20261118.1", "1.2.3.4.5", "1.x"} {
		if _, err := ParseVersion(in); err == nil {
			t.Errorf("ParseVersion(%q) succeeded", in)
		}
	}
}

func TestCheckFileName(t *testing.T) {
	for _, ok := range []string{"Hello", "Hello & Goodbye", "My App 2", "Ünïcødé 한글", "con-tool"} {
		if err := CheckFileName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{`Say "Hi"`, "a/b", "What?", "Trailing.", "CON", "nul.app", "tab\there"} {
		if err := CheckFileName(bad); err == nil {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestPlace(t *testing.T) {
	dir := t.TempDir()
	obj, _, err := Object(testResources(t, "amd64"))
	if err != nil {
		t.Fatal(err)
	}
	path, leftover, remove, err := Place(dir, "amd64", obj)
	if err != nil || leftover || filepath.Base(path) != "ggui_windows_amd64.syso" {
		t.Fatalf("Place = %s, %v, %v", path, leftover, err)
	}
	if !HasResources(path) {
		t.Error("the object written has no resources")
	}
	// A file a killed build left is replaced, and said to be.
	if _, leftover, _, err := Place(dir, "amd64", obj); err != nil || !leftover {
		t.Errorf("placing over a leftover: %v, %v", leftover, err)
	}
	if err := remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("the object is still there")
	}
	if err := remove(); err != nil {
		t.Errorf("removing again: %v", err)
	}

	// An object without a .rsrc section has no resources; one that is not
	// an object is taken to.
	plain := filepath.Join(dir, "code_windows_amd64.syso")
	noRsrc := bytes.Clone(obj)
	copy(noRsrc[20:28], ".text\x00\x00\x00")
	os.WriteFile(plain, noRsrc, 0o644)
	if HasResources(plain) {
		t.Error("an object with only .text has resources")
	}
	os.WriteFile(plain, []byte("not an object"), 0o644)
	if !HasResources(plain) {
		t.Error("a file that is no object was taken to have no resources")
	}
}
