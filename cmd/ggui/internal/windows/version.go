package windows

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// ParseVersion reads a version of up to four numbers, as app.version
// gives one to three, into the four 16-bit parts a Windows version has,
// the missing ones 0. Windows keeps each part in 16 bits, so a part above
// 65535, which macOS would take, is an error here.
func ParseVersion(v string) ([4]uint16, error) {
	var out [4]uint16
	parts := strings.Split(v, ".")
	if v == "" || len(parts) > 4 {
		return out, fmt.Errorf("version %q must be one to four numbers separated by dots, such as 1.2.0", v)
	}
	for i, p := range parts {
		n, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return out, fmt.Errorf("version %q: Windows keeps each part of a version as a number from 0 to 65535", v)
		}
		out[i] = uint16(n)
	}
	return out, nil
}

// versionString writes a version as its four parts, as a manifest's
// assemblyIdentity must have it.
func versionString(v [4]uint16) string {
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

// Version is what the version resource says, which Explorer's Properties,
// Details shows and Task Manager names the program by.
type Version struct {
	Version     [4]uint16
	Text        string // the version as written, for the text fields
	Product     string // ProductName
	Description string // FileDescription: what Task Manager calls the process
	Company     string // CompanyName
	Copyright   string // LegalCopyright
	Internal    string // InternalName: the program's name without .exe
	Original    string // OriginalFilename: Name.exe
}

// Block types of a version resource: binary or text values.
const (
	binaryValue = 0
	textValue   = 1
)

// versionInfo returns the VS_VERSIONINFO resource: the fixed file info,
// then the strings in one table, US English in Unicode, and the
// translation that names that table.
//
// Every block is the same shape: its length, its value's length, its
// type, its key as NUL-terminated UTF-16, padding to 32 bits, its value,
// and its children, each starting on a 32-bit boundary.
func versionInfo(v Version) []byte {
	ms := uint32(v.Version[0])<<16 | uint32(v.Version[1])
	ls := uint32(v.Version[2])<<16 | uint32(v.Version[3])
	fixed := make([]byte, 52)
	le := binary.LittleEndian
	for i, f := range []uint32{
		0xFEEF04BD, // dwSignature
		0x00010000, // dwStrucVersion 1.0
		ms, ls,     // dwFileVersion
		ms, ls, // dwProductVersion
		0x3F,       // dwFileFlagsMask: every flag; none is set
		0,          // dwFileFlags
		0x00040004, // dwFileOS: VOS_NT_WINDOWS32
		1,          // dwFileType: VFT_APP
		0, 0, 0,    // dwFileSubtype, dwFileDate
	} {
		le.PutUint32(fixed[4*i:], f)
	}

	var strs []*block
	for _, s := range []struct{ key, value string }{
		{"CompanyName", v.Company},
		{"FileDescription", v.Description},
		{"FileVersion", v.Text},
		{"InternalName", v.Internal},
		{"LegalCopyright", v.Copyright},
		{"OriginalFilename", v.Original},
		{"ProductName", v.Product},
		{"ProductVersion", v.Text},
	} {
		if s.value != "" {
			strs = append(strs, text(s.key, s.value))
		}
	}
	translation := make([]byte, 4)
	le.PutUint16(translation[0:], langENUS)
	le.PutUint16(translation[2:], 1200) // Unicode, the code page of the strings
	root := &block{key: "VS_VERSION_INFO", value: fixed, valueLen: len(fixed), kids: []*block{
		{key: "StringFileInfo", typ: textValue, kids: []*block{
			{key: fmt.Sprintf("%04X04B0", langENUS), typ: textValue, kids: strs},
		}},
		{key: "VarFileInfo", typ: textValue, kids: []*block{
			{key: "Translation", value: translation, valueLen: len(translation)},
		}},
	}}
	var b bytes.Buffer
	root.write(&b)
	return b.Bytes()
}

// block is one block of a version resource.
type block struct {
	key   string
	typ   uint16
	value []byte
	// valueLen is wValueLength: bytes for a binary value, UTF-16 units
	// with the NUL for text.
	valueLen int
	kids     []*block
}

// text returns a string's block.
func text(key, value string) *block {
	u := utf16.Encode([]rune(value + "\x00"))
	return &block{key: key, typ: textValue, value: utf16Bytes(u), valueLen: len(u)}
}

func utf16Bytes(u []uint16) []byte {
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

// write appends the block to b, which holds whole blocks so far, so that
// an offset in b is 32-bit aligned when the resource's is. Its length
// covers its children but not the padding after the last.
func (k *block) write(b *bytes.Buffer) {
	start := b.Len()
	b.Write(make([]byte, 6)) // wLength, wValueLength, wType: below
	b.Write(utf16Bytes(utf16.Encode([]rune(k.key + "\x00"))))
	pad4(b)
	b.Write(k.value)
	for _, c := range k.kids {
		pad4(b)
		c.write(b)
	}
	out := b.Bytes()
	binary.LittleEndian.PutUint16(out[start:], uint16(b.Len()-start))
	binary.LittleEndian.PutUint16(out[start+2:], uint16(k.valueLen))
	binary.LittleEndian.PutUint16(out[start+4:], k.typ)
}

func pad4(b *bytes.Buffer) {
	for b.Len()%4 != 0 {
		b.WriteByte(0)
	}
}
