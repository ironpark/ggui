package machosign

import (
	"bytes"
	"crypto/sha1"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ironpark/zapp/pkg/plist"
)

// programs caches the test programs by GOARCH, built once a run.
var programs struct {
	sync.Mutex
	dir  string
	bins map[string][]byte
}

// program returns a small Go program built for darwin/goarch, as the Go
// linker leaves it: signed by the linker on arm64, unsigned on amd64.
func program(t *testing.T, goarch string) []byte {
	t.Helper()
	if testing.Short() {
		t.Skip("builds programs")
	}
	programs.Lock()
	defer programs.Unlock()
	if b, ok := programs.bins[goarch]; ok {
		return b
	}
	if programs.bins == nil {
		dir, err := os.MkdirTemp("", "machosign-test-")
		if err != nil {
			t.Fatal(err)
		}
		programs.dir, programs.bins = dir, map[string][]byte{}
		os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module hello\n\ngo 1.22\n"), 0o644)
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n"), 0o644)
	}
	out := filepath.Join(programs.dir, "hello-"+goarch)
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, ".")
	cmd.Dir = programs.dir
	cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+goarch, "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=-mod=mod")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %s", msg)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	programs.bins[goarch] = b
	return b
}

func TestMain(m *testing.M) {
	code := m.Run()
	if programs.dir != "" {
		os.RemoveAll(programs.dir)
	}
	os.Exit(code)
}

const infoPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>Hello</string>
	<key>CFBundleIdentifier</key>
	<string>com.example.hello</string>
	<key>CFBundleName</key>
	<string>Hello</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
</dict>
</plist>
`

const entitlementsPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.get-task-allow</key>
	<true/>
	<key>com.apple.security.cs.allow-jit</key>
	<true/>
	<key>com.apple.security.app-sandbox</key>
	<false/>
	<key>com.apple.developer.associated-domains</key>
	<array>
		<string>applinks:example.com</string>
		<string>webcredentials:example.com</string>
	</array>
	<key>com.example.int</key>
	<integer>300</integer>
	<key>com.example.neg</key>
	<integer>-1</integer>
	<key>com.example.dict</key>
	<dict>
		<key>z</key>
		<string>last</string>
		<key>a</key>
		<integer>0</integer>
	</dict>
</dict>
</plist>
`

