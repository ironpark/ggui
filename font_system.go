package ggui

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

// systemUIFonts loads the platform's interface fonts once, with every
// weight it has of them, or returns nil for one it does not find: SF Pro
// and SF Mono on macOS, Segoe UI and Cascadia Mono or Consolas on Windows,
// and what fontconfig matches for sans-serif and monospace elsewhere.
// Browsers have no filesystem fonts.
var systemUIFonts = sync.OnceValues(func() (sans, mono *Font) {
	sansFiles, monoFiles := systemUIFiles()
	return loadFamily(sansFiles), loadFamily(monoFiles)
})

// systemUIFiles lists the faces of the platform's interface fonts, the
// regular one first.
func systemUIFiles() (sans, mono []fontFile) {
	switch runtime.GOOS {
	case "darwin":
		// Both are variable fonts, which draw every weight.
		return []fontFile{{path: "/System/Library/Fonts/SFNS.ttf"}}, []fontFile{{path: "/System/Library/Fonts/SFNSMono.ttf"}}
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		existing := func(names ...string) []fontFile {
			var out []fontFile
			for _, name := range names {
				if p := filepath.Join(dir, name); fileExists(p) {
					out = append(out, fontFile{path: p})
				}
			}
			return out
		}
		sans = existing("SegUIVar.ttf") // Windows 11, a variable font
		if len(sans) == 0 {
			sans = existing("segoeui.ttf", "segoeuil.ttf", "seguisb.ttf", "segoeuib.ttf")
		}
		mono = existing("CascadiaMono.ttf")
		if len(mono) == 0 {
			mono = existing("consola.ttf", "consolab.ttf")
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

// SystemItalicFonts returns the italic faces of the platform's interface
// fonts, with every weight it has of them, loaded once: SF Pro Italic and
// SF Mono Italic on macOS, Segoe UI Italic and Cascadia Mono Italic or
// Consolas Italic on Windows, and what fontconfig matches for sans-serif
// and monospace in italic elsewhere. Either is nil where the platform has
// no italic of that font, and fontconfig's match for an italic it does not
// have, an upright face, is not taken for one. Like any Font they fall back
// to SystemFonts. Text has no italic style of its own; a widget that draws
// italics, such as a terminal, chooses these for them.
func SystemItalicFonts() (sans, mono *Font) { return systemItalicFonts() }

var systemItalicFonts = sync.OnceValues(func() (sans, mono *Font) {
	sansFiles, monoFiles := systemItalicFiles()
	italic := func(f *Font) *Font {
		if f == nil || f.meta().Metadata().Style != text.StyleItalic {
			return nil
		}
		return f
	}
	return italic(loadFamily(sansFiles)), italic(loadFamily(monoFiles))
})

// systemItalicFiles lists the faces of the italics of the platform's
// interface fonts, the regular weight first.
func systemItalicFiles() (sans, mono []fontFile) {
	switch runtime.GOOS {
	case "darwin":
		return []fontFile{{path: "/System/Library/Fonts/SFNSItalic.ttf"}}, []fontFile{{path: "/System/Library/Fonts/SFNSMonoItalic.ttf"}}
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		existing := func(names ...string) []fontFile {
			var out []fontFile
			for _, name := range names {
				if p := filepath.Join(dir, name); fileExists(p) {
					out = append(out, fontFile{path: p})
				}
			}
			return out
		}
		sans = existing("segoeuii.ttf", "seguisbi.ttf", "segoeuiz.ttf")
		mono = existing("CascadiaMonoItalic.ttf")
		if len(mono) == 0 {
			mono = existing("consolai.ttf", "consolaz.ttf")
		}
		return sans, mono
	case "js", "wasip1", "ios", "android":
		return nil, nil
	}
	return fontconfig("sans-serif:italic"), fontconfig("monospace:italic")
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
	style := first.meta().Metadata().Style
	for _, f := range faces {
		if f != first && f.meta().Metadata().Style == style {
			out = append(out, f)
		}
	}
	return out, nil
}
