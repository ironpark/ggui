package updater

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// -----------------------------------------------------------------------------
// Archive test builders
// -----------------------------------------------------------------------------

type testArchiveEntry struct {
	Name       string
	Body       []byte
	Mode       fs.FileMode
	IsDir      bool
	LinkTarget string // non-empty => symlink
}

func writeTestZip(t *testing.T, path string, entries []testArchiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating zip %s: %v", path, err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for _, e := range entries {
		hdr := &zip.FileHeader{
			Name:   e.Name,
			Method: zip.Deflate,
		}
		switch {
		case e.LinkTarget != "":
			hdr.SetMode(fs.ModeSymlink | 0o777)
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				t.Fatalf("creating zip symlink header %s: %v", e.Name, err)
			}
			if _, err := w.Write([]byte(e.LinkTarget)); err != nil {
				t.Fatalf("writing zip symlink %s: %v", e.Name, err)
			}
		case e.IsDir:
			if !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
			mode := e.Mode
			if mode == 0 {
				mode = 0o755
			}
			hdr.SetMode(fs.ModeDir | mode)
			if _, err := zw.CreateHeader(hdr); err != nil {
				t.Fatalf("creating zip dir %s: %v", e.Name, err)
			}
		default:
			mode := e.Mode
			if mode == 0 {
				mode = 0o644
			}
			hdr.SetMode(mode)
			w, err := zw.CreateHeader(hdr)
			if err != nil {
				t.Fatalf("creating zip file %s: %v", e.Name, err)
			}
			if _, err := w.Write(e.Body); err != nil {
				t.Fatalf("writing zip file %s: %v", e.Name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip %s: %v", path, err)
	}
}

func writeTestTarGz(t *testing.T, path string, entries []testArchiveEntry) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating tar.gz %s: %v", path, err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		switch {
		case e.LinkTarget != "":
			hdr := &tar.Header{
				Name:     e.Name,
				Typeflag: tar.TypeSymlink,
				Linkname: e.LinkTarget,
				Mode:     0o777,
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("writing tar symlink %s: %v", e.Name, err)
			}
		case e.IsDir:
			mode := e.Mode
			if mode == 0 {
				mode = 0o755
			}
			hdr := &tar.Header{
				Name:     e.Name,
				Typeflag: tar.TypeDir,
				Mode:     int64(mode),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("writing tar dir %s: %v", e.Name, err)
			}
		default:
			mode := e.Mode
			if mode == 0 {
				mode = 0o644
			}
			hdr := &tar.Header{
				Name:     e.Name,
				Typeflag: tar.TypeReg,
				Mode:     int64(mode),
				Size:     int64(len(e.Body)),
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatalf("writing tar file header %s: %v", e.Name, err)
			}
			if _, err := tw.Write(e.Body); err != nil {
				t.Fatalf("writing tar file body %s: %v", e.Name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar %s: %v", path, err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing gzip %s: %v", path, err)
	}
}

func makeTestPlist(bundleID, extra string) []byte {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0"><dict>` + "\n")
	if extra != "" {
		b.WriteString(extra)
		b.WriteByte('\n')
	}
	if bundleID != "" {
		b.WriteString(`<key>CFBundleIdentifier</key><string>`)
		b.WriteString(bundleID)
		b.WriteString(`</string>` + "\n")
	}
	b.WriteString(`</dict></plist>` + "\n")
	return []byte(b.String())
}

func createFakeAppBundle(t *testing.T, appPath, bundleID, binContent string) {
	t.Helper()
	macOSDir := filepath.Join(appPath, "Contents", "MacOS")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", macOSDir, err)
	}
	if err := os.WriteFile(filepath.Join(appPath, "Contents", "Info.plist"), makeTestPlist(bundleID, ""), 0o644); err != nil {
		t.Fatalf("writing Info.plist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(macOSDir, "MyApp"), []byte(binContent), 0o755); err != nil {
		t.Fatalf("writing binary: %v", err)
	}
}

// -----------------------------------------------------------------------------
// DefaultApplier registration & shared helpers tests
// -----------------------------------------------------------------------------

func TestApplyDefaultApplierRegistration(t *testing.T) {
	if defaultApplierFunc == nil {
		t.Fatalf("defaultApplierFunc = nil; want DefaultApplier registered in init()")
	}
	if ap := DefaultApplier(); ap == nil {
		t.Fatalf("DefaultApplier() = nil; want non-nil platform applier")
	}
}

func TestExtractSubpathAndCopyFileAtomic(t *testing.T) {
	root := t.TempDir()
	if !isSubpath(root, root) {
		t.Errorf("isSubpath(root, root) = false; want true")
	}
	if !isSubpath(root, filepath.Join(root, "a", "b.txt")) {
		t.Errorf("isSubpath(root, child) = false; want true")
	}
	if !isSubpath(root, filepath.Join(root, "..hidden")) {
		t.Errorf("isSubpath(root, ..hidden) = false; want true")
	}
	if isSubpath(root, filepath.Dir(root)) {
		t.Errorf("isSubpath(root, parent) = true; want false")
	}
	if isSubpath(root, root+"-sibling") {
		t.Errorf("isSubpath(root, root-sibling) = true; want false")
	}
	if isSubpath(root, filepath.Join(root, "bad\x00name")) {
		t.Errorf("isSubpath(root, NUL) = true; want false")
	}

	src := filepath.Join(root, "src.bin")
	dst := filepath.Join(root, "nested", "dst.bin")
	if err := os.WriteFile(src, []byte("atomic-payload"), 0o600); err != nil {
		t.Fatalf("WriteFile src: %v", err)
	}
	if err := copyFileAtomic(src, dst, 0o755); err != nil {
		t.Fatalf("copyFileAtomic error: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile dst: %v", err)
	}
	if string(got) != "atomic-payload" {
		t.Errorf("dst content = %q; want atomic-payload", string(got))
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(dst)
		if err != nil {
			t.Fatalf("Stat dst: %v", err)
		}
		if fi.Mode().Perm() != 0o755 {
			t.Errorf("dst mode = %o; want 0755", fi.Mode().Perm())
		}
	}
}

// -----------------------------------------------------------------------------
// extractZipSafe tests
// -----------------------------------------------------------------------------

