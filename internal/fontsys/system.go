package fontsys

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ironpark/ggui/internal/text"
)

// SystemUI loads the platform's interface fonts, with every weight it has
// of them, or nil for one it does not find: SF Pro and SF Mono on macOS,
// Segoe UI and Cascadia Mono or Consolas on Windows, and what fontconfig
// matches for sans-serif and monospace elsewhere. Browsers have no
// filesystem fonts. Each call loads them anew; ggui keeps the first.
func SystemUI() (sans, mono *Font) {
	sansFiles, monoFiles := systemUIFiles()
	return loadFamily(sansFiles), loadFamily(monoFiles)
}

// systemUIFiles lists the faces of the platform's interface fonts, the
// regular one first.
func systemUIFiles() (sans, mono []fontFile) {
	switch runtime.GOOS {
	case "darwin":
		// Both are variable fonts, which draw every weight.
		return []fontFile{{path: "/System/Library/Fonts/SFNS.ttf"}}, []fontFile{{path: "/System/Library/Fonts/SFNSMono.ttf"}}
	case "windows":
		sans = windowsFonts("SegUIVar.ttf") // Windows 11, a variable font
		if len(sans) == 0 {
			sans = windowsFonts("segoeui.ttf", "segoeuil.ttf", "seguisb.ttf", "segoeuib.ttf")
		}
		mono = windowsFonts("CascadiaMono.ttf")
		if len(mono) == 0 {
			mono = windowsFonts("consola.ttf", "consolab.ttf")
		}
		return sans, mono
	case "js", "wasip1", "ios", "android":
		return nil, nil
	}
	return fontconfig("sans-serif"), fontconfig("monospace")
}

