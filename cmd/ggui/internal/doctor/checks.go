package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ironpark/ggui/cmd/ggui/internal/config"
	"github.com/ironpark/ggui/cmd/ggui/internal/gotool"
	"github.com/ironpark/ggui/cmd/ggui/internal/i18n"
	"github.com/ironpark/ggui/cmd/ggui/internal/project"
	"github.com/ironpark/ggui/cmd/ggui/internal/windows"
)

// Module paths the project is checked for.
const (
	gguiPath = "github.com/ironpark/ggui"
	ggfxPath = "github.com/ironpark/ggfx"
	ggwdPath = "github.com/ironpark/ggwd"
)

// knownGo is the Go version ggui asked for when this command was written,
// the yardstick for a directory that resolves no ggui of its own.
const knownGo = "1.27.0"

func checkGo(ctx context.Context, r *Report, o Options) (map[string]string, bool) {
	env, err := gotool.Env(ctx, o.Dir, nil, "GOVERSION", "GOOS", "GOARCH", "GOROOT",
		"CGO_ENABLED", "CC", "GOMOD", "GOWORK")
	if err != nil {
		r.fail("go", err.Error(), "install Go from https://go.dev/dl/ and put its bin directory on PATH")
		return nil, false
	}
	return env, true
}

type projectInfo struct {
	ggui *gotool.Module
}

// checkGoVersion compares the toolchain with what ggui's go.mod asks for:
// ggui uses generic methods, which older compilers reject with errors that
// do not say a newer Go is the fix.
func checkGoVersion(r *Report, env map[string]string, p projectInfo) {
	have := env["GOVERSION"]
	want, from := knownGo, "ggui"
	if p.ggui != nil && p.ggui.GoVersion != "" {
		want = p.ggui.GoVersion
		if v := p.ggui.Version; v != "" && p.ggui.Replace == nil {
			from = "ggui " + v
		}
	}
	if strings.HasPrefix(have, "devel") {
		r.warn("go version", have+", a development toolchain", "")
		return
	}
	if compareVersions(strings.TrimPrefix(have, "go"), want) < 0 {
		r.fail("go version", fmt.Sprintf("%s, but %s needs go%s", have, from, want),
			"install a newer Go from https://go.dev/dl/, or let the go command fetch one: go env -w GOTOOLCHAIN=auto")
		return
	}
	r.ok("go version", fmt.Sprintf("%s (%s needs go%s)", have, from, want))
}