// makeBundle writes an unsigned Hello.app in dir around exe, with
// resources of each kind the default rules treat apart.
func makeBundle(t *testing.T, dir string, exe []byte) string {
	t.Helper()
	app := filepath.Join(dir, "Hello.app")
	c := filepath.Join(app, "Contents")
	files := map[string]string{
		"Info.plist":                           infoPlist,
		"PkgInfo":                              "APPL????",
		"Resources/AppIcon.icns":               strings.Repeat("icon", 700),
		"Resources/en.lproj/InfoPlist.strings": `"CFBundleName" = "Hello";` + "\n",
		"Resources/Base.lproj/Main.strings":    "base\n",
		"Resources/de.lproj/locversion.plist":  "omitted\n",
		"Resources/.DS_Store":                  "omitted\n",
		"Resources/sub/file.txt":               "hi\n",
		"Resources/B.txt":                      "upper\n",
		"Resources/a.txt":                      "lower\n",
		"Resources/sp ace&<x>.txt":             "escaped\n",
	}
	for name, data := range files {
		p := filepath.Join(c, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(c, "MacOS"), 0o755)
	if err := os.WriteFile(filepath.Join(c, "MacOS", "Hello"), exe, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("sub/file.txt", filepath.Join(c, "Resources", "link.txt")); err != nil {
		t.Fatal(err)
	}
	return app
}

// check re-reads a signed program with debug/macho, apart from how it
// was written, and recomputes every hash of its signature.
func check(t *testing.T, data []byte, id string, special map[int][]byte) {
	t.Helper()
	if binary.BigEndian.Uint32(data) == fatMagic {
		f, err := macho.NewFatFile(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range f.Arches {
			if a.Align != 14 || a.Offset%(1<<14) != 0 {
				t.Errorf("%v slice at %d aligned 2^%d, want 2^14", a.Cpu, a.Offset, a.Align)
			}
			checkThin(t, data[a.Offset:a.Offset+a.Size], id, special)
		}
		return
	}
	checkThin(t, data, id, special)
}

func checkThin(t *testing.T, data []byte, id string, special map[int][]byte) {
	t.Helper()
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	le, be := binary.LittleEndian, binary.BigEndian
	var sigOff, sigSize uint32
	for _, l := range f.Loads {
		raw := l.Raw()
		if le.Uint32(raw) == lcCodeSignature {
			sigOff, sigSize = le.Uint32(raw[8:]), le.Uint32(raw[12:])
		}
	}
	if sigOff == 0 || sigOff%16 != 0 || sigSize%16 != 0 {
		t.Fatalf("%v: signature at %d, %d bytes", f.Cpu, sigOff, sigSize)
	}
	link, text := f.Segment("__LINKEDIT"), f.Segment("__TEXT")
	if end := link.Offset + link.Filesz; end != uint64(sigOff+sigSize) || end != uint64(len(data)) {
		t.Errorf("%v: __LINKEDIT ends at %d, the signature at %d, the file at %d", f.Cpu, end, sigOff+sigSize, len(data))
	}
	if link.Memsz%(1<<14) != 0 || link.Memsz < link.Filesz {
		t.Errorf("%v: __LINKEDIT vmsize %#x for filesize %#x", f.Cpu, link.Memsz, link.Filesz)
	}

	sig := data[sigOff : sigOff+sigSize]
	if be.Uint32(sig) != magicSuperBlob {
		t.Fatalf("%v: no superblob", f.Cpu)
	}
	blobs := map[uint32][]byte{}
	var order []uint32
	for i := range be.Uint32(sig[8:]) {
		slot, off := be.Uint32(sig[12+8*i:]), be.Uint32(sig[16+8*i:])
		blobs[slot] = sig[off : off+be.Uint32(sig[off+4:])]
		order = append(order, slot)
	}
	want := []uint32{0, 2, 0x10000}
	if special[5] != nil {
		want = []uint32{0, 2, 5, 7, 0x10000}
	}
	if fmt.Sprint(order) != fmt.Sprint(want) {
		t.Errorf("%v: slots %v, want %v", f.Cpu, order, want)
	}
	if !bytes.Equal(blobs[2], emptyRequirements) || !bytes.Equal(blobs[0x10000], []byte{0xfa, 0xde, 0x0b, 0x01, 0, 0, 0, 8}) {
		t.Errorf("%v: requirements %x, CMS %x", f.Cpu, blobs[2], blobs[0x10000])
	}

	cd := blobs[0]
	shift := uint8(12)
	if f.Cpu == macho.CpuArm64 {
		shift = 14
	}
	hashOff, identOff := be.Uint32(cd[16:]), be.Uint32(cd[20:])
	nSpecial, nCode, limit := int(be.Uint32(cd[24:])), int(be.Uint32(cd[28:])), be.Uint32(cd[32:])
	if v, flags := be.Uint32(cd[8:]), be.Uint32(cd[12:]); v != 0x20400 || flags != 2 {
		t.Errorf("%v: code directory version %#x flags %#x", f.Cpu, v, flags)
	}
	if cd[36] != 32 || cd[37] != 2 || cd[39] != shift {
		t.Errorf("%v: hash size %d type %d page 2^%d", f.Cpu, cd[36], cd[37], cd[39])
	}
	if got := string(cd[identOff : int(identOff)+bytes.IndexByte(cd[identOff:], 0)]); got != id {
		t.Errorf("%v: identifier %q, want %q", f.Cpu, got, id)
	}
	if limit != sigOff {
		t.Errorf("%v: code limit %d, signature at %d", f.Cpu, limit, sigOff)
	}
	if base, lim, fl := be.Uint64(cd[64:]), be.Uint64(cd[72:]), be.Uint64(cd[80:]); base != text.Offset || lim != text.Filesz || fl != 1 {
		t.Errorf("%v: executable segment %#x+%#x flags %#x", f.Cpu, base, lim, fl)
	}
	page := 1 << shift
	if nCode != (int(limit)+page-1)/page {
		t.Errorf("%v: %d code slots for %d bytes", f.Cpu, nCode, limit)
	}
	for i := range nCode {
		h := sha256.Sum256(data[i*page : min((i+1)*page, int(limit))])
		if !bytes.Equal(cd[int(hashOff)+32*i:][:32], h[:]) {
			t.Fatalf("%v: page %d hash is wrong", f.Cpu, i)
		}
	}
	special = maps.Clone(special)
	special[2] = blobs[2]
	if ent := special[5]; ent != nil {
		der, err := DEREntitlements(ent)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(blobs[5], wrap(magicEntitlements, ent)) || !bytes.Equal(blobs[7], wrap(magicDER, der)) {
			t.Errorf("%v: the entitlements are not as given", f.Cpu)
		}
		special[5], special[7] = blobs[5], blobs[7]
	}
	wantN := 0
	for slot := range special {
		wantN = max(wantN, slot)
	}
	if nSpecial != wantN {
		t.Errorf("%v: %d special slots, want %d", f.Cpu, nSpecial, wantN)
	}
	for slot := 1; slot <= nSpecial; slot++ {
		var want [32]byte
		if v, ok := special[slot]; ok {
			want = sha256.Sum256(v)
		}
		if !bytes.Equal(cd[int(hashOff)-32*slot:][:32], want[:]) {
			t.Errorf("%v: special slot %d hash is wrong", f.Cpu, slot)
		}
	}
}

func TestSignProgram(t *testing.T) {
	for _, arch := range []string{"arm64", "amd64"} {
		t.Run(arch, func(t *testing.T) {
			signed, err := Sign(program(t, arch), Options{Identifier: "com.example.hello"})
			if err != nil {
				t.Fatal(err)
			}
			check(t, signed, "com.example.hello", map[int][]byte{})
			// Signing again replaces the signature with the same one.
			again, err := Sign(signed, Options{Identifier: "com.example.hello"})
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, signed) {
				t.Error("signing a signed program again changed it")
			}
		})
	}
}

func TestLipo(t *testing.T) {
	arm, amd := program(t, "arm64"), program(t, "amd64")
	fat, err := Fat(arm, amd)
	if err != nil {
		t.Fatal(err)
	}
	f, err := macho.NewFatFile(bytes.NewReader(fat))
	if err != nil {
		t.Fatal(err)
	}
	// lipo puts the slice with the smaller alignment first.
	if len(f.Arches) != 2 || f.Arches[0].Cpu != macho.CpuAmd64 || f.Arches[1].Cpu != macho.CpuArm64 {
		t.Fatalf("slices %+v", f.Arches)
	}
	for i, want := range []struct {
		align uint32
		data  []byte
	}{{12, amd}, {14, arm}} {
		a := f.Arches[i]
		if a.Align != want.align || a.Offset%(1<<a.Align) != 0 || !bytes.Equal(fat[a.Offset:a.Offset+a.Size], want.data) {
			t.Errorf("slice %d: offset %d align %d", i, a.Offset, a.Align)
		}
	}
	if _, err := Fat(arm, arm); err == nil {
		t.Error("joined two arm64 programs")
	}

	signed, err := Sign(fat, Options{Identifier: "com.example.hello"})
	if err != nil {
		t.Fatal(err)
	}
	check(t, signed, "com.example.hello", map[int][]byte{})
}

func TestSignBundle(t *testing.T) {
	for _, tc := range []struct {
		name         string
		exe          func(t *testing.T) []byte
		entitlements []byte
	}{
		{"arm64", func(t *testing.T) []byte { return program(t, "arm64") }, nil},
		{"amd64", func(t *testing.T) []byte { return program(t, "amd64") }, nil},
		{"universal", func(t *testing.T) []byte {
			fat, err := Fat(program(t, "arm64"), program(t, "amd64"))
			if err != nil {
				t.Fatal(err)
			}
			return fat
		}, nil},
		{"entitlements", func(t *testing.T) []byte { return program(t, "arm64") }, []byte(entitlementsPlist)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := makeBundle(t, t.TempDir(), tc.exe(t))
			if err := SignBundle(app, tc.entitlements); err != nil {
				t.Fatal(err)
			}
			c := filepath.Join(app, "Contents")
			seal, err := os.ReadFile(filepath.Join(c, "_CodeSignature", "CodeResources"))
			if err != nil {
				t.Fatal(err)
			}
			checkSeal(t, c, seal)
			exe, _ := os.ReadFile(filepath.Join(c, "MacOS", "Hello"))
			special := map[int][]byte{1: []byte(infoPlist), 3: seal}
			if tc.entitlements != nil {
				special[5] = tc.entitlements
			}
			check(t, exe, "com.example.hello", special)
		})
	}
}

// checkSeal checks CodeResources against the files of the bundle.
func checkSeal(t *testing.T, contents string, seal []byte) {
	t.Helper()
	d, err := plist.ParseDict(seal)
	if err != nil {
		t.Fatal(err)
	}
	files, files2 := d["files"].(map[string]any), d["files2"].(map[string]any)
	read := func(name string) []byte {
		b, err := os.ReadFile(filepath.Join(contents, filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, name := range []string{"Resources/AppIcon.icns", "Resources/sub/file.txt", "Resources/a.txt", "Resources/B.txt", "Resources/sp ace&<x>.txt", "Resources/Base.lproj/Main.strings"} {
		h1, h2 := sha1.Sum(read(name)), sha256.Sum256(read(name))
		if got, _ := files[name].([]byte); !bytes.Equal(got, h1[:]) {
			t.Errorf("files[%s] = %v", name, files[name])
		}
		if got, _ := files2[name].(map[string]any)["hash2"].([]byte); !bytes.Equal(got, h2[:]) {
			t.Errorf("files2[%s] = %v", name, files2[name])
		}
	}
	lproj := files2["Resources/en.lproj/InfoPlist.strings"].(map[string]any)
	if lproj["optional"] != true || files["Resources/en.lproj/InfoPlist.strings"].(map[string]any)["optional"] != true {
		t.Error("the .lproj file is not optional")
	}
	if link := files2["Resources/link.txt"].(map[string]any); link["symlink"] != "sub/file.txt" {
		t.Errorf("symlink sealed as %v", link)
	}
	if _, ok := files["Resources/link.txt"]; ok {
		t.Error("the first version of the seal holds a symlink")
	}
	for _, name := range []string{"Info.plist", "PkgInfo", "MacOS/Hello", "Resources/.DS_Store", "Resources/de.lproj/locversion.plist", "_CodeSignature/CodeResources"} {
		// The first version's rules, as codesign writes them, do not
		// leave out .DS_Store.
		if (files[name] != nil && name != "Resources/.DS_Store") || files2[name] != nil {
			t.Errorf("%s is sealed", name)
		}
	}
	if len(d["rules"].(map[string]any)) != 5 || len(d["rules2"].(map[string]any)) != 13 {
		t.Error("the rules are not codesign's")
	}
}

func TestSignBundleNested(t *testing.T) {
	app := makeBundle(t, t.TempDir(), program(t, "arm64"))
	c := filepath.Join(app, "Contents")
	os.WriteFile(filepath.Join(c, "MacOS", "helper"), program(t, "amd64"), 0o755)
	os.MkdirAll(filepath.Join(c, "Frameworks"), 0o755)
	fat, _ := Fat(program(t, "arm64"), program(t, "amd64"))
	os.WriteFile(filepath.Join(c, "Frameworks", "libhello.dylib"), fat, 0o755)
	if err := SignBundle(app, nil); err != nil {
		t.Fatal(err)
	}
	seal, _ := os.ReadFile(filepath.Join(c, "_CodeSignature", "CodeResources"))
	d, _ := plist.ParseDict(seal)
	files2 := d["files2"].(map[string]any)
	for name, id := range map[string]string{"MacOS/helper": "helper-", "Frameworks/libhello.dylib": "libhello-"} {
		data, _ := os.ReadFile(filepath.Join(c, filepath.FromSlash(name)))
		uuid := programUUID(data)
		check(t, data, id+hex.EncodeToString(append([]byte("UUID"), uuid...)), map[int][]byte{})
		hashes, err := cdHashes(data)
		if err != nil {
			t.Fatal(err)
		}
		e := files2[name].(map[string]any)
		var reqs []string
		for _, h := range hashes {
			reqs = append(reqs, fmt.Sprintf(`cdhash H"%x"`, h.hash))
		}
		if !bytes.Equal(e["cdhash"].([]byte), hashes[0].hash) || e["requirement"] != strings.Join(reqs, " or ") {
			t.Errorf("%s sealed as %v", name, e)
		}
	}

	t.Run("not code", func(t *testing.T) {
		os.WriteFile(filepath.Join(c, "MacOS", "notes.txt"), []byte("x"), 0o644)
		defer os.Remove(filepath.Join(c, "MacOS", "notes.txt"))
		if err := SignBundle(app, nil); err == nil || !strings.Contains(err.Error(), "MacOS/notes.txt") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("nested bundle", func(t *testing.T) {
		os.MkdirAll(filepath.Join(c, "Frameworks", "Foo.framework"), 0o755)
		defer os.RemoveAll(filepath.Join(c, "Frameworks", "Foo.framework"))
		if err := SignBundle(app, nil); err == nil || !strings.Contains(err.Error(), "Foo.framework") {
			t.Errorf("err = %v", err)
		}
	})
}

// The DER encoding of entitlementsPlist, as codesign -d --entitlements -
// --der printed it on macOS 26 for a bundle codesign signed with them.
const entitlementsDER = "7082012e020101b0820127305c0c26636f6d2e6170706c652e646576656c6f7065722e6173736f6369617465642d646f6d61696e7330320c146170706c696e6b733a6578616d706c652e636f6d0c1a77656263726564656e7469616c733a6578616d706c652e636f6d30230c1e636f6d2e6170706c652e73656375726974792e6170702d73616e64626f78010100" +
	"30240c1f636f6d2e6170706c652e73656375726974792e63732e616c6c6f772d6a69740101ff30260c21636f6d2e6170706c652e73656375726974792e6765742d7461736b2d616c6c6f770101ff30270c10636f6d2e6578616d706c652e64696374b01330060c0161020100" +
	"30090c017a0c046c61737430150c0f636f6d2e6578616d706c652e696e740202012c30140c0f636f6d2e6578616d706c652e6e65670201ff"

func TestDEREntitlements(t *testing.T) {
	got, err := DEREntitlements([]byte(entitlementsPlist))
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(got) != entitlementsDER {
		t.Errorf("DER\n got %x\nwant %s", got, entitlementsDER)
	}
	for _, c := range []struct {
		n    int64
		want string
	}{{0, "00"}, {127, "7f"}, {128, "0080"}, {-128, "80"}, {-129, "ff7f"}, {255, "00ff"}, {256, "0100"}} {
		v, _ := derValue(c.n)
		if got := hex.EncodeToString(v[2:]); got != c.want {
			t.Errorf("%d: %s, want %s", c.n, got, c.want)
		}
	}
	if _, err := DEREntitlements([]byte(`<plist version="1.0"><dict><key>k</key><real>1.5</real></dict></plist>`)); err == nil {
		t.Error("encoded a real")
	}
}

func TestNotMachO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x")
	os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755)
	if err := SignFile(p, Options{Identifier: "x"}); err == nil {
		t.Error("signed a script")
	}
	// A Java class shares the universal magic number.
	if IsMachO([]byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 52}) {
		t.Error("took a class file for a universal program")
	}
}
