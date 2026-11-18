// Package linuxtest reads back the packages package linux makes, for
// tests: a Debian package's control fields and files, from its ar archive
// and the tarballs in it, and an RPM package's header, from the format's
// lead and header structures. It needs neither dpkg nor rpm.
package linuxtest

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// Deb is what a Debian package holds.
type Deb struct {
	// Members are the ar archive's members in order: debian-binary,
	// control.tar.gz and data.tar.gz.
	Members []string
	// Control is the control file's fields; a continued field's lines
	// are joined by newlines.
	Control map[string]string
	// Scripts are the maintainer scripts in control.tar.gz, by name.
	Scripts map[string]string
	// Files are data.tar.gz's regular files, by path from /, with their
	// modes.
	Files map[string]fs.FileMode
	// Size is each file's size.
	Size map[string]int64
}

// ReadDeb reads the Debian package at path.
func ReadDeb(path string) (*Deb, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("!<arch>\n")) {
		return nil, errors.New("not an ar archive")
	}
	d := &Deb{Control: map[string]string{}, Scripts: map[string]string{}, Files: map[string]fs.FileMode{}, Size: map[string]int64{}}
	members := map[string][]byte{}
	for off := 8; off < len(data); {
		if off+60 > len(data) {
			return nil, errors.New("truncated ar header")
		}
		h := data[off : off+60]
		if string(h[58:60]) != "`\n" {
			return nil, fmt.Errorf("bad ar header at %d", off)
		}
		name := strings.TrimSuffix(strings.TrimSpace(string(h[0:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(h[48:58])))
		if err != nil {
			return nil, err
		}
		off += 60
		if off+size > len(data) {
			return nil, errors.New("truncated ar member")
		}
		members[name] = data[off : off+size]
		d.Members = append(d.Members, name)
		off += size + size%2
	}
	if string(members["debian-binary"]) != "2.0\n" {
		return nil, fmt.Errorf("debian-binary is %q", members["debian-binary"])
	}
	err = readTarGz(members["control.tar.gz"], func(h *tar.Header, r io.Reader) error {
		body, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		switch name := strings.TrimPrefix(h.Name, "./"); name {
		case "control":
			d.Control = parseControl(string(body))
		case "":
		default:
			d.Scripts[name] = string(body)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("control.tar.gz: %w", err)
	}
	err = readTarGz(members["data.tar.gz"], func(h *tar.Header, r io.Reader) error {
		if h.Typeflag == tar.TypeReg {
			name := "/" + strings.TrimPrefix(h.Name, "./")
			d.Files[name] = fs.FileMode(h.Mode).Perm()
			d.Size[name] = h.Size
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("data.tar.gz: %w", err)
	}
	return d, nil
}

func readTarGz(data []byte, fn func(*tar.Header, io.Reader) error) error {
	if data == nil {
		return errors.New("missing")
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(h, tr); err != nil {
			return err
		}
	}
}

// parseControl reads deb822 fields: "Key: value", continued on lines
// that start with a space.
func parseControl(s string) map[string]string {
	fields := map[string]string{}
	last := ""
	for line := range strings.SplitSeq(s, "\n") {
		switch {
		case line == "":
		case line[0] == ' ' || line[0] == '\t':
			if last != "" {
				fields[last] += "\n" + strings.TrimSpace(line)
			}
		default:
			k, v, _ := strings.Cut(line, ":")
			last = k
			fields[k] = strings.TrimSpace(v)
		}
	}
	return fields
}

// RPM is what an RPM package's header says.
type RPM struct {
	Name, Version, Release, Arch, Summary, License string
	Requires                                       []string
	PostIn, PostUn                                 string
	// Files are the files it installs, by path, with their modes.
	Files map[string]fs.FileMode
}

// The header tags read.
const (
	tagName        = 1000
	tagVersion     = 1001
	tagRelease     = 1002
	tagSummary     = 1004
	tagLicense     = 1014
	tagArch        = 1022
	tagPostIn      = 1024
	tagPostUn      = 1026
	tagFileModes   = 1030
	tagRequireName = 1049
	tagDirIndexes  = 1116
	tagBaseNames   = 1117
	tagDirNames    = 1118
)

// ReadRPM reads the header of the RPM package at path.
func ReadRPM(path string) (*RPM, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 96 || !bytes.Equal(data[:4], []byte{0xed, 0xab, 0xee, 0xdb}) {
		return nil, errors.New("no RPM lead")
	}
	// The signature header, padded to 8 bytes, then the header.
	_, n, err := readHeader(data[96:])
	if err != nil {
		return nil, fmt.Errorf("signature: %w", err)
	}
	off := 96 + n
	off += (8 - off%8) % 8
	tags, _, err := readHeader(data[off:])
	if err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}
	str := func(tag int) string {
		if s, ok := tags[tag].([]string); ok && len(s) > 0 {
			return s[0]
		}
		return ""
	}
	strs := func(tag int) []string { s, _ := tags[tag].([]string); return s }
	ints := func(tag int) []int { s, _ := tags[tag].([]int); return s }
	r := &RPM{
		Name: str(tagName), Version: str(tagVersion), Release: str(tagRelease),
		Arch: str(tagArch), Summary: str(tagSummary), License: str(tagLicense),
		Requires: strs(tagRequireName),
		PostIn:   str(tagPostIn), PostUn: str(tagPostUn),
		Files: map[string]fs.FileMode{},
	}
	base, dirs, idx, modes := strs(tagBaseNames), strs(tagDirNames), ints(tagDirIndexes), ints(tagFileModes)
	if len(idx) != len(base) || len(modes) != len(base) {
		return nil, fmt.Errorf("%d base names, %d directory indexes, %d modes", len(base), len(idx), len(modes))
	}
	for i, b := range base {
		if idx[i] >= len(dirs) {
			return nil, errors.New("directory index out of range")
		}
		// Directories the package owns are listed too; the mode's type
		// bits tell them apart.
		if modes[i]&0o170000 == 0o100000 {
			r.Files[dirs[idx[i]]+b] = fs.FileMode(modes[i] & 0o777)
		}
	}
	return r, nil
}

// readHeader reads a header structure: its tags' values, as []string for
// the string types and []int for the integer ones, and its length.
func readHeader(b []byte) (map[int]any, int, error) {
	if len(b) < 16 || !bytes.Equal(b[:4], []byte{0x8e, 0xad, 0xe8, 0x01}) {
		return nil, 0, errors.New("no header magic")
	}
	be := binary.BigEndian
	nindex, hsize := int(be.Uint32(b[8:])), int(be.Uint32(b[12:]))
	store := 16 + 16*nindex
	if store+hsize > len(b) {
		return nil, 0, errors.New("truncated")
	}
	data := b[store : store+hsize]
	tags := map[int]any{}
	for i := range nindex {
		e := b[16+16*i:]
		tag, typ, off, count := int(be.Uint32(e)), be.Uint32(e[4:]), int(be.Uint32(e[8:])), int(be.Uint32(e[12:]))
		if off > len(data) {
			return nil, 0, fmt.Errorf("tag %d: offset out of range", tag)
		}
		switch typ {
		case 6, 8, 9: // STRING, STRING_ARRAY, I18NSTRING
			var ss []string
			p := data[off:]
			for range count {
				end := bytes.IndexByte(p, 0)
				if end < 0 {
					return nil, 0, fmt.Errorf("tag %d: unterminated string", tag)
				}
				ss = append(ss, string(p[:end]))
				p = p[end+1:]
			}
			tags[tag] = ss
		case 3: // INT16
			if off+2*count > len(data) {
				return nil, 0, fmt.Errorf("tag %d: truncated", tag)
			}
			var v []int
			for j := range count {
				v = append(v, int(be.Uint16(data[off+2*j:])))
			}
			tags[tag] = v
		case 4: // INT32
			if off+4*count > len(data) {
				return nil, 0, fmt.Errorf("tag %d: truncated", tag)
			}
			var v []int
			for j := range count {
				v = append(v, int(be.Uint32(data[off+4*j:])))
			}
			tags[tag] = v
		}
	}
	return tags, store + hsize, nil
}
