package windows

import (
	"archive/zip"
	"bytes"
	"debug/pe"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ironpark/ggui/cmd/ggui/internal/installer"
)

func TestObjectExecutionLevelAndOriginalName(t *testing.T) {
	r := testResources(t, "amd64")
	r.OriginalName = "Hello & Goodbye-Setup.exe"
	r.Description = "Hello & Goodbye Setup"
	r.ExecutionLevel = ExecutionLevelRequireAdmin
	obj, _, err := Object(r)
	if err != nil {
		t.Fatal(err)
	}
	f, err := pe.NewFile(bytes.NewReader(obj))
	if err != nil {
		t.Fatal(err)
	}
	sect, err := f.Sections[0].Data()
	if err != nil {
		t.Fatal(err)
	}
	res := walk(t, sect, 0)
	v := readVersion(t, res[resKey{rtVersion, 1, langENUS}])
	if v.strings["OriginalFilename"] != "Hello & Goodbye-Setup.exe" {
		t.Errorf("OriginalFilename = %q", v.strings["OriginalFilename"])
	}
	if v.strings["FileDescription"] != "Hello & Goodbye Setup" {
		t.Errorf("FileDescription = %q", v.strings["FileDescription"])
	}
	m := string(res[resKey{rtManifest, 1, langENUS}])
	if !strings.Contains(m, `level="requireAdministrator"`) {
		t.Errorf("manifest does not request requireAdministrator:\n%s", m)
	}
}

func TestSetupBuilderValidateAndPayload(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "Hello.exe")
	dll := filepath.Join(dir, "ggfx_driver.dll")
	lic := filepath.Join(dir, "LICENSE.txt")
	icon := pngFile(t, 256)
	os.WriteFile(exe, []byte("exe-bytes"), 0o755)
	os.WriteFile(dll, []byte("dll-bytes"), 0o644)
	os.WriteFile(lic, []byte("MIT License\n"), 0o644)

	spec := installer.Spec{
		Name:        "Hello",
		Executable:  "hello",
		ID:          "com.example.hello",
		Version:     "1.2.3",
		Arch:        "amd64",
		ProjectDir:  dir,
		Artifact:    exe,
		ExtraFiles:  []string{dll},
		OutDir:      dir,
		Description: "Friendly app",
		Publisher:   "Example Ltd",
		Copyright:   "© 2026 Example Ltd",
		Homepage:    "https://example.com",
		Icon:        icon,
		License:     lic,
		Windows: installer.WindowsOptions{
			Scope:             installer.ScopeMachine,
			InstallLocation:   `%ProgramFiles%\Hello`,
			StartMenuShortcut: true,
			DesktopShortcut:   true,
			LaunchAfter:       false,
		},
	}

	var b installer.Builder = SetupBuilder{}
	if b.Target() != "windows" {
		t.Fatalf("Target() = %q, want windows", b.Target())
	}
	if got, want := b.OutputPath(spec), filepath.Join(dir, "Hello-Setup.exe"); got != want {
		t.Fatalf("OutputPath() = %q, want %q", got, want)
	}
	if err := b.Validate(spec); err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	// Bad scope fails validation.
	bad := spec
	bad.Windows.Scope = "admin"
	if err := b.Validate(bad); err == nil {
		t.Error("expected validation error for bad scope")
	}

	// Check payload.zip layout and metadata JSON.
	raw, err := buildPayloadZip(spec, installer.ScopeMachine)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		buf.ReadFrom(rc)
		rc.Close()
		entries[f.Name] = buf.Bytes()
	}
	for _, want := range []string{"meta.json", "icon.png", "files/Hello.exe", "files/ggfx_driver.dll"} {
		if _, ok := entries[want]; !ok {
			t.Errorf("payload.zip missing %s", want)
		}
	}
	var meta payloadMetadata
	if err := json.Unmarshal(entries["meta.json"], &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Name != "Hello" || meta.ID != "com.example.hello" || meta.Executable != "Hello.exe" ||
		meta.Scope != "machine" || meta.License != "MIT License\n" || !meta.DesktopShortcut || meta.LaunchAfter {
		t.Errorf("unexpected payload metadata: %+v", meta)
	}

	// Inline LicenseText takes precedence over License file path.
	inlineSpec := spec
	inlineSpec.License = ""
	inlineSpec.LicenseText = "Inline Go License"
	if err := b.Validate(inlineSpec); err != nil {
		t.Fatalf("Validate with LicenseText failed: %v", err)
	}
	rawInline, err := buildPayloadZip(inlineSpec, installer.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	zr2, err := zip.NewReader(bytes.NewReader(rawInline), int64(len(rawInline)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr2.File {
		if f.Name == "meta.json" {
			rc, _ := f.Open()
			var m payloadMetadata
			json.NewDecoder(rc).Decode(&m)
			rc.Close()
			if m.License != "Inline Go License" {
				t.Errorf("inline license = %q, want Inline Go License", m.License)
			}
		}
	}

	// Nested ExtraEntries are packed deterministically under files/<rel>.
	helperExe := filepath.Join(dir, "docker.exe")
	pluginExe := filepath.Join(dir, "docker-buildx.exe")
	os.WriteFile(helperExe, []byte("docker-bytes"), 0o755)
	os.WriteFile(pluginExe, []byte("buildx-bytes"), 0o755)
	nestedSpec := spec
	nestedSpec.ExtraEntries = map[string]string{
		"bin/docker.exe":                helperExe,
		"cli-plugins/docker-buildx.exe": pluginExe,
	}
	if err := b.Validate(nestedSpec); err != nil {
		t.Fatalf("Validate(nestedSpec): %v", err)
	}
	rawNestedA, err := buildPayloadZip(nestedSpec, installer.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	rawNestedB, err := buildPayloadZip(nestedSpec, installer.ScopeUser)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rawNestedA, rawNestedB) {
		t.Error("buildPayloadZip with ExtraEntries is not deterministic")
	}
	zr3, err := zip.NewReader(bytes.NewReader(rawNestedA), int64(len(rawNestedA)))
	if err != nil {
		t.Fatal(err)
	}
	gotNested := map[string]bool{}
	for _, f := range zr3.File {
		gotNested[f.Name] = true
	}
	for _, want := range []string{"files/Hello.exe", "files/ggfx_driver.dll", "files/bin/docker.exe", "files/cli-plugins/docker-buildx.exe"} {
		if !gotNested[want] {
			t.Errorf("nested payload missing %s", want)
		}
	}
}
