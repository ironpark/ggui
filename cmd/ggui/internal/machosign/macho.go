// Package machosign signs macOS programs and app bundles ad hoc, as
// codesign --force --sign - does, in Go, so that a bundle built on Linux
// or Windows carries the same signature as one built on a Mac. It also
// joins thin programs into a universal one, as lipo -create does.
//
// An ad hoc signature names no one: it is a code directory, hashes of
// the program's pages and of what the bundle seals, with no certificate
// over it. That is what Apple silicon Macs need to run a program at all,
// and what makes a downloaded app one macOS offers to open after a
// warning, rather than one it calls damaged.
//
// The signature is the one codesign makes for the same input: a SHA-256
// code directory of version 0x20400, with 16 KB pages for arm64 and 4 KB
// pages for x86_64; an empty requirement set; the entitlements, as given
// and in DER; and an empty CMS blob.
package machosign

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/ggui/cmd/ggui/internal/shared"
)

// Options says what goes in a program's signature besides its own pages.
type Options struct {
	// Identifier names the program in its signature: a bundle's
	// CFBundleIdentifier.
	Identifier string
	// Entitlements is an entitlements property list, or nil.
	Entitlements []byte
	// InfoPlist and Resources are a bundle's Info.plist and
	// _CodeSignature/CodeResources, whose hashes the main program's
	// signature binds; nil outside a bundle.
	InfoPlist, Resources []byte
}

const (
	magic64  = 0xfeedfacf
	fatMagic = 0xcafebabe

	cpuX86_64 = 0x01000007
	cpuARM64  = 0x0100000c

	lcSegment64     = 0x19
	lcUUID          = 0x1b
	lcCodeSignature = 0x1d

	mhExecute = 2

	headerSize = 32 // mach_header_64
)

// IsMachO reports whether data starts like a 64-bit or universal Mach-O
// file.
func IsMachO(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	return binary.LittleEndian.Uint32(data) == magic64 || isFat(data)
}

// isFat reports whether data is a universal file rather than a Java
// class, which shares its magic number: a class file's next word is its
// version, 45 or more, and no universal file has that many slices.
func isFat(data []byte) bool {
	be := binary.BigEndian
	return len(data) >= 8 && be.Uint32(data) == fatMagic && be.Uint32(data[4:]) < 45
}

// Sign returns the thin or universal program data signed ad hoc,
// replacing any signature it had.
func Sign(data []byte, o Options) ([]byte, error) {
	if o.Identifier == "" {
		return nil, errors.New("signing needs an identifier")
	}
	var der []byte
	if o.Entitlements != nil {
		var err error
		if o.Entitlements, err = xmlEntitlements(o.Entitlements); err != nil {
			return nil, fmt.Errorf("entitlements: %w", err)
		}
		if der, err = DEREntitlements(o.Entitlements); err != nil {
			return nil, fmt.Errorf("entitlements: %w", err)
		}
	}
	if !isFat(data) {
		return signThin(data, o, der)
	}
	slices, err := fatSlices(data)
	if err != nil {
		return nil, err
	}
	for i, s := range slices {
		if s.data, err = signThin(s.data, o, der); err != nil {
			return nil, fmt.Errorf("%s slice: %w", archName(s.cpu), err)
		}
		// codesign lays out what it signs on 16 KB boundaries.
		slices[i].data, slices[i].align = s.data, 14
	}
	return writeFat(slices), nil
}

// SignFile signs the program at path in place.
func SignFile(path string, o Options) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !IsMachO(data) {
		return fmt.Errorf("%s is not a 64-bit Mach-O program", path)
	}
	signed, err := Sign(data, o)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return replaceFile(path, signed)
}

