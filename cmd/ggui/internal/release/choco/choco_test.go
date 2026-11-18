package choco

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"flag"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files")

func pkg() Package {
	return Package{
		ID: "hello-world", Version: "1.2.0", Title: "Hello World", Authors: "Alice",
		ProjectURL: "https://hello.example.com", LicenseURL: "https://github.com/alice/hello/blob/main/LICENSE",
		IconURL: "https://hello.example.com/icon.png", PackageSourceURL: "https://github.com/alice/hello",
		Tags: []string{"hello", "gui"}, Summary: "Says hello", Description: "Hello World says hello & means it.",
		Exe:   "Hello World.exe",
		URL64: "https://github.com/alice/hello/releases/download/v1.2.0/hello-world-1.2.0-windows-amd64.zip", Checksum64: strings.Repeat("a", 64),
		URL32: "https://github.com/alice/hello/releases/download/v1.2.0/hello-world-1.2.0-windows-386.zip", Checksum32: strings.Repeat("b", 64),
	}
}

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs:\n%s", path, got)
	}
}

func TestGolden(t *testing.T) {
	p := pkg()
	golden(t, "hello-world.nuspec", p.Nuspec())
	golden(t, "chocolateyinstall.ps1", p.InstallScript())
	golden(t, "chocolateyuninstall.ps1", p.UninstallScript())

	// Without a 386 build, only the 64-bit URL.
	p.URL32, p.Checksum32 = "", ""
	s := string(p.InstallScript())
	if strings.Contains(s, "  url ") || !strings.Contains(s, "url64bit") {
		t.Errorf("amd64 only:\n%s", s)
	}
}

func TestNuspecParses(t *testing.T) {
	var n struct {
		XMLName  xml.Name
		Metadata struct {
			ID               string `xml:"id"`
			Version          string `xml:"version"`
			Authors          string `xml:"authors"`
			Description      string `xml:"description"`
			Tags             string `xml:"tags"`
			ProjectURL       string `xml:"projectUrl"`
			PackageSourceURL string `xml:"packageSourceUrl"`
		} `xml:"metadata"`
	}
	if err := xml.Unmarshal(pkg().Nuspec(), &n); err != nil {
		t.Fatal(err)
	}
	m := n.Metadata
	if n.XMLName.Space != NuspecNS || m.ID != "hello-world" || m.Version != "1.2.0" || m.Tags != "hello gui" ||
		m.Description != "Hello World says hello & means it." || m.PackageSourceURL != "https://github.com/alice/hello" {
		t.Errorf("parsed %+v %+v", n.XMLName, m)
	}
}

func TestCheck(t *testing.T) {
	p := pkg()
	p.Authors, p.URL64, p.URL32 = "", "", ""
	err := p.Check()
	if err == nil || !strings.Contains(err.Error(), "authors") || !strings.Contains(err.Error(), "download") {
		t.Errorf("err = %v", err)
	}
}

// constReader supplies the same byte forever, for names made of random
// bytes.
type constReader byte

