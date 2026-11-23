// Package ziglib resolves and unpacks ggfx's prebuilt Zig graphics driver
// shared libraries from a project's module graph, and normalizes the build
// tags and linker flags that enable the Zig native renderer.
package ziglib

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

const (
	// RendererGo selects ggfx's default pure-Go graphics driver.
	RendererGo = "go"
	// RendererZig selects ggfx's Zig native graphics driver
	// (Metal on macOS, Direct3D 12 on Windows, Vulkan on Linux).
	RendererZig = "zig"

	// Tag makes the Zig driver the default and embeds the prebuilt shared
	// library into the Go binary.
	Tag = "ggfxzig"
	// NoEmbedTag omits the embedded shared library so the binary loads a
	// bundled library file instead (required for signed macOS apps under
	// Hardened Runtime and recommended for Windows/Linux release bundles).
	NoEmbedTag = "ggfxnoembed"

	// GGFXModule is the module that ships the prebuilt Zig driver libraries.
	GGFXModule = "github.com/ironpark/ggfx"
	// LibraryPathVar is the linker -X target in ggfx that pins the library
	// path relative to the executable directory when NoEmbedTag is set.
	LibraryPathVar = "github.com/ironpark/ggfx/internal/graphicsdriver/zigdriver.LibraryPath"
)

// Supported reports whether ggfx ships a Zig native driver for goos/goarch.
func Supported(goos, goarch string) bool {
	switch goos {
	case "darwin", "windows", "linux":
		return goarch == "amd64" || goarch == "arm64"
	default:
		return false
	}
}

// FileName returns the shared library filename ggfx's zigdriver loads on goos,
// or "" when goos has no Zig driver.
func FileName(goos string) string {
	switch goos {
	case "darwin":
		return "libggfx_driver.dylib"
	case "windows":
		return "ggfx_driver.dll"
	case "linux":
		return "libggfx_driver.so"
	default:
		return ""
	}
}

// FileMode returns the file mode for an unpacked shared library on goos.
func FileMode(goos string) os.FileMode {
	if goos == "windows" {
		return 0o644
	}
	return 0o755
}

// PrebuiltRelPath returns the relative path inside github.com/ironpark/ggfx
// to the gzip-compressed prebuilt shared library for goos/goarch.
func PrebuiltRelPath(goos, goarch string) string {
	name := FileName(goos)
	if name == "" || !Supported(goos, goarch) {
		return ""
	}
	return filepath.Join("internal", "graphicsdriver", "zigdriver", "prebuilt", goos+"_"+goarch, name+".gz")
}

// ResolveRenderer resolves whether the Zig native renderer is enabled from
// ggui.toml's build.renderer, a --renderer flag override, and the build tags.
//
// When --renderer is not set and build.renderer is "go" (or empty), a manual
// "ggfxzig" entry in tags still enables the Zig renderer so viewer builds and
// release packaging stay consistent. When --renderer go is passed explicitly,
// any ggfxzig/ggfxnoembed tags are stripped so the flag wins cleanly.
func ResolveRenderer(cfgRenderer, flagRenderer string, flagSet bool, tags []string) (useZig bool, cleanedTags []string, err error) {
	r := strings.ToLower(strings.TrimSpace(cfgRenderer))
	if r == "" {
		r = RendererGo
	}
	if flagSet {
		r = strings.ToLower(strings.TrimSpace(flagRenderer))
	}
	switch r {
	case RendererZig:
		return true, slices.Clone(tags), nil
	case RendererGo:
		if flagSet {
			return false, WithoutTags(tags), nil
		}
		if slices.Contains(tags, Tag) {
			return true, slices.Clone(tags), nil
		}
		return false, slices.Clone(tags), nil
	default:
		return false, nil, fmt.Errorf(`--renderer %q must be "go" or "zig"`, flagRenderer)
	}
}