func TestExtractZipSafe(t *testing.T) {
	t.Run("normal permissions and internal symlinks", func(t *testing.T) {
		tmp := t.TempDir()
		zipPath := filepath.Join(tmp, "ok.zip")
		dst := filepath.Join(tmp, "out")

		writeTestZip(t, zipPath, []testArchiveEntry{
			{Name: "MyApp.app/Contents/MacOS/", IsDir: true, Mode: 0o755},
			{Name: "MyApp.app/Contents/MacOS/MyApp", Body: []byte("#!/bin/sh\necho hi\n"), Mode: 0o755},
			{Name: "MyApp.app/Contents/Resources/data.txt", Body: []byte("hello"), Mode: 0o640},
			{Name: "MyApp.app/Contents/Frameworks/Versions/A/Lib", Body: []byte("libdata"), Mode: 0o644},
			{Name: "MyApp.app/Contents/Frameworks/Versions/Current", LinkTarget: "A"},
			{Name: "MyApp.app/Contents/Frameworks/Lib", LinkTarget: "Versions/Current/Lib"},
		})

		if err := extractZipSafe(zipPath, dst); err != nil {
			t.Fatalf("extractZipSafe error: %v", err)
		}

		binPath := filepath.Join(dst, "MyApp.app/Contents/MacOS/MyApp")
		binData, err := os.ReadFile(binPath)
		if err != nil || string(binData) != "#!/bin/sh\necho hi\n" {
			t.Fatalf("extracted bin = (%q, %v); want script", string(binData), err)
		}
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(binPath)
			if err != nil {
				t.Fatalf("Stat bin: %v", err)
			}
			if fi.Mode().Perm() != 0o755 {
				t.Errorf("bin mode = %o; want 0755", fi.Mode().Perm())
			}

			linkPath := filepath.Join(dst, "MyApp.app/Contents/Frameworks/Lib")
			lfi, err := os.Lstat(linkPath)
			if err != nil {
				t.Fatalf("Lstat symlink: %v", err)
			}
			if lfi.Mode()&fs.ModeSymlink == 0 {
				t.Errorf("expected symlink at %s", linkPath)
			}
			gotViaLink, err := os.ReadFile(linkPath)
			if err != nil || string(gotViaLink) != "libdata" {
				t.Errorf("read via symlink = (%q, %v); want libdata", string(gotViaLink), err)
			}
		}
	})

	t.Run("zip-slip path traversal rejected", func(t *testing.T) {
		badNames := []string{
			"../evil.txt",
			"sub/../../evil.txt",
			"/etc/passwd",
			`..\evil.txt`,
			`C:\evil.txt`,
			"bad\x00file.txt",
		}
		for _, name := range badNames {
			tmp := t.TempDir()
			zipPath := filepath.Join(tmp, "slip.zip")
			dst := filepath.Join(tmp, "out")
			writeTestZip(t, zipPath, []testArchiveEntry{
				{Name: name, Body: []byte("pwned"), Mode: 0o644},
			})
			if err := extractZipSafe(zipPath, dst); err == nil {
				t.Errorf("extractZipSafe(%q) = nil; want path traversal error", name)
			}
			if _, err := os.Stat(filepath.Join(tmp, "evil.txt")); err == nil {
				t.Errorf("evil.txt escaped outside dst for entry %q", name)
			}
		}
	})

	t.Run("symlink escape rejected", func(t *testing.T) {
		cases := []struct {
			name    string
			entries []testArchiveEntry
		}{
			{
				name: "relative parent escape",
				entries: []testArchiveEntry{
					{Name: "link", LinkTarget: "../../etc/passwd"},
				},
			},
			{
				name: "absolute symlink target",
				entries: []testArchiveEntry{
					{Name: "link", LinkTarget: "/etc/passwd"},
				},
			},
			{
				name: "nested symlink chain escape",
				entries: []testArchiveEntry{
					{Name: "a/b", LinkTarget: ".."},
					{Name: "a/b/escape", LinkTarget: "../../outside"},
				},
			},
			{
				name: "two-stage out-of-order symlink escape",
				entries: []testArchiveEntry{
					{Name: "a/hop", LinkTarget: "../pivot/.."},
					{Name: "pivot", LinkTarget: "."},
				},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				tmp := t.TempDir()
				zipPath := filepath.Join(tmp, "sym.zip")
				dst := filepath.Join(tmp, "out")
				writeTestZip(t, zipPath, tc.entries)
				err := extractZipSafe(zipPath, dst)
				if tc.name == "two-stage out-of-order symlink escape" {
					// pivot -> . means a/hop -> ../pivot/.. -> root/.. -> ESCAPES!
					if err == nil {
						t.Errorf("expected symlink chain escape to be rejected")
					}
					return
				}
				if err == nil {
					t.Errorf("extractZipSafe(%s) = nil; want symlink escape error", tc.name)
				}
			})
		}
	})

	t.Run("size limit bomb guard", func(t *testing.T) {
		tmp := t.TempDir()
		zipPath := filepath.Join(tmp, "bomb.zip")
		dst := filepath.Join(tmp, "out")
		// Highly compressible 64 KiB of zeroes, capped at 4 KiB limit.
		writeTestZip(t, zipPath, []testArchiveEntry{
			{Name: "huge.bin", Body: bytes.Repeat([]byte("A"), 64*1024), Mode: 0o644},
		})
		err := extractZipSafeWithLimit(zipPath, dst, 4*1024)
		if err == nil {
			t.Fatalf("extractZipSafeWithLimit = nil; want size limit error")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "size") {
			t.Errorf("unexpected error message: %v", err)
		}
	})
}

// -----------------------------------------------------------------------------
// extractTarGzSafe tests
// -----------------------------------------------------------------------------

func TestExtractTarGzSafe(t *testing.T) {
	t.Run("normal extraction preserves mode and safe symlinks", func(t *testing.T) {
		tmp := t.TempDir()
		tgz := filepath.Join(tmp, "app.tar.gz")
		dst := filepath.Join(tmp, "out")

		writeTestTarGz(t, tgz, []testArchiveEntry{
			{Name: ".", IsDir: true, Mode: 0o755},
			{Name: "usr/bin", IsDir: true, Mode: 0o755},
			{Name: "usr/bin/myapp", Body: []byte("elf-binary"), Mode: 0o755},
			{Name: "usr/lib/myapp/libggfx_driver.so", Body: []byte("so-data"), Mode: 0o644},
			{Name: "usr/bin/myapp-link", LinkTarget: "myapp"},
		})

		if err := extractTarGzSafe(tgz, dst); err != nil {
			t.Fatalf("extractTarGzSafe error: %v", err)
		}

		binPath := filepath.Join(dst, "usr/bin/myapp")
		got, err := os.ReadFile(binPath)
		if err != nil || string(got) != "elf-binary" {
			t.Fatalf("extracted bin = (%q, %v); want elf-binary", string(got), err)
		}
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(binPath)
			if err != nil {
				t.Fatalf("Stat bin: %v", err)
			}
			if fi.Mode().Perm() != 0o755 {
				t.Errorf("bin mode = %o; want 0755", fi.Mode().Perm())
			}
		}
	})

	t.Run("path traversal rejected", func(t *testing.T) {
		for _, bad := range []string{"../evil", "a/../../evil", "/etc/passwd", `..\evil`} {
			tmp := t.TempDir()
			tgz := filepath.Join(tmp, "bad.tar.gz")
			dst := filepath.Join(tmp, "out")
			writeTestTarGz(t, tgz, []testArchiveEntry{
				{Name: bad, Body: []byte("bad"), Mode: 0o644},
			})
			if err := extractTarGzSafe(tgz, dst); err == nil {
				t.Errorf("extractTarGzSafe(%q) = nil; want error", bad)
			}
		}
	})

	t.Run("symlink escape rejected", func(t *testing.T) {
		cases := []struct {
			entry  string
			target string
		}{
			{entry: "evil", target: "../../etc/passwd"},
			{entry: "usr/bin/evil", target: "../../../etc/passwd"},
			{entry: "usr/bin/abs", target: "/etc/passwd"},
		}
		for _, tc := range cases {
			tmp := t.TempDir()
			tgz := filepath.Join(tmp, "sym.tar.gz")
			dst := filepath.Join(tmp, "out")
			writeTestTarGz(t, tgz, []testArchiveEntry{
				{Name: tc.entry, LinkTarget: tc.target},
			})
			if err := extractTarGzSafe(tgz, dst); err == nil {
				t.Errorf("extractTarGzSafe(%q -> %q) = nil; want error", tc.entry, tc.target)
			}
		}
	})

	t.Run("size limit bomb guard", func(t *testing.T) {
		tmp := t.TempDir()
		tgz := filepath.Join(tmp, "bomb.tar.gz")
		dst := filepath.Join(tmp, "out")
		writeTestTarGz(t, tgz, []testArchiveEntry{
			{Name: "big.bin", Body: bytes.Repeat([]byte("Z"), 64*1024), Mode: 0o644},
		})
		if err := extractTarGzSafeWithLimit(tgz, dst, 4*1024); err == nil {
			t.Fatalf("extractTarGzSafeWithLimit = nil; want size limit error")
		}
	})

	t.Run("PAX header decompressed stream bomb rejected", func(t *testing.T) {
		tmp := t.TempDir()
		tgz := filepath.Join(tmp, "pax-bomb.tar.gz")
		dst := filepath.Join(tmp, "out")

		f, err := os.Create(tgz)
		if err != nil {
			t.Fatal(err)
		}
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		// Write >128 KiB of PAX header metadata with tiny regular file bodies;
		// tarStreamLimit(4 KiB) caps decompressed stream at 68 KiB + 1.
		bigVal := strings.Repeat("x", 16*1024)
		for i := range 12 {
			hdr := &tar.Header{
				Name:       "empty-" + string(rune('a'+i)),
				Typeflag:   tar.TypeReg,
				Mode:       0o644,
				Size:       0,
				Format:     tar.FormatPAX,
				PAXRecords: map[string]string{"SCHILY.xattr.user.pad": bigVal},
			}
			if err := tw.WriteHeader(hdr); err != nil {
				t.Fatal(err)
			}
		}
		_ = tw.Close()
		_ = gz.Close()
		_ = f.Close()

		err = extractTarGzSafeWithLimit(tgz, dst, 4*1024)
		if err == nil {
			t.Fatalf("extractTarGzSafeWithLimit(PAX bomb) = nil; want stream size limit error")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "size") {
			t.Errorf("unexpected PAX bomb error: %v", err)
		}
	})
}

