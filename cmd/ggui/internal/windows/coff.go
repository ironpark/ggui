package windows

import (
	"bytes"
	"cmp"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"slices"
)

// Resource types, the RT_ constants of winuser.h.
const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtVersion   = 16
	rtManifest  = 24
)

// langENUS is the language the resources are filed under, English (United
// States), as rc.exe files them by default; the version resource's
// translation names it too. Windows finds a program's icon, version and
// manifest whatever their language.
const langENUS = 0x0409

// resource is one resource: its type, its ID, its language and its bytes.
type resource struct {
	typ, id, lang uint16
	data          []byte
}

// rsrc lays resources out as a .rsrc section: the three-level directory
// of types, IDs and languages, the data entries, and the data. It returns
// the section and the offsets in it of the data entries' OffsetToData
// fields, which hold the data's offset in the section and must hold its
// address in the program: the linker adds the section's address to each,
// told so by a relocation.
//
// Every resource here has a numeric ID, and a directory's entries are
// sorted by it, as Windows' binary search through them expects.
func rsrc(res []resource) (sect []byte, relocs []uint32) {
	res = slices.Clone(res)
	slices.SortFunc(res, func(a, b resource) int {
		return cmp.Or(cmp.Compare(a.typ, b.typ), cmp.Compare(a.id, b.id), cmp.Compare(a.lang, b.lang))
	})

	// The tree: types, IDs within a type, languages within an ID, each
	// language a leaf naming its resource.
	type node struct {
		key  uint16
		kids []*node
		leaf int // the resource's index, for a leaf; -1 for a directory
	}
	child := func(n *node, key uint16) *node {
		if k := len(n.kids); k > 0 && n.kids[k-1].key == key {
			return n.kids[k-1] // sorted, so a key's resources are together
		}
		c := &node{key: key, leaf: -1}
		n.kids = append(n.kids, c)
		return c
	}
	root := &node{leaf: -1}
	for i, r := range res {
		n := child(child(root, r.typ), r.id)
		n.kids = append(n.kids, &node{key: r.lang, leaf: i})
	}

	// Offsets: the directories breadth first, as rc.exe writes them, then
	// the data entries, then the data, each 8-byte aligned.
	const dirSize, entrySize, dataEntrySize = 16, 8, 16
	var dirs []*node
	dirOff := map[*node]uint32{}
	off := uint32(0)
	for level := []*node{root}; len(level) > 0; {
		var next []*node
		for _, n := range level {
			dirs = append(dirs, n)
			dirOff[n] = off
			off += dirSize + entrySize*uint32(len(n.kids))
			for _, k := range n.kids {
				if k.leaf < 0 {
					next = append(next, k)
				}
			}
		}
		level = next
	}
	dataEntryOff := make([]uint32, len(res))
	for i := range res {
		dataEntryOff[i] = off
		off += dataEntrySize
	}
	dataOff := make([]uint32, len(res))
	for i, r := range res {
		off = align(off, 8)
		dataOff[i] = off
		off += uint32(len(r.data))
	}
	sect = make([]byte, align(off, 8))

	le := binary.LittleEndian
	for _, n := range dirs {
		at := dirOff[n]
		// Characteristics, TimeDateStamp and the version stay 0, and so
		// does NumberOfNamedEntries.
		le.PutUint16(sect[at+14:], uint16(len(n.kids)))
		for i, k := range n.kids {
			e := at + dirSize + entrySize*uint32(i)
			le.PutUint32(sect[e:], uint32(k.key))
			if k.leaf >= 0 {
				le.PutUint32(sect[e+4:], dataEntryOff[k.leaf])
			} else {
				le.PutUint32(sect[e+4:], 0x80000000|dirOff[k]) // a subdirectory
			}
		}
	}
	for i, r := range res {
		e := dataEntryOff[i]
		le.PutUint32(sect[e:], dataOff[i]) // OffsetToData, relocated
		le.PutUint32(sect[e+4:], uint32(len(r.data)))
		// CodePage and Reserved stay 0.
		copy(sect[dataOff[i]:], r.data)
		relocs = append(relocs, e)
	}
	return sect, relocs
}

func align(n, to uint32) uint32 { return (n + to - 1) / to * to }

// machine is the COFF machine of each GOARCH a Windows program is built
// for, with the relocation that adds an image's base-relative address of
// a section, ADDR32NB, which is what a resource's data entry holds.
var machine = map[string]struct{ machine, reloc uint16 }{
	"amd64": {pe.IMAGE_FILE_MACHINE_AMD64, 0x0003}, // IMAGE_REL_AMD64_ADDR32NB
	"arm64": {pe.IMAGE_FILE_MACHINE_ARM64, 0x0002}, // IMAGE_REL_ARM64_ADDR32NB
	"386":   {pe.IMAGE_FILE_MACHINE_I386, 0x0007},  // IMAGE_REL_I386_DIR32NB
}

// Arches lists the GOARCH values a resource object can be written for.
var Arches = []string{"amd64", "arm64", "386"}

// coff wraps a .rsrc section in a COFF object file for arch, which the go
// command links into a program when it finds it as a .syso file in the
// main package: what windres -O coff and cvtres write. The object has the
// one section, its relocations, and one symbol, the section's own, which
// the relocations are against.
func coff(arch string, sect []byte, relocs []uint32) ([]byte, error) {
	m, ok := machine[arch]
	if !ok {
		return nil, fmt.Errorf("no Windows resources for GOARCH=%s: build for amd64, arm64 or 386", arch)
	}
	if len(relocs) > 0xffff {
		return nil, fmt.Errorf("%d resources are more than a COFF section relocates", len(relocs))
	}
	const fileHeader, sectionHeader, relocSize, symbolSize = 20, 40, 10, 18
	dataAt := uint32(fileHeader + sectionHeader)
	relocAt := dataAt + uint32(len(sect))
	symAt := relocAt + relocSize*uint32(len(relocs))

	var b bytes.Buffer
	le := binary.LittleEndian
	w := func(v any) { binary.Write(&b, le, v) }
	w(pe.FileHeader{
		Machine:              m.machine,
		NumberOfSections:     1,
		PointerToSymbolTable: symAt,
		NumberOfSymbols:      2, // the section's symbol and its auxiliary record
	})
	sh := pe.SectionHeader32{
		SizeOfRawData:        uint32(len(sect)),
		PointerToRawData:     dataAt,
		PointerToRelocations: relocAt,
		NumberOfRelocations:  uint16(len(relocs)),
		// Read-only initialized data, as the linker takes .rdata; it
		// gathers a .rsrc section into the program's own.
		Characteristics: pe.IMAGE_SCN_CNT_INITIALIZED_DATA | pe.IMAGE_SCN_MEM_READ,
	}
	copy(sh.Name[:], ".rsrc")
	w(sh)
	b.Write(sect)
	for _, r := range relocs {
		w(pe.Reloc{VirtualAddress: r, SymbolTableIndex: 0, Type: m.reloc})
	}
	sym := pe.COFFSymbol{
		SectionNumber:      1,
		StorageClass:       3, // IMAGE_SYM_CLASS_STATIC: a section symbol
		NumberOfAuxSymbols: 1,
	}
	copy(sym.Name[:], ".rsrc")
	w(sym)
	// The section definition the auxiliary record holds: its length and
	// relocation count, padded to a symbol's size.
	aux := make([]byte, symbolSize)
	le.PutUint32(aux[0:], uint32(len(sect)))
	le.PutUint16(aux[4:], uint16(len(relocs)))
	b.Write(aux)
	w(uint32(4)) // the string table: only its own size
	return b.Bytes(), nil
}
