package doctor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ironpark/ggui/cmd/ggui/internal/project"
)

// The checks below are for the machine ggui runs on, which is where apps
// built here are run during development, whatever GOOS a build targets.
// Each list follows what ggwd and ggfx open at run time:
//
//   - macOS: frameworks by absolute path through purego, every one at
//     start-up (ggwd's glfw package opens OpenGL.framework even when ggfx
//     draws with Metal), panicking when one is missing.
//   - Linux and the BSDs: libX11, then libGLESv2 or libGL from ggfx, then
//     GLX or EGL from ggwd to make the context, with the other X
//     extensions optional. There is no Wayland backend; under Wayland the
//     app runs through XWayland, so DISPLAY must be set. ggfx's shaders
//     are GLSL 3.30 or GLSL ES 3.00, so the driver must offer OpenGL 3.3
//     or OpenGL ES 3.0.
//   - Windows: Direct3D 11 through d3d11, dxgi and a d3dcompiler, with
//     OpenGL as the fallback when the compiler is missing.

func hostIsDarwin() bool { return runtime.GOOS == "darwin" }

// xcodeTools reports whether the Xcode command line tools, or Xcode, are
// installed: what a C compiler comes from on macOS.
func xcodeTools() bool {
	return exec.Command("xcode-select", "-p").Run() == nil
}

func checkPlatform(ctx context.Context, r *Report, env map[string]string, proj *project.Project) {
	wantZig := projectUsesZig(proj)
	switch runtime.GOOS {
	case "darwin":
		checkDarwin(ctx, r)
	case "linux", "freebsd", "netbsd":
		checkUnix(ctx, r, wantZig)
	case "windows":
		checkWindows(r, wantZig)
	default:
		r.warn("platform", runtime.GOOS+": ggui apps cannot run here; builds for other targets still work", "")
	}
	if env["GOOS"] == "js" {
		r.ok("browser", "a js/wasm build runs in a browser with WebGL 2: current Chrome, Edge, Firefox and Safari")
	}
}

// darwinFrameworks are the frameworks loaded at start-up.
var darwinFrameworks = []string{
	"AppKit", "Carbon", "CoreFoundation", "CoreGraphics", "CoreVideo",
	"Foundation", "IOKit", "Metal", "OpenGL", "QuartzCore",
}

// darwinMinimum is the oldest macOS a Go 1.27 program starts on, which is
// older than the oldest ggfx's Metal code needs.
const darwinMinimum = "13.0"

