//go:build darwin

package machosign

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// codesign runs Apple's codesign and returns what it said.
func codesign(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("codesign", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("codesign %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// sameTree reports the files that differ between two directories.
func sameTree(t *testing.T, a, b string) {
	t.Helper()
	seen := map[string]bool{}
	filepath.WalkDir(a, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(a, p)
		seen[rel] = true
		if e.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		x, _ := os.ReadFile(p)
		y, err := os.ReadFile(filepath.Join(b, rel))
		if err != nil || !bytes.Equal(x, y) {
			t.Errorf("%s differs from what codesign wrote (%d and %d bytes)", rel, len(x), len(y))
		}
		return nil
	})
	filepath.WalkDir(b, func(p string, e fs.DirEntry, err error) error {
		if rel, _ := filepath.Rel(b, p); err == nil && !e.IsDir() && !seen[rel] {
			t.Errorf("codesign wrote %s, machosign did not", rel)
		}
		return err
	})
}

// jitEntitlements are entitlements an ad hoc signed program may have
// and still run; entitlementsPlist has some that need a provisioning
// profile, and macOS kills a program claiming them, however signed.
const jitEntitlements = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>com.apple.security.cs.allow-jit</key>
	<true/>
	<key>com.apple.security.get-task-allow</key>
	<true/>
</dict>
</plist>
`

// exact asks TestLikeCodesign for codesign's bytes, which hold only
// against the codesign machosign was written against, macOS 27's.
// macOS 26's wrote files about 18 KB larger for the same programs, so
// with another codesign the test asks for the same signature instead:
// what codesign -d says of it, and its checks.
var exact = os.Getenv("GGUI_CODESIGN_EXACT") == "1"

// sameSignature reports what codesign -d says differently of a and b,
// where it would not say it differently of the same signature laid out
// in a file of another size.
func sameSignature(t *testing.T, a, b string) {
	t.Helper()
	x, y := signatureFacts(codesign(t, "-dvvv", a)), signatureFacts(codesign(t, "-dvvv", b))
	if x != y {
		t.Errorf("codesign -d says of machosign's signature\n%s\nand of codesign's\n%s", x, y)
	}
}

// signatureFacts keeps the lines of codesign -dvvv that describe the
// signature rather than the file: the identifier, format, flags, hash
// and the sealed Info.plist and resources, with the CodeDirectory's size
// and hash counts left out.
func signatureFacts(out string) string {
	var facts []string
	for line := range strings.SplitSeq(out, "\n") {
		switch {
		case strings.HasPrefix(line, "CodeDirectory "):
			for f := range strings.FieldsSeq(line) {
				if strings.HasPrefix(f, "flags=") || strings.HasPrefix(f, "v=") {
					facts = append(facts, f)
				}
			}
		case strings.HasPrefix(line, "Identifier="), strings.HasPrefix(line, "Format="),
			strings.HasPrefix(line, "Hash type="), strings.HasPrefix(line, "Signature="),
			strings.HasPrefix(line, "Info.plist"), strings.HasPrefix(line, "Sealed Resources"),
			strings.HasPrefix(line, "Internal requirements"), strings.HasPrefix(line, "Runtime"):
			facts = append(facts, line)
		}
	}
	return strings.Join(facts, "\n")
}

// Signing a bundle must give the signature codesign --force --sign -
// gives, byte for byte under GGUI_CODESIGN_EXACT=1, and pass codesign's
// strictest checks.
func TestLikeCodesign(t *testing.T) {
	if _, err := exec.LookPath("codesign"); err != nil {
		t.Skip("no codesign")
	}
	arm, amd := program(t, "arm64"), program(t, "amd64")
	fat, err := Fat(arm, amd)
	if err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	if _, err := exec.LookPath("lipo"); err == nil {
		os.WriteFile(filepath.Join(tmp, "arm64"), arm, 0o755)
		os.WriteFile(filepath.Join(tmp, "amd64"), amd, 0o755)
		if out, err := exec.Command("lipo", "-create", "-output", filepath.Join(tmp, "fat"), filepath.Join(tmp, "arm64"), filepath.Join(tmp, "amd64")).CombinedOutput(); err != nil {
			t.Fatalf("lipo: %s", out)
		}
		if b, _ := os.ReadFile(filepath.Join(tmp, "fat")); !bytes.Equal(b, fat) {
			t.Error("Fat differs from lipo -create")
		}
	}
	dylib := clangDylib(t)

	for _, tc := range []struct {
		name         string
		exe          []byte
		entitlements string
		nested       bool
		runs         bool
	}{
		{name: "arm64", exe: arm, runs: true},
		{name: "amd64", exe: amd},
		{name: "universal", exe: fat, runs: true},
		{name: "entitlements", exe: arm, entitlements: entitlementsPlist},
		{name: "jit", exe: fat, entitlements: jitEntitlements, runs: true},
		{name: "nested", exe: arm, nested: true, runs: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var apps [2]string
			for i := range apps {
				apps[i] = makeBundle(t, t.TempDir(), tc.exe)
				if tc.nested {
					c := filepath.Join(apps[i], "Contents")
					os.WriteFile(filepath.Join(c, "MacOS", "helper"), amd, 0o755)
					os.MkdirAll(filepath.Join(c, "Frameworks"), 0o755)
					os.WriteFile(filepath.Join(c, "Frameworks", "libhello.dylib"), fat, 0o755)
					if dylib != nil {
						os.WriteFile(filepath.Join(c, "Frameworks", "libx.dylib"), dylib, 0o755)
					}
				}
			}
			var ent []byte
			args := []string{"--force", "--sign", "-"}
			if tc.entitlements != "" {
				ent = []byte(tc.entitlements)
				f := filepath.Join(t.TempDir(), "entitlements.plist")
				os.WriteFile(f, ent, 0o644)
				args = append(args, "--entitlements", f)
			}
			if tc.nested {
				args = append(args, "--deep")
			}
			if err := SignBundle(apps[0], ent); err != nil {
				t.Fatal(err)
			}
			codesign(t, append(args, apps[1])...)
			if exact {
				sameTree(t, apps[0], apps[1])
			} else {
				sameSignature(t, apps[0], apps[1])
			}

			out := codesign(t, "--verify", "--strict", "--deep", "-vv", apps[0])
			if !strings.Contains(out, "valid on disk") || !strings.Contains(out, "satisfies its Designated Requirement") {
				t.Errorf("codesign --verify:\n%s", out)
			}
			out = codesign(t, "-dvvv", apps[0])
			for _, want := range []string{"Identifier=com.example.hello", "flags=0x2(adhoc)", "Signature=adhoc", "Info.plist entries=4", "Sealed Resources version=2 rules=13"} {
				if !strings.Contains(out, want) {
					t.Errorf("codesign -dvvv says no %q:\n%s", want, out)
				}
			}
			if tc.entitlements != "" {
				for _, form := range []string{"--xml", "--der"} {
					// The first line names the program.
					_, a, _ := strings.Cut(codesign(t, "-d", "--entitlements", "-", form, apps[0]), "\n")
					_, b, _ := strings.Cut(codesign(t, "-d", "--entitlements", "-", form, apps[1]), "\n")
					if a != b || a == "" {
						t.Errorf("entitlements %s differ:\n%q\n%q", form, a, b)
					}
				}
			}
			if tc.runs && runtime.GOARCH == "arm64" {
				// The kernel kills a program whose signature does not hold.
				out, err := exec.Command(filepath.Join(apps[0], "Contents", "MacOS", "Hello")).CombinedOutput()
				if err != nil || string(out) != "hello\n" {
					t.Errorf("running the signed program: %v, %q", err, out)
				}
			}
		})
	}
}

// clangDylib returns a universal dynamic library built by clang, or nil
// without clang.
func clangDylib(t *testing.T) []byte {
	if _, err := exec.LookPath("clang"); err != nil {
		return nil
	}
	dir := t.TempDir()
	src, out := filepath.Join(dir, "x.c"), filepath.Join(dir, "libx.dylib")
	os.WriteFile(src, []byte("int f(void) { return 1; }\n"), 0o644)
	if msg, err := exec.Command("clang", "-dynamiclib", "-arch", "arm64", "-arch", "x86_64", "-o", out, src).CombinedOutput(); err != nil {
		t.Logf("clang: %s", msg)
		return nil
	}
	b, _ := os.ReadFile(out)
	return b
}
