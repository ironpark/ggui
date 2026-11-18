package linux

import (
	"bytes"
	"encoding/xml"
)

// component is an AppStream component of type desktop-application, with
// the fields software centres need to list the app:
// https://www.freedesktop.org/software/appstream/docs/chap-Metadata.html
type component struct {
	XMLName         xml.Name     `xml:"component"`
	Type            string       `xml:"type,attr"`
	ID              string       `xml:"id"`
	MetadataLicense string       `xml:"metadata_license"`
	ProjectLicense  string       `xml:"project_license,omitempty"`
	Name            string       `xml:"name"`
	Summary         string       `xml:"summary"`
	Launchable      launchable   `xml:"launchable"`
	URLs            []url        `xml:"url,omitempty"`
	Provides        provides     `xml:"provides"`
	Releases        *releaseList `xml:"releases,omitempty"`
}

type launchable struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type url struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

type provides struct {
	Binary string `xml:"binary"`
}

type releaseList struct {
	Releases []release `xml:"release"`
}

type release struct {
	Version string `xml:"version,attr"`
}

// Metainfo returns the AppStream metadata, ID.metainfo.xml in
// /usr/share/metainfo, which software centres such as GNOME Software and
// KDE Discover show the app from. The metadata itself is CC0-1.0, as
// AppStream recommends, so that catalogs may copy it.
func Metainfo(a App) []byte {
	c := component{
		Type:            "desktop-application",
		ID:              a.ID,
		MetadataLicense: "CC0-1.0",
		ProjectLicense:  a.License,
		Name:            a.Name,
		Summary:         a.summary(),
		Launchable:      launchable{Type: "desktop-id", Value: a.ID + ".desktop"},
		Provides:        provides{Binary: a.Package},
	}
	if a.Homepage != "" {
		c.URLs = append(c.URLs, url{Type: "homepage", Value: a.Homepage})
	}
	if a.Version != "" {
		c.Releases = &releaseList{Releases: []release{{Version: a.Version}}}
	}
	var b bytes.Buffer
	b.WriteString(xml.Header)
	enc := xml.NewEncoder(&b)
	enc.Indent("", "  ")
	if err := enc.Encode(c); err != nil {
		panic(err) // the struct is fixed, and its fields strings
	}
	b.WriteByte('\n')
	return b.Bytes()
}