func checkDarwin(ctx context.Context, r *Report) {
	if v := strings.TrimSpace(run(ctx, "sw_vers", "-productVersion")); v != "" {
		if compareVersions(v, darwinMinimum) < 0 {
			r.fail("macOS", "macOS "+v+"; programs built by Go 1.27 need macOS "+darwinMinimum+" or later", "update macOS")
		} else {
			r.ok("macOS", "macOS "+v)
		}
	}
	// Since Big Sur a system framework's binary lives in the dyld shared
	// cache rather than in its directory, but the directory remains, and
	// is what a damaged or stripped-down system would lack.
	var missing []string
	for _, f := range darwinFrameworks {
		if _, err := os.Stat("/System/Library/Frameworks/" + f + ".framework"); err != nil {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		r.fail("frameworks", "missing: "+strings.Join(missing, ", "), "these are part of macOS; reinstall or update it")
	} else {
		r.ok("frameworks", "Metal, OpenGL, AppKit and the others ggwd and ggfx load are present")
	}
}

func checkUnix(ctx context.Context, r *Report, wantZig bool) {
	display, wayland := os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY")
	switch {
	case display != "":
		r.ok("display", "X11 display "+display)
	case wayland != "":
		r.fail("display", "Wayland session without DISPLAY: ggwd draws through X11, which XWayland provides",
			"enable XWayland in the compositor, or set DISPLAY to its display")
	default:
		r.fail("display", "no display: DISPLAY is not set", "run the app in a graphical session; building works without one")
	}

	libs := newLibIndex(ctx)
	checkRequiredLibraries(r, libs, wantZig)
	checkGLVersion(ctx, r, display)
	checkXExtensions(r, libs)
	checkFeaturePrograms(r)
}

// checkRequiredLibraries checks for the libraries no window opens
// without: libX11, libGLESv2 or libGL, and GLX or EGL for the context,
// plus the Vulkan loader for the optional Zig Vulkan renderer.
func checkRequiredLibraries(r *Report, libs *libIndex, wantZig bool) {
	if p := libs.find("libX11.so.6", "libX11.so"); p != "" {
		r.ok("libX11", p)
	} else {
		r.fail("libX11", "not found; windows cannot open without it", installHint("libx11-6", "libX11"))
	}
	if vk := libs.find("libvulkan.so.1", "libvulkan.so"); vk != "" {
		r.ok("Vulkan", "Vulkan loader through "+vk+" (used by the Zig renderer)")
	} else if wantZig && runtime.GOOS == "linux" {
		r.warn("Vulkan", "libvulkan.so.1 not found; the Zig renderer falls back to OpenGL without Vulkan",
			installHint("libvulkan1", "vulkan-loader"))
	}
	gles := libs.find("libGLESv2.so", "libGLESv2.so.2", "libGLESv2.so.1", "libGLESv2.so.0")
	gl := libs.find("libGL.so", "libGL.so.2", "libGL.so.1", "libGL.so.0")
	switch {
	case gles != "":
		r.ok("OpenGL", "OpenGL ES through "+gles)
	case gl != "":
		r.ok("OpenGL", "OpenGL through "+gl)
	default:
		r.fail("OpenGL", "neither libGLESv2 nor libGL found; ggfx has no other renderer on this system",
			installHint("libgl1 libgles2", "mesa-libGL mesa-libGLES"))
	}
	glx := libs.find("libGLX.so.0", "libGL.so.1", "libGL.so")
	egl := libs.find("libEGL.so.1", "libEGL.so")
	switch {
	case gles != "" && egl == "":
		// ggwd makes an OpenGL ES context with GLX only when glxinfo
		// lists GLX_EXT_create_context_es2_profile, and with EGL
		// otherwise.
		r.warn("context", "no libEGL: an OpenGL ES context needs GLX's ES profile extension, which glxinfo must report",
			installHint("libegl1", "mesa-libEGL"))
	case glx == "" && egl == "":
		r.fail("context", "neither GLX nor EGL found to make an OpenGL context", installHint("libglx0 libegl1", "mesa-libGL mesa-libEGL"))
	default:
		r.ok("context", strings.Trim(strings.Join([]string{glx, egl}, ", "), ", "))
	}
}

// checkXExtensions checks for the optional X extensions input,
// monitors, cursors and transparency need.
func checkXExtensions(r *Report, libs *libIndex) {
	var missing []string
	for _, l := range [][]string{
		{"libXi.so.6", "libXi.so"},
		{"libXrandr.so.2", "libXrandr.so"},
		{"libXcursor.so.1", "libXcursor.so"},
		{"libXinerama.so.1", "libXinerama.so"},
		{"libXrender.so.1", "libXrender.so"},
		{"libXext.so.6", "libXext.so"},
	} {
		if libs.find(l...) == "" {
			missing = append(missing, strings.TrimSuffix(l[1], ".so"))
		}
	}
	if len(missing) > 0 {
		r.warn("X extensions", "missing "+strings.Join(missing, ", ")+": input, monitors, cursors or transparency will be limited",
			installHint("libxi6 libxrandr2 libxcursor1 libxinerama1 libxrender1 libxext6", "libXi libXrandr libXcursor libXinerama libXrender libXext"))
	} else {
		r.ok("X extensions", "Xi, Xrandr, Xcursor, Xinerama, Xrender and Xext found")
	}
}

// checkFeaturePrograms checks for the programs ggui runs for a feature;
// without one only that feature is missing.
func checkFeaturePrograms(r *Report) {
	tools := []struct{ names, what, fix string }{
		{"fc-match", "ggui finds the system's UI fonts with it, and finds none without it", installHint("fontconfig", "fontconfig")},
		{"zenity kdialog", "native file and message dialogs; without either they report ErrUnsupported", installHint("zenity", "zenity")},
		{"xdg-open", "OpenURL and Reveal", installHint("xdg-utils", "xdg-utils")},
	}
	for _, t := range tools {
		found := ""
		for n := range strings.FieldsSeq(t.names) {
			if p, err := exec.LookPath(n); err == nil {
				found = p
				break
			}
		}
		name := strings.Fields(t.names)[0]
		if found != "" {
			r.ok(name, found)
		} else {
			r.warn(name, "not found: "+t.what, t.fix)
		}
	}
}

// glVersion matches glxinfo's version lines.
var glVersion = regexp.MustCompile(`(?m)^OpenGL (core profile |ES profile )?version string: (OpenGL ES )?([0-9]+\.[0-9]+)`)

// checkGLVersion asks glxinfo, when it is installed, which OpenGL the
// driver offers: ggfx's shaders need 3.3, or ES 3.0.
func checkGLVersion(ctx context.Context, r *Report, display string) {
	p, err := exec.LookPath("glxinfo")
	if err != nil || display == "" {
		r.warn("OpenGL version", "not checked: needs glxinfo and a display; the driver must offer OpenGL 3.3 or OpenGL ES 3.0",
			installHint("mesa-utils", "glx-utils"))
		return
	}
	out := run(ctx, p, "-B")
	best, es := "", false
	for _, m := range glVersion.FindAllStringSubmatch(out, -1) {
		isES := strings.HasPrefix(m[1], "ES") || m[2] != ""
		need := "3.3"
		if isES {
			need = "3.0"
		}
		if compareVersions(m[3], need) >= 0 {
			r.ok("OpenGL version", m[0])
			return
		}
		best, es = m[3], isES
	}
	if best == "" {
		r.warn("OpenGL version", "glxinfo did not report one", "")
		return
	}
	kind := "OpenGL"
	if es {
		kind = "OpenGL ES"
	}
	r.fail("OpenGL version", fmt.Sprintf("the driver offers %s %s; ggfx needs OpenGL 3.3 or OpenGL ES 3.0", kind, best),
		"update the graphics driver or Mesa")
}

// installHint names the packages to install on Debian and Fedora
// families, which is most Linux desktops.
func installHint(debian, fedora string) string {
	if runtime.GOOS != "linux" {
		return "install it with the system's package manager"
	}
	return fmt.Sprintf("sudo apt install %s  (Fedora: sudo dnf install %s)", debian, fedora)
}

// libIndex finds shared libraries as the dynamic loader would: through the
// loader's cache when ldconfig can print it, and otherwise in the usual
// directories and those LD_LIBRARY_PATH and ld.so.conf name.
type libIndex struct {
	cache map[string]string
	dirs  []string
}

func newLibIndex(ctx context.Context) *libIndex {
	ix := &libIndex{cache: map[string]string{}}
	for _, ldconfig := range []string{"ldconfig", "/sbin/ldconfig", "/usr/sbin/ldconfig"} {
		out := run(ctx, ldconfig, "-p")
		if out == "" {
			continue
		}
		// Lines read "\tlibX11.so.6 (libc6,x86-64) => /lib/x86_64-linux-gnu/libX11.so.6".
		sc := bufio.NewScanner(strings.NewReader(out))
		for sc.Scan() {
			name, path, ok := strings.Cut(sc.Text(), "=>")
			if !ok {
				continue
			}
			name = strings.TrimSpace(name)
			if i := strings.IndexByte(name, ' '); i >= 0 {
				name = name[:i]
			}
			if _, seen := ix.cache[name]; !seen {
				ix.cache[name] = strings.TrimSpace(path)
			}
		}
		break
	}
	for _, d := range filepath.SplitList(os.Getenv("LD_LIBRARY_PATH")) {
		if d != "" {
			ix.dirs = append(ix.dirs, d)
		}
	}
	ix.dirs = append(ix.dirs, ldConfDirs("/etc/ld.so.conf", 0)...)
	ix.dirs = append(ix.dirs, "/lib", "/usr/lib", "/lib64", "/usr/lib64", "/usr/local/lib", "/usr/X11R7/lib", "/usr/pkg/lib")
	if m, _ := filepath.Glob("/usr/lib/*-linux-gnu*"); len(m) > 0 {
		ix.dirs = append(ix.dirs, m...)
	}
	if m, _ := filepath.Glob("/lib/*-linux-gnu*"); len(m) > 0 {
		ix.dirs = append(ix.dirs, m...)
	}
	return ix
}

// ldConfDirs reads the directories an ld.so.conf names, following its
// include lines.
func ldConfDirs(path string, depth int) []string {
	if depth > 4 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var dirs []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case line == "":
		case strings.HasPrefix(line, "include "):
			pattern := strings.TrimSpace(strings.TrimPrefix(line, "include "))
			if !filepath.IsAbs(pattern) {
				pattern = filepath.Join(filepath.Dir(path), pattern)
			}
			matches, _ := filepath.Glob(pattern)
			for _, m := range matches {
				dirs = append(dirs, ldConfDirs(m, depth+1)...)
			}
		default:
			dirs = append(dirs, line)
		}
	}
	return dirs
}