func TestExtractCopyDirSafeAndRemoveAllForce(t *testing.T) {
	t.Run("copyDirSafeWithLimit enforces size, entry count, and symlink containment", func(t *testing.T) {
		src := t.TempDir()
		if err := os.WriteFile(filepath.Join(src, "a.bin"), bytes.Repeat([]byte("A"), 2048), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(src, "b.bin"), bytes.Repeat([]byte("B"), 2048), 0o644); err != nil {
			t.Fatal(err)
		}
		// Exceeding byte limit fails.
		if err := copyDirSafeWithLimit(src, filepath.Join(t.TempDir(), "dst-size"), 1024, 100); err == nil {
			t.Errorf("copyDirSafeWithLimit(size exceeded) = nil; want error")
		}
		// Exceeding entry limit fails.
		if err := copyDirSafeWithLimit(src, filepath.Join(t.TempDir(), "dst-entries"), 1<<20, 1); err == nil {
			t.Errorf("copyDirSafeWithLimit(entries exceeded) = nil; want error")
		}

		if runtime.GOOS != "windows" {
			// Safe internal symlink succeeds.
			safeSrc := t.TempDir()
			_ = os.WriteFile(filepath.Join(safeSrc, "real.txt"), []byte("ok"), 0o644)
			_ = os.Symlink("real.txt", filepath.Join(safeSrc, "link.txt"))
			safeDst := filepath.Join(t.TempDir(), "dst-safe")
			if err := copyDirSafeWithLimit(safeSrc, safeDst, 1<<20, 100); err != nil {
				t.Fatalf("copyDirSafeWithLimit(safe symlink) error = %v", err)
			}

			// Escaping symlink is rejected and not left on disk.
			evilSrc := t.TempDir()
			_ = os.Symlink("../../etc/passwd", filepath.Join(evilSrc, "escape"))
			evilDst := filepath.Join(t.TempDir(), "dst-evil")
			if err := copyDirSafeWithLimit(evilSrc, evilDst, 1<<20, 100); err == nil {
				t.Errorf("copyDirSafeWithLimit(escaping symlink) = nil; want error")
			}
			if _, err := os.Lstat(filepath.Join(evilDst, "escape")); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("escaping symlink left behind at destination")
			}
		}
	})

	t.Run("removeAllForce cleans read-only directories without following symlinks", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX permission test")
		}
		outside := t.TempDir()
		outsideFile := filepath.Join(outside, "keep.txt")
		if err := os.WriteFile(outsideFile, []byte("safe"), 0o400); err != nil {
			t.Fatal(err)
		}

		root := filepath.Join(t.TempDir(), "ro-tree")
		sub := filepath.Join(root, "locked")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "ro.txt"), []byte("x"), 0o400); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outsideFile, filepath.Join(sub, "ext-link")); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(sub, 0o500); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(root, 0o500); err != nil {
			t.Fatal(err)
		}

		if err := removeAllForce(root); err != nil {
			t.Fatalf("removeAllForce error = %v", err)
		}
		if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("root still exists after removeAllForce: %v", err)
		}
		// Symlink target outside root must remain untouched and still 0400.
		fi, err := os.Stat(outsideFile)
		if err != nil {
			t.Fatalf("outside file vanished: %v", err)
		}
		if fi.Mode().Perm() != 0o400 {
			t.Errorf("outside file mode changed via symlink: got %o; want 0400", fi.Mode().Perm())
		}
	})
}

