// Package choco makes the Chocolatey package of a Windows release, packs
// it as choco pack would, and pushes it to a NuGet v2 feed as choco push
// does.
//
// The package does not embed the program: its install script downloads
// the zip of the GitHub release and checks its SHA-256. So it carries no
// tools/VERIFICATION.txt and LICENSE.txt, which the community repository
// asks for only of a package with binaries inside.
package choco

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Package is what the package says and installs.
type Package struct {
	ID               string
	Version          string
	Title            string // the app's name
	Authors          string
	ProjectURL       string
	LicenseURL       string
	IconURL          string
	PackageSourceURL string // where the package's own source is
	Tags             []string
	Summary          string
	Description      string

	// Exe is the program's file name in the zip, Name.exe.
	Exe string
	// URL64 and Checksum64 are the amd64 zip and its SHA-256; URL32 and
	// Checksum32 the 386 one, "" when there is none. Chocolatey's
	// helpers take no ARM64 URL: Windows on Arm runs the amd64 program.
	URL64, Checksum64 string
	URL32, Checksum32 string
}

// Check reports what the package lacks that the feed requires.
func (p Package) Check() error {
	var missing []string
	for _, f := range []struct{ v, name string }{
		{p.ID, "id"}, {p.Version, "version"}, {p.Authors, "authors"}, {p.Description, "description"}, {p.Exe, "the program"},
	} {
		if strings.TrimSpace(f.v) == "" {
			missing = append(missing, f.name)
		}
	}
	if p.URL64 == "" && p.URL32 == "" {
		missing = append(missing, "a Windows amd64 or 386 download")
	}
	if len(missing) > 0 {
		return fmt.Errorf("the Chocolatey package needs %s", strings.Join(missing, ", "))
	}
	return nil
}

// NuspecNS is the nuspec schema choco new writes.
const NuspecNS = "http://schemas.microsoft.com/packaging/2015/06/nuspec.xsd"

type nuspec struct {
	XMLName  xml.Name `xml:"package"`
	Xmlns    string   `xml:"xmlns,attr"`
	Metadata metadata `xml:"metadata"`
}

type metadata struct {
	ID                       string `xml:"id"`
	Version                  string `xml:"version"`
	PackageSourceURL         string `xml:"packageSourceUrl,omitempty"`
	Title                    string `xml:"title,omitempty"`
	Authors                  string `xml:"authors"`
	ProjectURL               string `xml:"projectUrl,omitempty"`
	IconURL                  string `xml:"iconUrl,omitempty"`
	LicenseURL               string `xml:"licenseUrl,omitempty"`
	RequireLicenseAcceptance bool   `xml:"requireLicenseAcceptance"`
	Tags                     string `xml:"tags,omitempty"`
	Summary                  string `xml:"summary,omitempty"`
	Description              string `xml:"description"`
}

// Nuspec writes the package's manifest.
func (p Package) Nuspec() []byte {
	n := nuspec{Xmlns: NuspecNS, Metadata: metadata{
		ID: p.ID, Version: p.Version, PackageSourceURL: p.PackageSourceURL,
		Title: p.Title, Authors: p.Authors, ProjectURL: p.ProjectURL,
		IconURL: p.IconURL, LicenseURL: p.LicenseURL,
		Tags: strings.Join(p.Tags, " "), Summary: p.Summary, Description: p.Description,
	}}
	data, err := xml.MarshalIndent(n, "", "  ")
	if err != nil {
		panic(err) // strings and a bool always marshal
	}
	return append([]byte(xml.Header+"<!-- Written by ggui release. -->\n"), append(data, '\n')...)
}

var psSingleQuote = strings.NewReplacer(
	"'", "''",
	"\u2018", "\u2018\u2018",
	"\u2019", "\u2019\u2019",
	"\u201a", "\u201a\u201a",
	"\u201b", "\u201b\u201b",
)