// fontconfig asks fc-match for family at regular, medium, semibold and bold
// weights, in fontconfig's weight numbers, and returns each distinct face.
func fontconfig(family string) []fontFile {
	var out []fontFile
	for _, weight := range []string{"regular", "medium", "semibold", "bold"} {
		b, err := exec.Command("fc-match", "-f", "%{file}\n%{index}", family+":"+weight).Output()
		if err != nil {
			return out
		}
		path, index, _ := strings.Cut(string(b), "\n")
		f := fontFile{path: path}
		f.index, _ = strconv.Atoi(strings.TrimSpace(index))
		if path != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	return out
}

// SystemItalic loads the italic faces of the platform's interface fonts;
// see ggui.SystemItalicFonts. Either is nil where the platform has no
// italic of that font. Each call loads them anew; ggui keeps the first.
func SystemItalic() (sans, mono *Font) {
	sansFiles, monoFiles := systemItalicFiles()
	italic := func(f *Font) *Font {
		if f == nil || f.Meta().Metadata().Style != text.StyleItalic {
			return nil
		}
		return f
	}
	return italic(loadFamily(sansFiles)), italic(loadFamily(monoFiles))
}

// systemItalicFiles lists the faces of the italics of the platform's
// interface fonts, the regular weight first.
func systemItalicFiles() (sans, mono []fontFile) {
	switch runtime.GOOS {
	case "darwin":
		return []fontFile{{path: "/System/Library/Fonts/SFNSItalic.ttf"}}, []fontFile{{path: "/System/Library/Fonts/SFNSMonoItalic.ttf"}}
	case "windows":
		sans = windowsFonts("segoeuii.ttf", "seguisbi.ttf", "segoeuiz.ttf")
		mono = windowsFonts("CascadiaMonoItalic.ttf")
		if len(mono) == 0 {
			mono = windowsFonts("consolai.ttf", "consolaz.ttf")
		}
		return sans, mono
	case "js", "wasip1", "ios", "android":
		return nil, nil
	}
	return fontconfig("sans-serif:italic"), fontconfig("monospace:italic")
}

// windowsFonts lists those of the named files that are in Windows' font
// folder, in order.
func windowsFonts(names ...string) []fontFile {
	dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	var out []fontFile
	for _, name := range names {
		if p := filepath.Join(dir, name); fileExists(p) {
			out = append(out, fontFile{path: p})
		}
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadFamily loads files as one Font: the first as the font, and the
// faces of other weights as its weights. A collection's upright faces of
// other weights join too, so a family shipped as one .ttc draws bold.
// Files that fail to load are skipped.
func loadFamily(files []fontFile) *Font {
	var base *Font
	for _, file := range files {
		faces, err := loadFaces(file)
		if err != nil {
			continue
		}
		if base == nil {
			base, faces = faces[0], faces[1:]
		}
		for _, f := range faces {
			if !slices.ContainsFunc(append([]*Font{base}, base.weights...), func(have *Font) bool { return have.weight == f.weight }) {
				base.weights = append(base.weights, f)
			}
		}
	}
	return base
}

// loadFaces loads a font file: its one face, or for a collection the face
// at file.index followed by the collection's other faces of its style,
// upright for an upright face and italic for an italic one. Each is read
// as it draws; see fileFont.
func loadFaces(file fontFile) ([]*Font, error) {
	r, size, err := openFontFile(file.path)
	if err != nil {
		return nil, err
	}
	n, err := faceCount(r)
	if err != nil {
		return nil, err
	}
	var faces []*Font
	for i := range n {
		if f, err := fileFont(r, size, i); err == nil {
			faces = append(faces, f)
		}
	}
	if len(faces) == 0 {
		return nil, os.ErrNotExist
	}
	first := faces[min(file.index, len(faces)-1)]
	out := []*Font{first}
	style := first.Meta().Metadata().Style
	for _, f := range faces {
		if f != first && f.Meta().Metadata().Style == style {
			out = append(out, f)
		}
	}
	return out, nil
}

// SystemFonts are the fonts every Font falls back to first, found once;
// see ggui.SystemFonts. A glyph none of them has is then looked for in
// every font the system has (see SystemFallback).
var SystemFonts = sync.OnceValue(func() []*Font {
	var fonts []*Font
	seen := map[string]bool{}
	load := func(files []fontFile) {
		if len(files) == 0 || seen[files[0].path] {
			return
		}
		seen[files[0].path] = true
		// Every weight of the family, so text asking for one draws it
		// in these scripts too.
		if f := loadFamily(files); f != nil {
			f.noFallback = true
			fonts = append(fonts, f)
		}
	}
	for _, path := range filepath.SplitList(os.Getenv("GGUI_FONTS")) {
		if path != "" {
			load([]fontFile{{path: path}})
		}
	}
	for _, family := range systemFallbackFamilies() {
		if f := systemFamily(family); f != nil {
			f.noFallback = true
			fonts = append(fonts, f)
		}
	}
	return fonts
})

// systemFallbackFamilies lists the families SystemFonts looks for, most
// specific first: one per script where the platform has them, then
// symbols, then pan-Unicode fonts.
func systemFallbackFamilies() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"Apple SD Gothic Neo", // Korean
			"Hiragino Sans",       // Japanese
			"Hiragino Sans GB",    // Simplified Chinese
			"PingFang SC",         // Chinese
			"AppleGothic",         // Korean
			"Noto Sans Gothic",
			"Menlo",         // dingbats such as ❯ ✳ ✻
			"Apple Symbols", // technical symbols such as ⎿
			"STIX Two Math", // media symbols such as ⏵ ⏺
			// No pan-Unicode font: Arial Unicode MS takes 46 MB once
			// loaded, and a glyph none of these has comes from a font made
			// for its script (see SystemFallback).
		}
	case "windows":
		return []string{
			"Malgun Gothic", "Meiryo", "Yu Gothic", "Microsoft YaHei", "Microsoft JhengHei", "SimSun",
			"Segoe UI", "Segoe UI Symbol", "Cambria Math",
		}
	case "js":
		return nil
	}
	return []string{
		"Noto Sans CJK KR", "Noto Sans CJK JP", "Noto Sans CJK SC", "Noto Sans CJK TC",
		"Noto Sans KR", "Noto Sans JP", "Noto Sans SC",
		"NanumGothic", "Droid Sans Fallback", "WenQuanYi Micro Hei", "WenQuanYi Zen Hei",
		"Noto Sans Symbols", "Noto Sans Symbols 2", "Noto Sans Math", "DejaVu Sans",
	}
}