// find returns the path of the first of names that exists, or "".
func (ix *libIndex) find(names ...string) string {
	for _, n := range names {
		if p, ok := ix.cache[n]; ok {
			return p
		}
	}
	for _, n := range names {
		for _, d := range ix.dirs {
			p := filepath.Join(d, n)
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return ""
}

// checkWindows looks in System32 for the libraries ggfx draws with. Go
// 1.21 and later run on Windows 10 or later only, so the version needs no
// check: an older Windows could not have started this command.
func checkWindows(r *Report, wantZig bool) {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	sys := filepath.Join(root, "System32")
	has := func(name string) bool {
		_, err := os.Stat(filepath.Join(sys, name)) //nolint:gosec // G703: the path is the CLI user's own argument or build output
		return err == nil
	}
	compiler := ""
	for _, n := range []string{"d3dcompiler_47.dll", "d3dcompiler_46.dll", "d3dcompiler_43.dll"} {
		if has(n) {
			compiler = n
			break
		}
	}
	d3d := has("d3d11.dll") && has("dxgi.dll")
	d3d12 := has("d3d12.dll") && has("dxgi.dll")
	gl := has("opengl32.dll")
	switch {
	case d3d && d3d12 && compiler != "":
		r.ok("graphics", "Direct3D 11 & 12 (d3d11.dll, d3d12.dll, dxgi.dll, "+compiler+")")
	case d3d && compiler != "":
		r.ok("graphics", "Direct3D 11 (d3d11.dll, dxgi.dll, "+compiler+")")
		if wantZig {
			r.warn("Direct3D 12", "d3d12.dll not found; the Zig renderer falls back to Direct3D 11", "install Windows updates and a DirectX 12 GPU driver")
		}
	case d3d12 && wantZig:
		r.ok("graphics", "Direct3D 12 (d3d12.dll, dxgi.dll)")
	case gl:
		r.warn("graphics", "Direct3D 11 is not usable (no d3d11, dxgi or d3dcompiler); ggfx falls back to OpenGL 3.3, which needs the GPU vendor's driver",
			"install the GPU driver, or the DirectX runtime for d3dcompiler_47.dll")
	default:
		r.fail("graphics", "neither Direct3D 11 nor OpenGL found in "+sys, "install the GPU driver and Windows updates")
	}
	if has("uiautomationcore.dll") {
		r.ok("accessibility", "UI Automation available")
	} else {
		r.warn("accessibility", "uiautomationcore.dll not found: screen readers will not see the app", "")
	}
}