// psQuote quotes s as a PowerShell single-quoted string. PowerShell treats
// Unicode curly single quotes as single-quote delimiters as well.
func psQuote(s string) string { return "'" + psSingleQuote.Replace(s) + "'" }

// shortcutName is the Start menu shortcut's file name.
func (p Package) shortcutName() string {
	name := p.Title
	if name == "" {
		name = strings.TrimSuffix(p.Exe, ".exe")
	}
	return name + ".lnk"
}

// InstallScript writes tools/chocolateyinstall.ps1.
func (p Package) InstallScript() []byte {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w("# Written by ggui release.\n")
	w("#\n# The program is downloaded from the GitHub release and checked against\n")
	w("# its SHA-256 rather than embedded, so this package carries no\n")
	w("# VERIFICATION.txt or LICENSE.txt.\n")
	w("$ErrorActionPreference = 'Stop'\n")
	w("$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition\n\n")
	w("$packageArgs = @{\n")
	w("  packageName    = $env:ChocolateyPackageName\n")
	w("  unzipLocation  = $toolsDir\n")
	if p.URL32 != "" {
		w("  url            = %s\n", psQuote(p.URL32))
		w("  checksum       = %s\n", psQuote(p.Checksum32))
		w("  checksumType   = 'sha256'\n")
	}
	if p.URL64 != "" {
		w("  url64bit       = %s\n", psQuote(p.URL64))
		w("  checksum64     = %s\n", psQuote(p.Checksum64))
		w("  checksumType64 = 'sha256'\n")
	}
	w("}\nInstall-ChocolateyZipPackage @packageArgs\n\n")
	w("# A GUI program: the shim starts it without a console and does not wait.\n")
	w("$exe = Join-Path $toolsDir %s\n", psQuote(p.Exe))
	w("New-Item -ItemType File -Force -Path \"$exe.gui\" | Out-Null\n\n")
	w("$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) %s\n", psQuote(p.shortcutName()))
	w("Install-ChocolateyShortcut -ShortcutFilePath $shortcut -TargetPath $exe -WorkingDirectory $toolsDir\n")
	return []byte(b.String())
}

// UninstallScript writes tools/chocolateyuninstall.ps1, which removes the
// shortcut; Chocolatey removes the package's directory with the program.
func (p Package) UninstallScript() []byte {
	var b strings.Builder
	b.WriteString("# Written by ggui release.\n")
	b.WriteString("$ErrorActionPreference = 'Stop'\n\n")
	fmt.Fprintf(&b, "$shortcut = Join-Path ([Environment]::GetFolderPath('CommonPrograms')) %s\n", psQuote(p.shortcutName()))
	b.WriteString("if (Test-Path -LiteralPath $shortcut) {\n  Remove-Item -LiteralPath $shortcut -Force\n}\n")
	return []byte(b.String())
}

// Files are the package's files by their path in it, the nuspec first.
// The scripts are written as Windows editors write them, with CRLF and a
// byte order mark, without which PowerShell 5 reads them in the system's
// code page.
func (p Package) Files() []File {
	ps1 := func(data []byte) []byte {
		return append([]byte{0xEF, 0xBB, 0xBF}, bytes.ReplaceAll(data, []byte("\n"), []byte("\r\n"))...)
	}
	return []File{
		{p.ID + ".nuspec", p.Nuspec()},
		{"tools/chocolateyinstall.ps1", ps1(p.InstallScript())},
		{"tools/chocolateyuninstall.ps1", ps1(p.UninstallScript())},
	}
}

// File is a file of the package.
type File struct {
	Path string // slash-separated
	Data []byte
}

// Write writes the package's source to dir, as choco new lays it out, so
// that choco pack in dir makes the same package.
func (p Package) Write(dir string) error {
	for _, f := range p.Files() {
		path := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, f.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// FileName is the .nupkg's name, as choco pack names it.
func (p Package) FileName() string { return p.ID + "." + p.Version + ".nupkg" }