// replaceFile writes data over path through a file beside it, keeping
// its mode, so that a failed write leaves path as it was.
func replaceFile(path string, data []byte) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, fi.Mode().Perm())
	}
	if err == nil {
		err = shared.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// thin is what signing needs to know of a 64-bit Mach-O image.
type thin struct {
	cpu, filetype uint32
	ncmds, sizeof uint32
	// Offsets of the load commands signing changes, 0 when absent.
	linkedit, codeSig int
	textOff, textSize uint64
	uuid              []byte
	firstSection      uint64 // file offset of the first section's contents
}

func parseThin(data []byte) (*thin, error) {
	le := binary.LittleEndian
	if len(data) < headerSize || le.Uint32(data) != magic64 {
		return nil, errors.New("not a 64-bit little-endian Mach-O image")
	}
	t := &thin{
		cpu: le.Uint32(data[4:]), filetype: le.Uint32(data[12:]),
		ncmds: le.Uint32(data[16:]), sizeof: le.Uint32(data[20:]),
		firstSection: uint64(len(data)),
	}
	end := headerSize + int(t.sizeof)
	if end > len(data) {
		return nil, errors.New("load commands run past the end of the file")
	}
	off := headerSize
	for range t.ncmds {
		if off+8 > end {
			return nil, errors.New("load commands run past their size")
		}
		cmd, size := le.Uint32(data[off:]), int(le.Uint32(data[off+4:]))
		if size < 8 || off+size > end {
			return nil, fmt.Errorf("load command %#x has a bad size", cmd)
		}
		c := data[off : off+size]
		switch cmd {
		case lcSegment64:
			if size < 72 {
				return nil, errors.New("short segment command")
			}
			name := string(bytes.TrimRight(c[8:24], "\x00"))
			fileoff, filesize := le.Uint64(c[40:]), le.Uint64(c[48:])
			switch name {
			case "__LINKEDIT":
				t.linkedit = off
			case "__TEXT":
				t.textOff, t.textSize = fileoff, filesize
			}
			nsects := int(le.Uint32(c[64:]))
			if 72+80*nsects > size {
				return nil, fmt.Errorf("segment %s has more sections than fit", name)
			}
			for i := range nsects {
				s := c[72+80*i:]
				if o := uint64(le.Uint32(s[48:])); o != 0 && o < t.firstSection {
					t.firstSection = o
				}
			}
			if filesize > 0 && fileoff > 0 && fileoff < t.firstSection {
				t.firstSection = fileoff
			}
		case lcCodeSignature:
			if size < 16 {
				return nil, errors.New("short code signature command")
			}
			t.codeSig = off
		case lcUUID:
			if size >= 24 {
				t.uuid = c[8:24]
			}
		}
		off += size
	}
	if t.linkedit == 0 {
		return nil, errors.New("no __LINKEDIT segment, where the signature goes")
	}
	return t, nil
}

// pageShift is log2 of the size of the pages hashed: 16 KB on arm64,
// whose pages are that size, and 4 KB elsewhere, as codesign does.
func pageShift(cpu uint32) uint8 {
	if cpu == cpuARM64 {
		return 14
	}
	return 12
}

func archName(cpu uint32) string {
	switch cpu {
	case cpuARM64:
		return "arm64"
	case cpuX86_64:
		return "x86_64"
	}
	return fmt.Sprintf("cpu %#x", cpu)
}

func alignUp(n, a uint64) uint64 { return (n + a - 1) &^ (a - 1) }

// signThin signs one 64-bit image: it makes room for the signature at
// the end of __LINKEDIT, adding the load command for it when there is
// none, and appends the signature.
func signThin(data []byte, o Options, der []byte) ([]byte, error) {
	t, err := parseThin(data)
	if err != nil {
		return nil, err
	}
	le := binary.LittleEndian
	buf := bytes.Clone(data)
	seg := buf[t.linkedit:]
	segOff, segSize := le.Uint64(seg[40:]), le.Uint64(seg[48:])
	linkEnd := segOff + segSize

	var sigOff uint64
	if t.codeSig != 0 {
		sigOff = uint64(le.Uint32(buf[t.codeSig+8:]))
		if sigOff < segOff || sigOff > linkEnd || sigOff > uint64(len(buf)) {
			return nil, errors.New("the code signature is not at the end of __LINKEDIT")
		}
	} else {
		if linkEnd != uint64(len(buf)) {
			return nil, errors.New("__LINKEDIT does not end the file, so there is no place for a signature")
		}
		// The new load command goes where the commands end, which must
		// still be before the first section's contents.
		at := headerSize + int(t.sizeof)
		if uint64(at+16) > t.firstSection {
			return nil, errors.New("no room for a code signature load command; link with -headerpad")
		}
		cmd := buf[at : at+16]
		le.PutUint32(cmd, lcCodeSignature)
		le.PutUint32(cmd[4:], 16)
		le.PutUint32(buf[16:], t.ncmds+1)
		le.PutUint32(buf[20:], t.sizeof+16)
		t.codeSig = at
		sigOff = alignUp(linkEnd, 16)
	}
	// What is hashed stops where the signature starts.
	if uint64(len(buf)) > sigOff {
		buf = buf[:sigOff]
	} else {
		buf = append(buf, make([]byte, sigOff-uint64(len(buf)))...)
	}

	cd := codeDirectory{
		identifier: o.Identifier,
		pageShift:  pageShift(t.cpu),
		codeLimit:  sigOff,
		execBase:   t.textOff,
		execLimit:  t.textSize,
		execFlags:  execSegFlags(t.filetype, o.Entitlements),
	}
	cd.special[slotInfo] = o.InfoPlist
	cd.special[slotResources] = o.Resources
	sig := superBlob{cd: &cd, entitlements: o.Entitlements, der: der}
	size := uint64(sig.space())

	// The headers say how large the signature is before the pages that
	// hold them are hashed.
	le.PutUint32(buf[t.codeSig+8:], uint32(sigOff))
	le.PutUint32(buf[t.codeSig+12:], uint32(size))
	seg = buf[t.linkedit:]
	newSize := sigOff + size - segOff
	le.PutUint64(seg[48:], newSize)
	// codesign rounds the segment's size in memory to 16 KB, whatever the
	// architecture's page size.
	le.PutUint64(seg[32:], alignUp(newSize, 1<<14))

	blob := sig.encode(buf)
	out := append(buf, blob...)
	// The padding after the signature is not hashed; codesign leaves in
	// it what the file held there, and so does this, so that the two
	// sign alike byte for byte.
	pad := make([]byte, int(size)-len(blob))
	if len(out) < len(data) {
		copy(pad, data[len(out):])
	}
	return append(out, pad...), nil
}

// execSegFlags returns the code directory's executable segment flags,
// as codesign sets them: the main binary flag for a program, and what
// some entitlements imply.
func execSegFlags(filetype uint32, entitlements []byte) uint64 {
	var f uint64
	if filetype == mhExecute {
		f |= 0x1
	}
	if entitlements == nil {
		return f
	}
	ents, err := parseEntitlements(entitlements)
	if err != nil {
		return f
	}
	for key, flag := range map[string]uint64{
		"get-task-allow":                            0x10,
		"run-unsigned-code":                         0x10,
		"com.apple.private.cs.debugger":             0x20,
		"dynamic-codesigning":                       0x40,
		"com.apple.private.skip-library-validation": 0x80,
		"com.apple.private.amfi.can-load-cdhash":    0x100,
		"com.apple.private.amfi.can-execute-cdhash": 0x200,
	} {
		if v, _ := ents[key].(bool); v {
			f |= flag
		}
	}
	return f
}