// -----------------------------------------------------------------------------
// parseCommandArgs tests
// -----------------------------------------------------------------------------

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{in: "", want: nil},
		{in: "   \t\n ", want: nil},
		{in: "--silent --no-desktop", want: []string{"--silent", "--no-desktop"}},
		{in: `--dir "C:\My App" --silent`, want: []string{"--dir", `C:\My App`, "--silent"}},
		{in: `--dir="C:\Program Files\My App" /S`, want: []string{`--dir=C:\Program Files\My App`, "/S"}},
		{in: `--msg "say \"hi\"" --ok`, want: []string{"--msg", `say "hi"`, "--ok"}},
		{in: `--dir "C:\My App\\" --silent`, want: []string{"--dir", `C:\My App\`, "--silent"}},
		{in: `--dir 'C:\Single App' --flag`, want: []string{"--dir", `C:\Single App`, "--flag"}},
		{in: `--empty "" --after`, want: []string{"--empty", "", "--after"}},
	}

	for _, tc := range tests {
		got := parseCommandArgs(tc.in)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseCommandArgs(%q) = %#v; want %#v", tc.in, got, tc.want)
		}
	}
}

// -----------------------------------------------------------------------------
// macOS bundle helper tests
// -----------------------------------------------------------------------------

func TestBundleHelpers(t *testing.T) {
	t.Run("findAppBundle", func(t *testing.T) {
		app, ok := findAppBundle("/Applications/MyApp.app/Contents/MacOS/MyApp")
		if !ok || app != filepath.Clean("/Applications/MyApp.app") {
			t.Errorf("findAppBundle(standard) = (%q, %v); want (/Applications/MyApp.app, true)", app, ok)
		}

		app, ok = findAppBundle("/Applications/MyApp.app/")
		if !ok || app != filepath.Clean("/Applications/MyApp.app") {
			t.Errorf("findAppBundle(.app dir) = (%q, %v); want (/Applications/MyApp.app, true)", app, ok)
		}

		if _, ok := findAppBundle("/usr/local/bin/myapp"); ok {
			t.Errorf("findAppBundle(non-bundle) = true; want false")
		}
		if _, ok := findAppBundle("/tmp/MyApp.app/Other/MacOS/MyApp"); ok {
			t.Errorf("findAppBundle(bad structure) = true; want false")
		}
	})

	t.Run("readBundleID", func(t *testing.T) {
		tmp := t.TempDir()
		app := filepath.Join(tmp, "Demo.app")
		if err := os.MkdirAll(filepath.Join(app, "Contents"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		// Include a nested dict containing a decoy CFBundleIdentifier BEFORE the real top-level key.
		decoy := `<key>Nested</key><dict><key>CFBundleIdentifier</key><string>com.decoy.nested</string></dict>`
		if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), makeTestPlist("com.example.demo", decoy), 0o644); err != nil {
			t.Fatalf("write plist: %v", err)
		}
		id, err := readBundleID(app)
		if err != nil {
			t.Fatalf("readBundleID error: %v", err)
		}
		if id != "com.example.demo" {
			t.Errorf("readBundleID = %q; want com.example.demo", id)
		}

		// Plist without CFBundleIdentifier returns empty string, nil error.
		emptyApp := filepath.Join(tmp, "Empty.app")
		_ = os.MkdirAll(filepath.Join(emptyApp, "Contents"), 0o755)
		_ = os.WriteFile(filepath.Join(emptyApp, "Contents", "Info.plist"), makeTestPlist("", ""), 0o644)
		id, err = readBundleID(emptyApp)
		if err != nil || id != "" {
			t.Errorf("readBundleID(missing key) = (%q, %v); want (\"\", nil)", id, err)
		}

		// Missing plist returns error.
		if _, err := readBundleID(filepath.Join(tmp, "NoSuch.app")); err == nil {
			t.Errorf("readBundleID(missing plist) = nil; want error")
		}
	})

	t.Run("locateExtractedApp", func(t *testing.T) {
		tmp := t.TempDir()
		// Create __MACOSX noise and an embedded helper .app inside the main bundle.
		createFakeAppBundle(t, filepath.Join(tmp, "__MACOSX", "MyApp.app"), "com.noise", "noise")
		outer := filepath.Join(tmp, "release-2.0", "MyApp.app")
		createFakeAppBundle(t, outer, "com.example.app", "v2")
		createFakeAppBundle(t, filepath.Join(outer, "Contents", "Helpers", "Helper.app"), "com.example.helper", "helper")

		got, err := locateExtractedApp(tmp)
		if err != nil {
			t.Fatalf("locateExtractedApp error: %v", err)
		}
		if got != outer {
			t.Errorf("locateExtractedApp = %q; want %q", got, outer)
		}

		emptyDir := t.TempDir()
		if _, err := locateExtractedApp(emptyDir); err == nil {
			t.Errorf("locateExtractedApp(empty) = nil; want error")
		}
	})
}

func TestMacOSReplaceAppBundle(t *testing.T) {
	t.Run("successful atomic swap", func(t *testing.T) {
		root := t.TempDir()
		cur := filepath.Join(root, "MyApp.app")
		stage := filepath.Join(t.TempDir(), "MyApp.app")
		createFakeAppBundle(t, cur, "com.example.myapp", "v1")
		createFakeAppBundle(t, stage, "com.example.myapp", "v2")

		if err := replaceAppBundle(cur, stage); err != nil {
			t.Fatalf("replaceAppBundle error: %v", err)
		}
		got, err := os.ReadFile(filepath.Join(cur, "Contents", "MacOS", "MyApp"))
		if err != nil || string(got) != "v2" {
			t.Fatalf("swapped binary = (%q, %v); want v2", string(got), err)
		}
		// Ensure no backup directories remain in root.
		entries, _ := os.ReadDir(root)
		if len(entries) != 1 || entries[0].Name() != "MyApp.app" {
			t.Errorf("leftover entries in %s: %v", root, entries)
		}
	})

	t.Run("bundle ID mismatch rejected", func(t *testing.T) {
		root := t.TempDir()
		cur := filepath.Join(root, "MyApp.app")
		stage := filepath.Join(root, "Stage.app")
		createFakeAppBundle(t, cur, "com.example.original", "v1")
		createFakeAppBundle(t, stage, "com.evil.impostor", "evil")

		err := replaceAppBundle(cur, stage)
		if err == nil {
			t.Fatalf("replaceAppBundle(mismatch) = nil; want error")
		}
		if !strings.Contains(err.Error(), "bundle identifier mismatch") {
			t.Errorf("unexpected mismatch error: %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(cur, "Contents", "MacOS", "MyApp"))
		if string(got) != "v1" {
			t.Errorf("original app modified on mismatch: got %q; want v1", string(got))
		}
	})

	t.Run("missing bundle ID in update rejected when current has one", func(t *testing.T) {
		root := t.TempDir()
		cur := filepath.Join(root, "MyApp.app")
		stageEmptyID := filepath.Join(root, "EmptyID.app")
		stageNoPlist := filepath.Join(root, "NoPlist.app")
		createFakeAppBundle(t, cur, "com.example.original", "v1")
		createFakeAppBundle(t, stageEmptyID, "", "v2-noid")
		createFakeAppBundle(t, stageNoPlist, "", "v2-noplist")
		_ = os.Remove(filepath.Join(stageNoPlist, "Contents", "Info.plist"))

		if err := replaceAppBundle(cur, stageEmptyID); err == nil {
			t.Errorf("replaceAppBundle(empty update bundle ID) = nil; want error")
		}
		if err := replaceAppBundle(cur, stageNoPlist); err == nil {
			t.Errorf("replaceAppBundle(missing update Info.plist) = nil; want error")
		}
	})

	t.Run("atomic rollback on swap failure", func(t *testing.T) {
		root := t.TempDir()
		cur := filepath.Join(root, "MyApp.app")
		stage := filepath.Join(root, "New.app")
		createFakeAppBundle(t, cur, "com.example.myapp", "v1-intact")
		createFakeAppBundle(t, stage, "com.example.myapp", "v2-fail")

		calls := 0
		failingRename := func(oldpath, newpath string) error {
			calls++
			if calls == 1 {
				return os.Rename(oldpath, newpath)
			}
			return errors.New("injected swap rename failure")
		}

		err := replaceAppBundleWithRename(cur, stage, failingRename)
		if err == nil {
			t.Fatalf("replaceAppBundleWithRename = nil; want injected error")
		}
		got, readErr := os.ReadFile(filepath.Join(cur, "Contents", "MacOS", "MyApp"))
		if readErr != nil || string(got) != "v1-intact" {
			t.Fatalf("after rollback, binary = (%q, %v); want v1-intact", string(got), readErr)
		}
	})
}

func TestMacOSDarwinApplierEndToEnd(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only end-to-end test")
	}

	root := t.TempDir()
	curApp := filepath.Join(root, "Demo.app")
	createFakeAppBundle(t, curApp, "com.example.demo", "v1")

	zipPath := filepath.Join(t.TempDir(), "Demo-2.0.0.zip")
	writeTestZip(t, zipPath, []testArchiveEntry{
		{Name: "Demo.app/Contents/Info.plist", Body: makeTestPlist("com.example.demo", ""), Mode: 0o644},
		{Name: "Demo.app/Contents/MacOS/MyApp", Body: []byte("v2-darwin"), Mode: 0o755},
	})

	quitCalled := false
	err := DefaultApplier().Apply(context.Background(), &DownloadedUpdate{
		FilePath: zipPath,
		Verified: true,
	}, ApplyOptions{
		AppPath:  curApp,
		Relaunch: false,
		QuitFunc: func() { quitCalled = true },
	})
	if err != nil {
		t.Fatalf("darwinApplier.Apply error: %v", err)
	}
	if !quitCalled {
		t.Errorf("QuitFunc was not called")
	}
	got, err := os.ReadFile(filepath.Join(curApp, "Contents", "MacOS", "MyApp"))
	if err != nil || string(got) != "v2-darwin" {
		t.Errorf("updated binary = (%q, %v); want v2-darwin", string(got), err)
	}

	// Corrupted code signature must fail verification before replacing currentApp.
	if _, err := os.Stat("/usr/bin/codesign"); err == nil {
		badSigZip := filepath.Join(t.TempDir(), "Demo-tampered.zip")
		writeTestZip(t, badSigZip, []testArchiveEntry{
			{Name: "Demo.app/Contents/Info.plist", Body: makeTestPlist("com.example.demo", ""), Mode: 0o644},
			{Name: "Demo.app/Contents/MacOS/MyApp", Body: []byte("tampered"), Mode: 0o755},
			{Name: "Demo.app/Contents/_CodeSignature/CodeResources", Body: []byte("corrupt"), Mode: 0o644},
		})
		quitTampered := false
		err := DefaultApplier().Apply(context.Background(), &DownloadedUpdate{
			FilePath: badSigZip,
			Verified: true,
		}, ApplyOptions{
			AppPath:  curApp,
			Relaunch: false,
			QuitFunc: func() { quitTampered = true },
		})
		if err == nil {
			t.Fatalf("darwinApplier.Apply(corrupt signature) = nil; want codesign error")
		}
		if quitTampered {
			t.Errorf("QuitFunc must not be called when signature verification fails")
		}
		still, _ := os.ReadFile(filepath.Join(curApp, "Contents", "MacOS", "MyApp"))
		if string(still) != "v2-darwin" {
			t.Errorf("currentApp was modified despite failed signature verification: got %q", string(still))
		}

		// Update missing CFBundleIdentifier must also be rejected.
		noIDZip := filepath.Join(t.TempDir(), "Demo-noid.zip")
		writeTestZip(t, noIDZip, []testArchiveEntry{
			{Name: "Demo.app/Contents/Info.plist", Body: makeTestPlist("", ""), Mode: 0o644},
			{Name: "Demo.app/Contents/MacOS/MyApp", Body: []byte("noid"), Mode: 0o755},
		})
		if err := DefaultApplier().Apply(context.Background(), &DownloadedUpdate{
			FilePath: noIDZip,
			Verified: true,
		}, ApplyOptions{AppPath: curApp}); err == nil {
			t.Errorf("darwinApplier.Apply(missing CFBundleIdentifier) = nil; want error")
		}
	}
}

func TestMacOSCodeSignParsingAndIdentity(t *testing.T) {
	devIDOut := `Executable=/Applications/Demo.app/Contents/MacOS/Demo
Identifier=com.example.demo
Authority=Developer ID Application: Example Inc (TEAM123456)
Authority=Developer ID Certification Authority
Authority=Apple Root CA
TeamIdentifier=TEAM123456
`
	adHoc, team, auths := parseDarwinCodeSignOutput(devIDOut)
	if adHoc || team != "TEAM123456" || len(auths) != 3 {
		t.Fatalf("parseDarwinCodeSignOutput(devID) = (%v, %q, %v)", adHoc, team, auths)
	}

	adHocOut := `Executable=/tmp/Demo.app/Contents/MacOS/Demo
Signature=adhoc
TeamIdentifier=not set
`
	adHoc, team, auths = parseDarwinCodeSignOutput(adHocOut)
	if !adHoc || team != "not set" || len(auths) != 0 {
		t.Fatalf("parseDarwinCodeSignOutput(adhoc) = (%v, %q, %v)", adHoc, team, auths)
	}

	localCertOut := `Authority=Local Dev Cert
TeamIdentifier=not set
`
	adHoc, team, auths = parseDarwinCodeSignOutput(localCertOut)
	if adHoc || team != "not set" || !reflect.DeepEqual(auths, []string{"Local Dev Cert"}) {
		t.Fatalf("parseDarwinCodeSignOutput(local cert) = (%v, %q, %v)", adHoc, team, auths)
	}

	// Signing identity checks.
	if err := verifyDarwinSigningIdentity("TEAM123456", []string{"A"}, "TEAM123456", []string{"B"}); err != nil {
		t.Errorf("matching TeamID should pass: %v", err)
	}
	if err := verifyDarwinSigningIdentity("TEAM123456", []string{"A"}, "OTHER99999", []string{"A"}); err == nil {
		t.Errorf("mismatched TeamID must fail")
	}
	if err := verifyDarwinSigningIdentity("not set", []string{"Local Dev Cert"}, "not set", []string{"Local Dev Cert"}); err != nil {
		t.Errorf("matching Authorities should pass: %v", err)
	}
	if err := verifyDarwinSigningIdentity("not set", []string{"Local Dev Cert"}, "not set", []string{"Evil Cert"}); err == nil {
		t.Errorf("mismatched Authorities must fail")
	}
	if err := verifyDarwinSigningIdentity("not set", []string{"Local Dev Cert"}, "not set", nil); err == nil {
		t.Errorf("empty update Authorities must fail")
	}
}

// -----------------------------------------------------------------------------
// Windows helper tests (run on every host OS)
// -----------------------------------------------------------------------------

func TestWindowsInstallerArgs(t *testing.T) {
	got := buildWindowsInstallerArgs("", `C:\Users\Alice\AppData\Local\MyApp`, ApplyOptions{
		Silent: true,
	})
	want := []string{"--silent", "--dir", `C:\Users\Alice\AppData\Local\MyApp`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildWindowsInstallerArgs(default silent) = %#v; want %#v", got, want)
	}

	// Untrusted feed args only keep allowlisted boolean flags; --dir, --uninstall,
	// --from, /LOADINF=..., /D=... and value tokens are dropped and cannot suppress --dir.
	got = buildWindowsInstallerArgs(
		`/S --uninstall --dir "C:\Evil Dir" --from "C:\Other" /LOADINF=C:\evil.inf /D=C:\NSIS --no-desktop --no-launch`,
		`C:\Trusted\InstallDir`,
		ApplyOptions{
			Silent:    true,
			ExtraArgs: []string{"--no-start-menu"},
		},
	)
	want = []string{"/S", "--no-desktop", "--no-launch", "--no-start-menu", "--dir", `C:\Trusted\InstallDir`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildWindowsInstallerArgs(untrusted injection) = %#v; want %#v", got, want)
	}

	// Caller-trusted ExtraArgs MAY override install directory and suppress --dir.
	got = buildWindowsInstallerArgs(`--dir "C:\Untrusted"`, `C:\Ignored`, ApplyOptions{
		Silent:    false,
		ExtraArgs: []string{`/D=C:\TrustedNSIS`},
	})
	want = []string{`/D=C:\TrustedNSIS`}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("buildWindowsInstallerArgs(trusted ExtraArgs /D=) = %#v; want %#v", got, want)
	}

	// formatWin32Args quoting & control character rejection.
	formatted, err := formatWin32Args([]string{"--silent", "--dir", `C:\Program Files\My App\`, `say "hi"`, "", `&calc|whoami`})
	if err != nil {
		t.Fatalf("formatWin32Args error: %v", err)
	}
	wantFormatted := `--silent --dir "C:\Program Files\My App\\" "say \"hi\"" "" &calc|whoami`
	if formatted != wantFormatted {
		t.Errorf("formatWin32Args = %q; want %q", formatted, wantFormatted)
	}
	if _, err := formatWin32Args([]string{"bad\narg"}); err == nil {
		t.Errorf("formatWin32Args(LF) = nil; want error")
	}
	if _, err := formatWin32Args([]string{"bad\x00arg"}); err == nil {
		t.Errorf("formatWin32Args(NUL) = nil; want error")
	}

	// Static PowerShell silent relaunch helper: zero string interpolation, all dynamic data via Env.
	evilSetup := `C:\Temp\a" & calc.exe & $(whoami)\Setup.exe`
	evilTarget := `C:\Program Files\App & $(id)\MyApp.exe`
	spec, err := buildWindowsSilentRelaunchHelper(
		12345,
		evilSetup,
		[]string{"--silent", "--dir", `C:\Program Files\My App`},
		evilTarget,
	)
	if err != nil {
		t.Fatalf("buildWindowsSilentRelaunchHelper error: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(spec.Exe), `\system32\windowspowershell\v1.0\powershell.exe`) {
		t.Errorf("helper exe = %q; want powershell.exe under SystemRoot", spec.Exe)
	}
	if len(spec.Args) != 6 || spec.Args[0] != "-NoProfile" || spec.Args[1] != "-NonInteractive" || spec.Args[4] != "-Command" {
		t.Errorf("unexpected helper args: %#v", spec.Args)
	}
	script := spec.Args[5]
	for _, forbidden := range []string{"calc.exe", "whoami", "MyApp.exe", "12345", "Program Files"} {
		if strings.Contains(script, forbidden) {
			t.Errorf("static script must not interpolate dynamic value %q: %s", forbidden, script)
		}
	}
	envMap := map[string]string{}
	for _, kv := range spec.Env {
		k, v, _ := strings.Cut(kv, "=")
		envMap[k] = v
	}
	if envMap["GGUI_UPDATER_PID"] != "12345" ||
		envMap["GGUI_UPDATER_SETUP"] != evilSetup ||
		envMap["GGUI_UPDATER_ARGS"] != `--silent --dir "C:\Program Files\My App"` ||
		envMap["GGUI_UPDATER_TARGET"] != evilTarget {
		t.Errorf("unexpected silent helper env: %#v", envMap)
	}
	if _, err := buildWindowsSilentRelaunchHelper(1, "bad\r\nsetup.exe", nil, "ok.exe"); err == nil {
		t.Errorf("buildWindowsSilentRelaunchHelper(CRLF setup) = nil; want error")
	}

	// Static PowerShell portable cleanup + relaunch helper.
	oldPaths := []string{`C:\My App [1]\app.exe.old`, `C:\My App & $(calc)\ggfx_driver.dll.old`}
	portSpec, err := buildWindowsPortableHelper(54321, evilTarget, oldPaths)
	if err != nil {
		t.Fatalf("buildWindowsPortableHelper error: %v", err)
	}
	portScript := portSpec.Args[5]
	if !strings.Contains(portScript, "Remove-Item -LiteralPath") || !strings.Contains(portScript, "ConvertFrom-Json") {
		t.Errorf("portable script missing LiteralPath/ConvertFrom-Json: %s", portScript)
	}
	for _, forbidden := range []string{"ggfx_driver.dll.old", "MyApp.exe", "54321", "calc"} {
		if strings.Contains(portScript, forbidden) {
			t.Errorf("portable script must not interpolate %q: %s", forbidden, portScript)
		}
	}
	portEnv := map[string]string{}
	for _, kv := range portSpec.Env {
		k, v, _ := strings.Cut(kv, "=")
		portEnv[k] = v
	}
	if portEnv["GGUI_UPDATER_PID"] != "54321" || portEnv["GGUI_UPDATER_TARGET"] != evilTarget {
		t.Errorf("unexpected portable helper env: %#v", portEnv)
	}
	var decodedOld []string
	if err := json.Unmarshal([]byte(portEnv["GGUI_UPDATER_CLEANUP"]), &decodedOld); err != nil || !reflect.DeepEqual(decodedOld, oldPaths) {
		t.Errorf("GGUI_UPDATER_CLEANUP = %q (err=%v); want JSON %#v", portEnv["GGUI_UPDATER_CLEANUP"], err, oldPaths)
	}
	if _, err := buildWindowsPortableHelper(1, "", []string{"bad\npath.old"}); err == nil {
		t.Errorf("buildWindowsPortableHelper(LF path) = nil; want error")
	}
	if nilSpec, err := buildWindowsPortableHelper(1, "", nil); err != nil || nilSpec != nil {
		t.Errorf("buildWindowsPortableHelper(empty) = (%v, %v); want (nil, nil)", nilSpec, err)
	}
	if gotExe := resolveWindowsPowerShellExe(""); gotExe != `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Errorf("resolveWindowsPowerShellExe(empty) = %q", gotExe)
	}
	if gotExe := resolveWindowsPowerShellExe(`D:\WinNT\`); gotExe != `D:\WinNT\System32\WindowsPowerShell\v1.0\powershell.exe` {
		t.Errorf("resolveWindowsPowerShellExe(custom) = %q", gotExe)
	}
}

func TestPortableWindowsReplacement(t *testing.T) {
	t.Run("replace exe and sibling dll from extracted zip", func(t *testing.T) {
		installDir := t.TempDir()
		targetExe := filepath.Join(installDir, "MyApp.exe")
		targetDLL := filepath.Join(installDir, "ggfx_driver.dll")
		if err := os.WriteFile(targetExe, []byte("exe-v1"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(targetDLL, []byte("dll-v1"), 0o644); err != nil {
			t.Fatal(err)
		}

		zipPath := filepath.Join(t.TempDir(), "MyApp-2.0.0-windows-amd64.zip")
		writeTestZip(t, zipPath, []testArchiveEntry{
			{Name: "MyApp-2.0.0/MyApp.exe", Body: []byte("exe-v2"), Mode: 0o755},
			{Name: "MyApp-2.0.0/ggfx_driver.dll", Body: []byte("dll-v2"), Mode: 0o644},
			{Name: "MyApp-2.0.0/README.txt", Body: []byte("notes"), Mode: 0o644},
		})

		stageDir := t.TempDir()
		if err := extractZipSafe(zipPath, stageDir); err != nil {
			t.Fatalf("extractZipSafe error: %v", err)
		}

		repls, err := locateWindowsPortableReplacements(stageDir, targetExe)
		if err != nil {
			t.Fatalf("locateWindowsPortableReplacements error: %v", err)
		}
		if len(repls) != 2 {
			t.Fatalf("replacements count = %d; want 2 (%+v)", len(repls), repls)
		}

		oldBackups, err := replacePortableFiles(repls)
		if err != nil {
			t.Fatalf("replacePortableFiles error: %v", err)
		}
		if len(oldBackups) != 2 {
			t.Errorf("oldBackups count = %d; want 2", len(oldBackups))
		}

		gotExe, _ := os.ReadFile(targetExe)
		gotDLL, _ := os.ReadFile(targetDLL)
		if string(gotExe) != "exe-v2" || string(gotDLL) != "dll-v2" {
			t.Errorf("replaced files = (%q, %q); want (exe-v2, dll-v2)", string(gotExe), string(gotDLL))
		}
		for _, old := range oldBackups {
			if _, err := os.Stat(old); err != nil {
				t.Errorf("expected backup %s to exist: %v", old, err)
			}
		}
	})

	t.Run("rollback restores all .old backups on failure", func(t *testing.T) {
		installDir := t.TempDir()
		srcDir := t.TempDir()

		targetExe := filepath.Join(installDir, "MyApp.exe")
		targetDLL := filepath.Join(installDir, "ggfx_driver.dll")
		_ = os.WriteFile(targetExe, []byte("exe-v1"), 0o755)
		_ = os.WriteFile(targetDLL, []byte("dll-v1"), 0o644)

		newExe := filepath.Join(srcDir, "MyApp.exe")
		newDLL := filepath.Join(srcDir, "ggfx_driver.dll")
		_ = os.WriteFile(newExe, []byte("exe-v2"), 0o755)
		_ = os.WriteFile(newDLL, []byte("dll-v2"), 0o644)

		repls := []portableReplacement{
			{Src: newExe, Dst: targetExe},
			{Src: newDLL, Dst: targetDLL},
		}

		calls := 0
		failSecond := func(src, dst string) error {
			calls++
			if calls == 2 {
				return errors.New("injected dll install failure")
			}
			return copyFileAtomic(src, dst, 0)
		}

		if _, err := replacePortableFilesWithInstaller(repls, failSecond); err == nil {
			t.Fatalf("replacePortableFilesWithInstaller = nil; want error")
		}

		gotExe, err := os.ReadFile(targetExe)
		if err != nil || string(gotExe) != "exe-v1" {
			t.Errorf("after rollback targetExe = (%q, %v); want exe-v1", string(gotExe), err)
		}
		gotDLL, err := os.ReadFile(targetDLL)
		if err != nil || string(gotDLL) != "dll-v1" {
			t.Errorf("after rollback targetDLL = (%q, %v); want dll-v1", string(gotDLL), err)
		}
		if _, err := os.Stat(targetExe + ".old"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s.old to be restored away", targetExe)
		}
		if _, err := os.Stat(targetDLL + ".old"); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("expected %s.old to be restored away", targetDLL)
		}
	})

	t.Run("ambiguous multiple executables rejected when none matches targetExe", func(t *testing.T) {
		stage := t.TempDir()
		_ = os.WriteFile(filepath.Join(stage, "AppA.exe"), []byte("a"), 0o755)
		_ = os.WriteFile(filepath.Join(stage, "AppB.exe"), []byte("b"), 0o755)
		target := filepath.Join(t.TempDir(), "MyApp.exe")
		if _, err := locateWindowsPortableReplacements(stage, target); err == nil {
			t.Errorf("locateWindowsPortableReplacements(ambiguous) = nil; want error")
		}
	})
}

// -----------------------------------------------------------------------------
// Linux helper tests (run on every host OS)
// -----------------------------------------------------------------------------

func TestLinuxUpdateHelpers(t *testing.T) {
	t.Run("locate binary and bundled libs in usr layout", func(t *testing.T) {
		tmp := t.TempDir()
		tgz := filepath.Join(tmp, "demo-2.0.0-linux-amd64.tar.gz")
		writeTestTarGz(t, tgz, []testArchiveEntry{
			{Name: "usr/bin/demo", Body: []byte("elf-v2"), Mode: 0o755},
			{Name: "usr/lib/demo/libggfx_driver.so", Body: []byte("so-v2"), Mode: 0o755},
			{Name: "usr/share/applications/com.example.demo.desktop", Body: []byte("[Desktop Entry]"), Mode: 0o644},
		})

		stage := t.TempDir()
		if err := extractTarGzSafe(tgz, stage); err != nil {
			t.Fatalf("extractTarGzSafe error: %v", err)
		}

		binPath, err := locateLinuxBinary(stage, "demo")
		if err != nil {
			t.Fatalf("locateLinuxBinary error: %v", err)
		}
		wantBin := filepath.Join(stage, "usr", "bin", "demo")
		if binPath != wantBin {
			t.Errorf("locateLinuxBinary = %q; want %q", binPath, wantBin)
		}

		prefix := t.TempDir()
		targetExe := filepath.Join(prefix, "bin", "demo")
		libs := locateLinuxBundledLibs(stage, binPath, targetExe)
		if len(libs) != 1 {
			t.Fatalf("locateLinuxBundledLibs count = %d; want 1 (%+v)", len(libs), libs)
		}
		wantLibDst := filepath.Join(prefix, "lib", "demo", "libggfx_driver.so")
		if libs[0].Dst != wantLibDst {
			t.Errorf("lib dst = %q; want %q", libs[0].Dst, wantLibDst)
		}
	})

	t.Run("locate binary in flat archive", func(t *testing.T) {
		tmp := t.TempDir()
		tgz := filepath.Join(tmp, "flat.tar.gz")
		writeTestTarGz(t, tgz, []testArchiveEntry{
			{Name: "myapp", Body: []byte("flat-elf"), Mode: 0o755},
			{Name: "README.md", Body: []byte("readme"), Mode: 0o644},
		})
		stage := t.TempDir()
		if err := extractTarGzSafe(tgz, stage); err != nil {
			t.Fatalf("extractTarGzSafe error: %v", err)
		}
		binPath, err := locateLinuxBinary(stage, "myapp")
		if err != nil {
			t.Fatalf("locateLinuxBinary error: %v", err)
		}
		if filepath.Base(binPath) != "myapp" {
			t.Errorf("locateLinuxBinary = %q; want basename myapp", binPath)
		}
	})

	t.Run("replaceLinuxBinary atomic replacement and rollback", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "myapp")
		newBin := filepath.Join(t.TempDir(), "myapp-new")
		if err := os.WriteFile(target, []byte("v1-bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(newBin, []byte("v2-bin"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := replaceLinuxBinary(target, newBin); err != nil {
			t.Fatalf("replaceLinuxBinary error: %v", err)
		}
		got, err := os.ReadFile(target)
		if err != nil || string(got) != "v2-bin" {
			t.Fatalf("replaced binary = (%q, %v); want v2-bin", string(got), err)
		}
		if runtime.GOOS != "windows" {
			fi, err := os.Stat(target)
			if err != nil {
				t.Fatalf("Stat target: %v", err)
			}
			if fi.Mode().Perm() != 0o755 {
				t.Errorf("replaced mode = %o; want 0755", fi.Mode().Perm())
			}
		}
		// Ensure no temporary .myapp.new-* files were left behind.
		if leftover, _ := filepath.Glob(filepath.Join(dir, ".myapp.new*")); len(leftover) != 0 {
			t.Errorf("expected no leftover .myapp.new* files, got %v", leftover)
		}

		// Failed single atomic rename leaves target untouched and cleans up temp staged file.
		newBin3 := filepath.Join(t.TempDir(), "myapp-v3")
		_ = os.WriteFile(newBin3, []byte("v3-bad"), 0o755)
		failRename := func(oldpath, newpath string) error {
			return errors.New("injected rename failure")
		}
		if err := replaceLinuxBinaryWithRename(target, newBin3, failRename); err == nil {
			t.Fatalf("replaceLinuxBinaryWithRename = nil; want error")
		}
		got, err = os.ReadFile(target)
		if err != nil || string(got) != "v2-bin" {
			t.Errorf("after failed rename binary = (%q, %v); want v2-bin", string(got), err)
		}
		if leftover, _ := filepath.Glob(filepath.Join(dir, ".myapp.new*")); len(leftover) != 0 {
			t.Errorf("expected staged .new file to be cleaned up on failure, got %v", leftover)
		}
	})

	t.Run("detectLinuxPackageKind rejects deb/rpm and unknown formats", func(t *testing.T) {
		tmp := t.TempDir()
		write := func(name string, data []byte) string {
			p := filepath.Join(tmp, name)
			if err := os.WriteFile(p, data, 0o644); err != nil {
				t.Fatal(err)
			}
			return p
		}

		if _, err := detectLinuxPackageKind(write("app.deb", []byte("!<arch>\n"))); err == nil {
			t.Errorf("detectLinuxPackageKind(.deb) = nil; want error")
		}
		if _, err := detectLinuxPackageKind(write("app.rpm", []byte{0xed, 0xab, 0xee, 0xdb})); err == nil {
			t.Errorf("detectLinuxPackageKind(.rpm) = nil; want error")
		}
		if _, err := detectLinuxPackageKind(write("unknown.bin", []byte("<html>404</html>"))); err == nil {
			t.Errorf("detectLinuxPackageKind(html) = nil; want error")
		}
		if k, err := detectLinuxPackageKind(write("pkg.bin", []byte{0x1f, 0x8b, 0x08, 0x00})); err != nil || k != linuxPackageTarGz {
			t.Errorf("detectLinuxPackageKind(gzip magic) = (%v, %v); want tar.gz", k, err)
		}
		if k, err := detectLinuxPackageKind(write("zip.bin", []byte{'P', 'K', 0x03, 0x04})); err != nil || k != linuxPackageZip {
			t.Errorf("detectLinuxPackageKind(zip magic) = (%v, %v); want zip", k, err)
		}
		if k, err := detectLinuxPackageKind(write("app.AppImage", []byte{0x7f, 'E', 'L', 'F', 0x02})); err != nil || k != linuxPackageRawExecutable {
			t.Errorf("detectLinuxPackageKind(ELF magic) = (%v, %v); want raw executable", k, err)
		}
		if k, err := detectLinuxPackageKind(write("wrapper", []byte("#!/bin/sh\necho hi\n"))); err != nil || k != linuxPackageRawExecutable {
			t.Errorf("detectLinuxPackageKind(shebang) = (%v, %v); want raw executable", k, err)
		}
	})

	t.Run("locateLinuxBundledLibs refuses arbitrary system lib paths", func(t *testing.T) {
		stage := t.TempDir()
		newBin := filepath.Join(stage, "usr", "bin", "demo")
		_ = os.MkdirAll(filepath.Dir(newBin), 0o755)
		_ = os.WriteFile(newBin, []byte("elf"), 0o755)
		// Allowed: matches exe name.
		_ = os.MkdirAll(filepath.Join(stage, "usr", "lib", "demo"), 0o755)
		_ = os.WriteFile(filepath.Join(stage, "usr", "lib", "demo", "libggfx_driver.so"), []byte("ok1"), 0o644)
		// Allowed: subdir already exists under targetPrefix/lib.
		_ = os.MkdirAll(filepath.Join(stage, "usr", "lib", "existingpkg"), 0o755)
		_ = os.WriteFile(filepath.Join(stage, "usr", "lib", "existingpkg", "libhelper.so.1"), []byte("ok2"), 0o644)
		// Blocked: top-level usr/lib/*.so.
		_ = os.WriteFile(filepath.Join(stage, "usr", "lib", "libc.so.6"), []byte("evil-sys"), 0o644)
		// Blocked: unknown subdir not matching exe and not existing in targetPrefix/lib.
		_ = os.MkdirAll(filepath.Join(stage, "usr", "lib", "otherpkg"), 0o755)
		_ = os.WriteFile(filepath.Join(stage, "usr", "lib", "otherpkg", "libevil.so"), []byte("evil-other"), 0o644)

		prefix := t.TempDir()
		_ = os.MkdirAll(filepath.Join(prefix, "lib", "existingpkg"), 0o755)
		targetExe := filepath.Join(prefix, "bin", "demo")

		libs := locateLinuxBundledLibs(stage, newBin, targetExe)
		if len(libs) != 2 {
			t.Fatalf("locateLinuxBundledLibs count = %d; want 2 (%+v)", len(libs), libs)
		}
		gotDsts := map[string]bool{}
		for _, l := range libs {
			gotDsts[l.Dst] = true
		}
		if !gotDsts[filepath.Join(prefix, "lib", "demo", "libggfx_driver.so")] ||
			!gotDsts[filepath.Join(prefix, "lib", "existingpkg", "libhelper.so.1")] {
			t.Errorf("unexpected mapped libs: %+v", libs)
		}
	})

	t.Run("replaceLinuxArchiveFiles rolls back .so libs on binary or lib failure", func(t *testing.T) {
		dstDir := t.TempDir()
		srcDir := t.TempDir()

		targetExe := filepath.Join(dstDir, "demo")
		existingLib := filepath.Join(dstDir, "libexisting.so")
		newLibDst := filepath.Join(dstDir, "libbrandnew.so")
		_ = os.WriteFile(targetExe, []byte("exe-v1"), 0o755)
		_ = os.WriteFile(existingLib, []byte("so-v1"), 0o755)

		srcBin := filepath.Join(srcDir, "demo")
		srcExisting := filepath.Join(srcDir, "libexisting.so")
		srcNew := filepath.Join(srcDir, "libbrandnew.so")
		_ = os.WriteFile(srcBin, []byte("exe-v2"), 0o755)
		_ = os.WriteFile(srcExisting, []byte("so-v2"), 0o755)
		_ = os.WriteFile(srcNew, []byte("so-new"), 0o755)

		libs := []portableReplacement{
			{Src: srcExisting, Dst: existingLib},
			{Src: srcNew, Dst: newLibDst},
		}

		// Failure in binary replacement must roll back both existing and newly created .so files.
		err := replaceLinuxArchiveFiles(libs, targetExe, srcBin, copyFileAtomic, func(string, string) error {
			return errors.New("binary swap failed")
		})
		if err == nil {
			t.Fatalf("replaceLinuxArchiveFiles = nil; want binary swap error")
		}
		if got, _ := os.ReadFile(existingLib); string(got) != "so-v1" {
			t.Errorf("existingLib after rollback = %q; want so-v1", string(got))
		}
		if _, err := os.Lstat(newLibDst); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("newly created lib must be deleted on rollback: stat err = %v", err)
		}
		if leftover, _ := filepath.Glob(filepath.Join(dstDir, "*.old-*")); len(leftover) != 0 {
			t.Errorf("unexpected leftover backup files after rollback: %v", leftover)
		}

		// Success cleans up all .old-* backups.
		if err := replaceLinuxArchiveFiles(libs, targetExe, srcBin, copyFileAtomic, replaceLinuxBinary); err != nil {
			t.Fatalf("replaceLinuxArchiveFiles(success) error = %v", err)
		}
		if got, _ := os.ReadFile(existingLib); string(got) != "so-v2" {
			t.Errorf("existingLib = %q; want so-v2", string(got))
		}
		if got, _ := os.ReadFile(newLibDst); string(got) != "so-new" {
			t.Errorf("newLibDst = %q; want so-new", string(got))
		}
		if got, _ := os.ReadFile(targetExe); string(got) != "exe-v2" {
			t.Errorf("targetExe = %q; want exe-v2", string(got))
		}
		if leftover, _ := filepath.Glob(filepath.Join(dstDir, ".*.old-*")); len(leftover) != 0 {
			t.Errorf("unexpected leftover .old-* files after success: %v", leftover)
		}
	})
}