// compareVersions compares dotted version numbers, a missing part being 0
// and anything after a part's leading digits, such as rc1, ignored.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	lead := func(s string) int {
		end := strings.IndexFunc(s, func(r rune) bool { return r < '0' || r > '9' })
		if end >= 0 {
			s = s[:end]
		}
		n, _ := strconv.Atoi(s)
		return n
	}
	for i := range max(len(pa), len(pb)) {
		var x, y int
		if i < len(pa) {
			x = lead(pa[i])
		}
		if i < len(pb) {
			y = lead(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// supported lists the platforms ggfx and ggwd have code for: ok for those
// ggui's CI builds and tests, warn for those that only compile.
var supported = map[string]map[string]Status{
	"darwin":  {"amd64": OK, "arm64": OK},
	"windows": {"amd64": OK, "arm64": OK, "386": Warn},
	"linux":   {"amd64": OK, "arm64": OK, "riscv64": OK, "386": Warn, "arm": Warn, "loong64": Warn, "ppc64le": Warn, "s390x": Warn},
	"freebsd": {"amd64": Warn, "arm64": Warn},
	"netbsd":  {"amd64": Warn, "arm64": Warn},
	"js":      {"wasm": OK},
}

func checkTarget(r *Report, env map[string]string) {
	goos, goarch := env["GOOS"], env["GOARCH"]
	target := goos + "/" + goarch
	archs, ok := supported[goos]
	switch {
	case !ok:
		r.fail("target", target+": ggfx and ggwd have no code for "+goos,
			"build for darwin, windows, linux, freebsd, netbsd or js/wasm")
	case archs[goarch] == "":
		r.fail("target", target+": ggui's dependencies do not build for "+goarch+" on "+goos, "")
	case archs[goarch] == Warn:
		r.warn("target", target+": builds, but ggui's CI does not test it", "")
	default:
		r.ok("target", target)
	}

	// purego works without cgo, so ggui needs no C compiler, but with cgo
	// on, the go command links runtime/cgo and calls one. On macOS the
	// command line tools' clang is a stub until they are installed, which
	// the go command cannot tell from a compiler.
	if env["CGO_ENABLED"] == "1" && goos == "darwin" && hostIsDarwin() && !xcodeTools() {
		r.warn("cgo", "on, and the Xcode command line tools are not installed: builds will fail at the C compiler",
			"ggui needs no cgo: set CGO_ENABLED=0 (go env -w CGO_ENABLED=0), or install the tools: xcode-select --install")
	} else if env["CGO_ENABLED"] == "1" {
		r.ok("cgo", "on (ggui needs none; CGO_ENABLED=0 also builds)")
	} else {
		r.ok("cgo", "off (ggui needs none)")
	}
}

func checkProject(ctx context.Context, r *Report, o Options, env map[string]string) projectInfo {
	var p projectInfo
	gomod := env["GOMOD"]
	if gomod == "" || gomod == os.DevNull {
		r.warn("project", "not in a Go module, so no project was checked", "create one with: ggui init")
		return p
	}
	mods, err := gotool.Modules(ctx, o.Dir, gguiPath, ggfxPath, ggwdPath)
	if err != nil {
		r.fail("project", err.Error(), "")
		return p
	}
	byPath := map[string]*gotool.Module{}
	for i := range mods {
		byPath[mods[i].Path] = &mods[i]
	}
	g := byPath[gguiPath]
	if g == nil || g.Error != nil {
		r.warn("project", fmt.Sprintf("module %s does not require %s", filepath.Dir(gomod), gguiPath),
			"add it with: go get "+gguiPath+"@latest")
		return p
	}
	p.ggui = g
	r.ok("ggui", version(g))

	// ggui is developed with exact versions of ggfx and ggwd, so a build
	// that resolves others, raised by another requirement or a go get, is
	// a combination nobody has tested.
	var mf *gotool.ModFile
	if g.GoMod != "" {
		mf, _ = gotool.ReadModFile(ctx, g.GoMod)
	}
	for _, path := range []string{ggfxPath, ggwdPath} {
		name := path[strings.LastIndex(path, "/")+1:]
		m := byPath[path]
		want := ""
		if mf != nil {
			want = mf.Required(path)
		}
		switch {
		case m == nil || m.Error != nil:
			r.fail(name, "not in the build, though ggui requires it", "run: go mod tidy")
		case m.Main || m.Replace != nil && m.Replace.Version == "":
			r.ok(name, version(m)+fmt.Sprintf(" (ggui asks for %s)", orUnknown(want)))
		case want == "":
			r.ok(name, version(m))
		case m.Version == want:
			r.ok(name, m.Version+" (as ggui asks)")
		default:
			r.warn(name, fmt.Sprintf("%s, but ggui %s asks for %s", m.Version, version(g), want),
				fmt.Sprintf("another requirement raised it; to build what ggui is tested with: go get %s@%s", path, want))
		}
	}
	return p
}

func orUnknown(s string) string {
	if s == "" {
		return "no version"
	}
	return s
}

// version describes where a module's source comes from.
func version(m *gotool.Module) string {
	switch {
	case m.Main:
		return "workspace module " + m.Dir
	case m.Replace != nil && m.Replace.Version == "":
		return "replaced by " + m.Replace.Path
	case m.Replace != nil:
		return m.Replace.Version + " (replaced)"
	default:
		return m.Version
	}
}

// checkConfig checks ggui.toml and returns the project it configures, or
// nil when it is invalid.
func checkConfig(r *Report, o Options) *project.Project {
	proj, err := project.Open(o.Dir, "", o.Config)
	if err != nil {
		fix := "fix the file; docs/cli.md lists every key"
		var ce config.Errors
		var e *config.Error
		if errors.As(err, &ce) || errors.As(err, &e) {
			r.fail("ggui.toml", "invalid:\n"+err.Error(), fix)
		} else {
			r.fail("ggui.toml", err.Error(), fix)
		}
		return nil
	}
	if proj.File == "" {
		r.ok("ggui.toml", "none; the defaults are in effect (ggui init writes one)")
		return proj
	}
	r.ok("ggui.toml", proj.File)
	if icon := proj.Config.App.Icon; icon != "" {
		checkIcon(r, proj.Path(icon))
	}
	checkWindowsBuild(r, proj)
	return proj
}

// checkTranslations sums up ggui i18n check, for a project with catalogs:
// a translation missing or wrong is a warning here, since the app still
// builds and runs, showing the source text or a garbled one.
func checkTranslations(r *Report, root string) {
	src, err := i18n.Find(root, i18n.DefaultGlobs)
	if err != nil || len(src.Catalogs) == 0 {
		return // a Go file that does not parse is the build's to say
	}
	var bad []string
	warns := 0
	for _, c := range src.Catalogs {
		errs := 0
		for _, p := range i18n.Check(c, src, i18n.Options{}) {
			if p.Error {
				errs++
			} else {
				warns++
			}
		}
		if errs > 0 {
			name := c.Lang
			if name == "" {
				name = c.Name
			}
			what := "errors"
			if errs == 1 {
				what = "error"
			}
			bad = append(bad, fmt.Sprintf("%s: %d %s", name, errs, what))
		}
	}
	langs := strings.Join(i18n.Langs(src.Catalogs), ", ")
	switch {
	case len(bad) > 0:
		r.warn("translations", strings.Join(bad, "; "), "ggui i18n check lists them; ggui i18n extract adds what is missing")
	case warns > 0:
		r.ok("translations", fmt.Sprintf("%s: every message translated; ggui i18n check has %d warnings", langs, warns))
	default:
		r.ok("translations", langs+": every message translated")
	}
}

// checkWindowsBuild checks what ggui build --target windows takes from
// ggui.toml beyond what the file's own validation does, since macOS takes
// it as it is: a version whose parts fit Windows' 16 bits, a name a file
// can have, and a manifest Windows can parse.
func checkWindowsBuild(r *Report, proj *project.Project) {
	cfg := proj.Config
	var problems []string
	if _, err := windows.ParseVersion(cfg.App.Version); err != nil {
		problems = append(problems, "app.version: "+err.Error())
	}
	if cfg.App.Name != "" {
		if err := windows.CheckFileName(cfg.App.Name); err != nil {
			problems = append(problems, err.Error())
		}
	}
	manifest := "ggui's manifest"
	if m := cfg.Windows.Manifest; m != "" {
		if _, err := windows.ReadManifest(proj.Path(m)); err != nil {
			problems = append(problems, "windows.manifest: "+err.Error())
		}
		manifest = "the manifest " + m
	}
	if len(problems) > 0 {
		r.fail("Windows build", strings.Join(problems, "\n"), "ggui build --target windows needs these fixed; other targets build as they are")
		return
	}
	name := cfg.App.Name
	if name == "" {
		name = "the main package's name"
	}
	link := "a GUI program"
	if cfg.Windows.Console {
		link = "a console program (windows.console)"
	}
	icon := "no icon"
	if cfg.App.Icon != "" {
		icon = "app.icon"
	}
	r.ok("Windows build", fmt.Sprintf("%s.exe, %s, with %s, version %s and %s", name, link, icon, cfg.App.Version, manifest))
}

func checkIcon(r *Report, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.fail("icon", err.Error(), "add the PNG, or remove app.icon from ggui.toml")
		return
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		r.fail("icon", path+" is not a PNG: "+err.Error(), "export the icon as PNG")
		return
	}
	switch {
	case cfg.Width != cfg.Height:
		r.fail("icon", fmt.Sprintf("%s is %d×%d; it must be square", path, cfg.Width, cfg.Height), "")
	case cfg.Width < 64:
		r.fail("icon", fmt.Sprintf("%s is %d×%d; it must be at least 64×64", path, cfg.Width, cfg.Height), "")
	case cfg.Width < 256:
		r.warn("icon", fmt.Sprintf("%s is %d×%d; Windows' largest icon is 256×256, and a Retina Dock and Finder want 1024×1024", path, cfg.Width, cfg.Height), "")
	case cfg.Width < 1024:
		r.warn("icon", fmt.Sprintf("%s is %d×%d; a Retina Dock and Finder want 1024×1024", path, cfg.Width, cfg.Height), "")
	default:
		r.ok("icon", fmt.Sprintf("%s (%d×%d)", path, cfg.Width, cfg.Height))
	}
}

func checkTools(ctx context.Context, r *Report, env map[string]string) {
	// wasm-opt only shrinks web builds; without it they ship as linked.
	if p, err := exec.LookPath("wasm-opt"); err == nil {
		v := firstLine(run(ctx, p, "--version"))
		r.ok("wasm-opt", strings.TrimSpace(v))
	} else {
		r.warn("wasm-opt", "not on PATH: web builds ship unoptimized, about 2% larger",
			"install binaryen: brew install binaryen, apt install binaryen, or https://github.com/WebAssembly/binaryen/releases")
	}

	if !hostIsDarwin() {
		return
	}
	if _, err := exec.LookPath("codesign"); err != nil {
		r.fail("codesign", "not found; ggui build --target macos signs every bundle", "codesign is part of macOS: check that /usr/bin is on PATH")
	} else {
		r.ok("codesign", "found")
	}
	if xcodeTools() {
		r.ok("lipo", "found (for --universal)")
	} else {
		r.warn("lipo", "needs the Xcode command line tools, which are not installed: --universal will fail",
			"xcode-select --install")
	}
}

// run runs a program briefly and returns its output, "" when it fails.
func run(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
