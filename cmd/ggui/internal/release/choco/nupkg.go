package choco

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

// The parts of the Open Packaging Conventions zip a .nupkg is, as
// NuGet's PackageBuilder, which choco pack runs, writes them.
const (
	relsPath         = "_rels/.rels"
	contentTypesPath = "[Content_Types].xml"
	corePropsDir     = "package/services/metadata/core-properties/"

	relsNS         = "http://schemas.openxmlformats.org/package/2006/relationships"
	contentTypesNS = "http://schemas.openxmlformats.org/package/2006/content-types"
	corePropsNS    = "http://schemas.openxmlformats.org/package/2006/metadata/core-properties"
	manifestRel    = "http://schemas.microsoft.com/packaging/2010/07/manifest"
	corePropsRel   = "http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties"
)

// Pack writes the package to out as a .nupkg. random supplies the bytes
// the part names and relationship IDs are made of, as NuGet makes them
// from GUIDs; crypto/rand.Reader outside tests.
func (p Package) Pack(out string, random io.Reader) (err error) {
	if err := p.Check(); err != nil {
		return err
	}
	id := func(n int) (string, error) {
		b := make([]byte, n)
		if _, err := io.ReadFull(random, b); err != nil {
			return "", err
		}
		return hex.EncodeToString(b), nil
	}
	psmdcp, err := id(16)
	if err != nil {
		return err
	}
	relManifest, err := id(8)
	if err != nil {
		return err
	}
	relCore, err := id(8)
	if err != nil {
		return err
	}
	coreName := corePropsDir + psmdcp + ".psmdcp"
	files := p.Files()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, data []byte) error {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	for _, f := range files {
		if err := add(f.Path, f.Data); err != nil {
			return err
		}
	}
	rels := xmlDoc(relationships{Xmlns: relsNS, Rels: []relationship{
		{Type: manifestRel, Target: "/" + files[0].Path, ID: "R" + relManifest},
		{Type: corePropsRel, Target: "/" + coreName, ID: "R" + relCore},
	}})
	if err := add(relsPath, rels); err != nil {
		return err
	}
	if err := add(coreName, xmlDoc(p.coreProperties())); err != nil {
		return err
	}
	if err := add(contentTypesPath, xmlDoc(contentTypes(files))); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}

func xmlDoc(v any) []byte {
	data, err := xml.Marshal(v)
	if err != nil {
		panic(err) // the types are fixed and hold strings
	}
	return append([]byte(xml.Header), data...)
}

type relationships struct {
	XMLName xml.Name       `xml:"Relationships"`
	Xmlns   string         `xml:"xmlns,attr"`
	Rels    []relationship `xml:"Relationship"`
}

type relationship struct {
	Type   string `xml:"Type,attr"`
	Target string `xml:"Target,attr"`
	ID     string `xml:"Id,attr"`
}

type types struct {
	XMLName  xml.Name      `xml:"Types"`
	Xmlns    string        `xml:"xmlns,attr"`
	Defaults []defaultType `xml:"Default"`
}

type defaultType struct {
	Extension   string `xml:"Extension,attr"`
	ContentType string `xml:"ContentType,attr"`
}

// contentTypes lists a type for each extension in the package, as NuGet
// does: its own two, and application/octet for the rest.
func contentTypes(files []File) types {
	t := types{Xmlns: contentTypesNS, Defaults: []defaultType{
		{"rels", "application/vnd.openxmlformats-package.relationships+xml"},
		{"psmdcp", "application/vnd.openxmlformats-package.core-properties+xml"},
	}}
	var exts []string
	for _, f := range files {
		if e := strings.TrimPrefix(path.Ext(f.Path), "."); e != "" && !slices.Contains(exts, e) {
			exts = append(exts, e)
		}
	}
	for _, e := range exts {
		t.Defaults = append(t.Defaults, defaultType{e, "application/octet"})
	}
	return t
}

type coreProperties struct {
	XMLName        xml.Name `xml:"coreProperties"`
	XmlnsDC        string   `xml:"xmlns:dc,attr"`
	XmlnsDCTerms   string   `xml:"xmlns:dcterms,attr"`
	XmlnsXSI       string   `xml:"xmlns:xsi,attr"`
	Xmlns          string   `xml:"xmlns,attr"`
	Creator        string   `xml:"dc:creator"`
	Description    string   `xml:"dc:description"`
	Identifier     string   `xml:"dc:identifier"`
	Version        string   `xml:"version"`
	Keywords       string   `xml:"keywords"`
	Title          string   `xml:"dc:title,omitempty"`
	LastModifiedBy string   `xml:"lastModifiedBy"`
}

func (p Package) coreProperties() coreProperties {
	return coreProperties{
		XmlnsDC:      "http://purl.org/dc/elements/1.1/",
		XmlnsDCTerms: "http://purl.org/dc/terms/",
		XmlnsXSI:     "http://www.w3.org/2001/XMLSchema-instance",
		Xmlns:        corePropsNS,
		Creator:      p.Authors, Description: p.Description, Identifier: p.ID,
		Version: p.Version, Keywords: strings.Join(p.Tags, " "), Title: p.Title,
		LastModifiedBy: "ggui release",
	}
}

// PushURL is where choco push sends a package for a source: the NuGet v2
// API's package endpoint below it.
func PushURL(source string) string {
	return strings.TrimRight(source, "/") + "/api/v2/package/"
}

// Push sends the .nupkg at file to source as choco push does: a PUT of
// multipart/form-data with the package as its one file, and the API key
// in X-NuGet-ApiKey.
func Push(ctx context.Context, hc *http.Client, source, apiKey, file string) error {
	if apiKey == "" {
		return errors.New("no Chocolatey API key: set $CHOCOLATEY_API_KEY to the key of your chocolatey.org account")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="package"; filename="package.nupkg"`)
	h.Set("Content-Type", "application/octet-stream")
	pw, err := mw.CreatePart(h)
	if err != nil {
		return err
	}
	if _, err := pw.Write(data); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	u := PushURL(source)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body.Bytes()))
	if err != nil {
		return err
	}
	req.ContentLength = int64(body.Len())
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-NuGet-ApiKey", apiKey)
	req.Header.Set("X-NuGet-Protocol-Version", "4.1.0")
	req.Header.Set("User-Agent", "ggui-release")
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Minute}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf("pushing to %s: %w", u, err)
	}
	defer resp.Body.Close()
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		text := strings.TrimSpace(string(msg))
		if text == "" {
			text = http.StatusText(resp.StatusCode)
		}
		if resp.StatusCode == http.StatusConflict {
			text += "; that version was pushed already"
		}
		return fmt.Errorf("PUT %s: %d %s", u, resp.StatusCode, strings.ReplaceAll(text, apiKey, "(the API key)"))
	}
	return nil
}
