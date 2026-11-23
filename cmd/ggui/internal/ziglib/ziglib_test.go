package ziglib

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSupportedAndNames(t *testing.T) {
	for _, tc := range []struct {
		goos, goarch string
		want         bool
		file         string
	}{
		{"darwin", "arm64", true, "libggfx_driver.dylib"},
		{"darwin", "amd64", true, "libggfx_driver.dylib"},
		{"windows", "amd64", true, "ggfx_driver.dll"},
		{"windows", "arm64", true, "ggfx_driver.dll"},
		{"windows", "386", false, "ggfx_driver.dll"},
		{"linux", "amd64", true, "libggfx_driver.so"},
		{"linux", "arm64", true, "libggfx_driver.so"},
		{"linux", "riscv64", false, "libggfx_driver.so"},
		{"js", "wasm", false, ""},
		{"freebsd", "amd64", false, ""},
	} {
		if got := Supported(tc.goos, tc.goarch); got != tc.want {
			t.Errorf("Supported(%q, %q) = %v, want %v", tc.goos, tc.goarch, got, tc.want)
		}
		if got := FileName(tc.goos); got != tc.file {
			t.Errorf("FileName(%q) = %q, want %q", tc.goos, got, tc.file)
		}
		rel := PrebuiltRelPath(tc.goos, tc.goarch)
		if tc.want && !strings.HasSuffix(filepath.ToSlash(rel), "/"+tc.goos+"_"+tc.goarch+"/"+tc.file+".gz") {
			t.Errorf("PrebuiltRelPath(%q, %q) = %q", tc.goos, tc.goarch, rel)
		}
		if !tc.want && rel != "" {
			t.Errorf("PrebuiltRelPath(%q, %q) = %q, want empty", tc.goos, tc.goarch, rel)
		}
	}
}

func TestResolveRenderer(t *testing.T) {
	for _, tc := range []struct {
		name        string
		cfg, flag   string
		flagSet     bool
		tags        []string
		wantZig     bool
		wantTags    []string
		wantErrPart string
	}{
		{"defaults", "", "", false, nil, false, nil, ""},
		{"config go", "go", "", false, []string{"prod"}, false, []string{"prod"}, ""},
		{"config zig", "zig", "", false, []string{"prod"}, true, []string{"prod"}, ""},
		{"flag zig overrides go", "go", "zig", true, []string{"prod"}, true, []string{"prod"}, ""},
		{"flag go overrides zig", "zig", "go", true, []string{"prod", "ggfxzig", "ggfxnoembed"}, false, []string{"prod"}, ""},
		{"manual ggfxzig tag auto-detected", "go", "", false, []string{"ggui_inspector", "ggfxzig"}, true, []string{"ggui_inspector", "ggfxzig"}, ""},
		{"flag go overrides manual ggfxzig", "go", "go", true, []string{"ggui_inspector", "ggfxzig"}, false, []string{"ggui_inspector"}, ""},
		{"invalid flag", "go", "metal", true, nil, false, nil, `--renderer "metal"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotZig, gotTags, err := ResolveRenderer(tc.cfg, tc.flag, tc.flagSet, tc.tags)
			if tc.wantErrPart != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErrPart) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if gotZig != tc.wantZig {
				t.Errorf("useZig = %v, want %v", gotZig, tc.wantZig)
			}
			if !slices.Equal(gotTags, tc.wantTags) {
				t.Errorf("tags = %v, want %v", gotTags, tc.wantTags)
			}
		})
	}
}

func TestWithAndWithoutTags(t *testing.T) {
	if got := WithTags([]string{"prod"}, true); !slices.Equal(got, []string{"prod", "ggfxzig"}) {
		t.Errorf("WithTags(embed=true) = %v", got)
	}
	if got := WithTags([]string{"prod", "ggfxzig"}, false); !slices.Equal(got, []string{"prod", "ggfxzig", "ggfxnoembed"}) {
		t.Errorf("WithTags(embed=false) = %v", got)
	}
	if got := WithoutTags([]string{"prod", "ggfxzig", "ggfxnoembed", "other"}); !slices.Equal(got, []string{"prod", "other"}) {
		t.Errorf("WithoutTags = %v", got)
	}
}

func TestExtractFromReplacedGGFX(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the go command")
	}
	t.Setenv("GOWORK", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOPROXY", "off")

	ggfxDir := t.TempDir()
	os.WriteFile(filepath.Join(ggfxDir, "go.mod"), []byte("module github.com/ironpark/ggfx\n\ngo 1.24\n"), 0o644)
	payload := []byte("fake-zig-driver-binary-payload")
	gzPath := filepath.Join(ggfxDir, PrebuiltRelPath("linux", "amd64"))
	if err := os.MkdirAll(filepath.Dir(gzPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(gzPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	if _, err := gw.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	appDir := t.TempDir()
	goMod := "module example.com/app\n\ngo 1.24\n\nrequire github.com/ironpark/ggfx v0.1.0\n\nreplace github.com/ironpark/ggfx => " + filepath.ToSlash(ggfxDir) + "\n"
	os.WriteFile(filepath.Join(appDir, "go.mod"), []byte(goMod), 0o644)

	dst := filepath.Join(t.TempDir(), "nested", "libggfx_driver.so")
	if err := Extract(t.Context(), appDir, "linux", "amd64", dst); err != nil {
		t.Fatalf("Extract failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("extracted %q, want %q", got, payload)
	}

	// Missing platform in local checkout gives an actionable build-zig-prebuilt hint.
	err = Extract(t.Context(), appDir, "darwin", "arm64", filepath.Join(t.TempDir(), "libggfx_driver.dylib"))
	if err == nil || !strings.Contains(err.Error(), "build-zig-prebuilt.sh") {
		t.Fatalf("missing prebuilt err = %v, want build-zig-prebuilt.sh hint", err)
	}
}