// WithTags returns tags with the Zig driver build tags added.
// When embed is false, both ggfxzig and ggfxnoembed are included.
func WithTags(tags []string, embed bool) []string {
	out := slices.Clone(tags)
	if !slices.Contains(out, Tag) {
		out = append(out, Tag)
	}
	if !embed && !slices.Contains(out, NoEmbedTag) {
		out = append(out, NoEmbedTag)
	}
	return out
}

// WithoutTags returns a copy of tags with ggfxzig and ggfxnoembed removed.
func WithoutTags(tags []string) []string {
	if !slices.Contains(tags, Tag) && !slices.Contains(tags, NoEmbedTag) {
		return slices.Clone(tags)
	}
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if t == Tag || t == NoEmbedTag {
			continue
		}
		out = append(out, t)
	}
	return out
}

// LibraryPathLDFlag returns the -X linker flag pinning zigdriver.LibraryPath
// to relPath (evaluated relative to the executable directory at run time).
func LibraryPathLDFlag(relPath string) string {
	return "-X " + LibraryPathVar + "=" + relPath
}

// Prebuilt locates the gzip-compressed Zig driver library for goos/goarch in
// the project's resolved github.com/ironpark/ggfx module.
func Prebuilt(ctx context.Context, dir, goos, goarch string) (string, error) {
	rel := PrebuiltRelPath(goos, goarch)
	if rel == "" {
		return "", fmt.Errorf("the Zig renderer does not support %s/%s", goos, goarch)
	}
	m, err := resolveGGFX(ctx, dir)
	if err != nil {
		return "", err
	}
	if m.Dir == "" && m.Version != "" {
		dl, err := gotool.DownloadModule(ctx, dir, GGFXModule)
		if err != nil {
			return "", fmt.Errorf("downloading %s for the Zig driver library: %w", GGFXModule, err)
		}
		m.Dir = dl.Dir
	}
	if m.Dir == "" {
		return "", fmt.Errorf("could not locate %s on disk for the Zig driver library", GGFXModule)
	}
	gz := filepath.Join(m.Dir, rel)
	if _, err := os.Stat(gz); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", missingPrebuiltError(m, goos, goarch, rel)
		}
		return "", err
	}
	return gz, nil
}

// Extract unpacks the project's prebuilt Zig driver library for goos/goarch
// to dst, creating parent directories and setting platform-appropriate permissions.
func Extract(ctx context.Context, dir, goos, goarch, dst string) error {
	gz, err := Prebuilt(ctx, dir, goos, goarch)
	if err != nil {
		return err
	}
	return gunzipFile(gz, dst, FileMode(goos))
}

func resolveGGFX(ctx context.Context, dir string) (*gotool.Module, error) {
	mods, err := gotool.Modules(ctx, dir, GGFXModule)
	if err != nil {
		return nil, err
	}
	if len(mods) != 1 || mods[0].Error != nil {
		return nil, fmt.Errorf("the Zig renderer requires %s v0.1.0 or later, and this module does not require %s", GGFXModule, GGFXModule)
	}
	return &mods[0], nil
}

func missingPrebuiltError(m *gotool.Module, goos, goarch, rel string) error {
	if m.Local() {
		return fmt.Errorf("the project's %s (%s) lacks the prebuilt Zig driver library for %s/%s (%s); run ./scripts/build-zig-prebuilt.sh --all in %s",
			GGFXModule, m.Dir, goos, goarch, filepath.ToSlash(rel), m.Dir)
	}
	ver := m.Version
	if m.Replace != nil && m.Replace.Version != "" {
		ver = m.Replace.Path + "@" + m.Replace.Version
	}
	return fmt.Errorf("the project's %s (%s) has no prebuilt Zig driver library for %s/%s; update to %s@v0.1.0 or later (go get %s@v0.1.0)",
		GGFXModule, ver, goos, goarch, GGFXModule, GGFXModule)
}

func gunzipFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return fmt.Errorf("reading %s: %w", src, err)
	}
	defer gz.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".ggfx-driver-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, gz); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("unpacking %s: %w", src, err)
	}
	if err := gz.Close(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("unpacking %s: %w", src, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := shared.Rename(tmpName, dst); err != nil {
		return err
	}
	ok = true
	return nil
}