func (c constReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

func TestPack(t *testing.T) {
	dir := t.TempDir()
	p := pkg()
	out := filepath.Join(dir, p.FileName())
	if p.FileName() != "hello-world.1.2.0.nupkg" {
		t.Errorf("FileName = %s", p.FileName())
	}
	if err := p.Pack(out, constReader(0xab)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	parts := map[string]string{}
	var names []string
	for _, f := range zr.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name] = string(data)
		names = append(names, f.Name)
	}
	psmdcp := "package/services/metadata/core-properties/" + strings.Repeat("ab", 16) + ".psmdcp"
	want := []string{"hello-world.nuspec", "tools/chocolateyinstall.ps1", "tools/chocolateyuninstall.ps1", "_rels/.rels", psmdcp, "[Content_Types].xml"}
	if !slices.Equal(names, want) {
		t.Fatalf("parts %v, want %v", names, want)
	}
	if parts["hello-world.nuspec"] != string(p.Nuspec()) {
		t.Error("the packed nuspec differs")
	}
	if ps := parts["tools/chocolateyinstall.ps1"]; !strings.HasPrefix(ps, "\uFEFF# Written") || !strings.Contains(ps, "\r\n") {
		t.Errorf("install script has no BOM or CRLF: %q", ps[:20])
	}

	var rels struct {
		Rels []struct {
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
			ID     string `xml:"Id,attr"`
		} `xml:"Relationship"`
	}
	if err := xml.Unmarshal([]byte(parts["_rels/.rels"]), &rels); err != nil {
		t.Fatal(err)
	}
	if len(rels.Rels) != 2 || rels.Rels[0].Target != "/hello-world.nuspec" || rels.Rels[0].Type != manifestRel ||
		rels.Rels[1].Target != "/"+psmdcp || !strings.HasPrefix(rels.Rels[0].ID, "R") {
		t.Errorf(".rels %+v", rels)
	}

	var types struct {
		Defaults []struct {
			Extension   string `xml:"Extension,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Default"`
	}
	if err := xml.Unmarshal([]byte(parts["[Content_Types].xml"]), &types); err != nil {
		t.Fatal(err)
	}
	var exts []string
	for _, d := range types.Defaults {
		exts = append(exts, d.Extension)
	}
	if !slices.Equal(exts, []string{"rels", "psmdcp", "nuspec", "ps1"}) {
		t.Errorf("content types for %v", exts)
	}

	var core struct {
		Identifier string `xml:"identifier"`
		Version    string `xml:"version"`
		Creator    string `xml:"creator"`
	}
	if err := xml.Unmarshal([]byte(parts[psmdcp]), &core); err != nil {
		t.Fatal(err)
	}
	if core.Identifier != "hello-world" || core.Version != "1.2.0" || core.Creator != "Alice" {
		t.Errorf("core properties %+v\n%s", core, parts[psmdcp])
	}

	// The source written beside it is the same package.
	src := filepath.Join(dir, "src")
	if err := p.Write(src); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(src, "tools", "chocolateyinstall.ps1")); err != nil || string(data) != parts["tools/chocolateyinstall.ps1"] {
		t.Errorf("written install script differs: %v", err)
	}
}

func TestPush(t *testing.T) {
	const key = "choco-key-123"
	var (
		method, path, apiKey string
		field, file          string
		body                 []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, apiKey = r.Method, r.URL.Path, r.Header.Get("X-NuGet-ApiKey")
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || mt != "multipart/form-data" {
			t.Errorf("Content-Type %q", r.Header.Get("Content-Type"))
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		part, err := mr.NextPart()
		if err != nil {
			t.Error(err)
			return
		}
		field, file = part.FormName(), part.FileName()
		body, _ = io.ReadAll(part)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	nupkg := filepath.Join(t.TempDir(), "x.nupkg")
	if err := os.WriteFile(nupkg, []byte("PK fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Push(t.Context(), nil, srv.URL+"/", key, nupkg); err != nil {
		t.Fatal(err)
	}
	if method != "PUT" || path != "/api/v2/package/" || apiKey != key || field != "package" || file == "" || string(body) != "PK fake" {
		t.Errorf("pushed %s %s key %q field %q file %q body %q", method, path, apiKey, field, file, body)
	}

	if err := Push(t.Context(), nil, srv.URL, "", nupkg); err == nil || !strings.Contains(err.Error(), "CHOCOLATEY_API_KEY") {
		t.Errorf("no key: %v", err)
	}

	// A refusal that echoes the key does not print it.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("package exists; key " + r.Header.Get("X-NuGet-ApiKey")))
	}))
	defer echo.Close()
	err := Push(t.Context(), nil, echo.URL, key, nupkg)
	if err == nil || strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "409") {
		t.Errorf("err = %v", err)
	}
}
