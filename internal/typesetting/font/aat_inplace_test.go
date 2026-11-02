//go:build darwin

package font

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"
	"testing"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
)

// aatFonts are the system fonts with 'morx' tables ggui falls back on, and
// one with an old 'kern' state table.
var aatFonts = []string{
	"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	"/System/Library/Fonts/Menlo.ttc",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
	"/System/Library/Fonts/Supplemental/AppleGothic.ttf",
	"/System/Library/Fonts/Helvetica.ttc",
	"/System/Library/Fonts/GeezaPro.ttc",
}

// mappedFile is a font file mapped into memory, lending its bytes, as the
// files ggui reads its system fonts from are.
type mappedFile struct {
	*bytes.Reader
	b []byte
}

func (m mappedFile) Slice(off, n int64) ([]byte, bool) {
	if off < 0 || n < 0 || off+n > int64(len(m.b)) {
		return nil, false
	}
	return m.b[off : off+n : off+n], true
}

// Permanent implements opentype.PermanentSlicer: the file is never unmapped.
func (mappedFile) Permanent() bool { return true }

// mapLoaders maps the font file at path, never unmapped, and reads its
// faces' table directories.
func mapLoaders(t testing.TB, path string) []*ot.Loader {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	b, err := syscall.Mmap(int(f.Fd()), 0, int(st.Size()), syscall.PROT_READ, syscall.MAP_SHARED)
	if err != nil {
		t.Fatal(err)
	}
	lds, err := ot.NewLoaders(mappedFile{bytes.NewReader(b), b})
	if err != nil {
		t.Fatal(err)
	}
	return lds
}

// refEntry is a state table entry as the spec reads it.
type refEntry struct {
	newState, flags uint16
	data            [4]byte
}

// refStates is a state table decoded straight from its bytes.
type refStates struct {
	nClass, nStates int
	cells           []int // nStates rows of nClass entry indexes
	entries         []refEntry
}

// refStateTable decodes the state array and entries of an extended state
// table, at the start of b, with entries of 4+dataSize bytes.
func refStateTable(t *testing.T, b []byte, dataSize int) refStates {
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(b[o:])) }
	nClass, stateArray, entryTable := u32(0), u32(8), u32(12)
	r := refStates{nClass: nClass, nStates: (entryTable - stateArray) / 2 / nClass}
	maxIndex := 0
	for i := 0; i < r.nStates*nClass; i++ {
		e := int(binary.BigEndian.Uint16(b[stateArray+2*i:]))
		r.cells = append(r.cells, e)
		maxIndex = max(maxIndex, e)
	}
	size := 4 + dataSize
	for i := 0; i <= maxIndex; i++ {
		o := entryTable + i*size
		e := refEntry{newState: binary.BigEndian.Uint16(b[o:]), flags: binary.BigEndian.Uint16(b[o+2:])}
		copy(e.data[:], b[o+4:o+size])
		r.entries = append(r.entries, e)
	}
	if r.nStates == 0 {
		t.Fatal("no states")
	}
	return r
}

// checkStates compares every (state, class) cell of st with r, and the
// cells out of range.
func checkStates(t *testing.T, name string, st *AATStateTable, r refStates, data bool) int {
	t.Helper()
	bad := 0
	for s := 0; s < r.nStates; s++ {
		for c := 0; c < r.nClass; c++ {
			want := r.entries[r.cells[s*r.nClass+c]]
			got := st.GetEntry(uint16(s), uint16(c))
			mark, current := got.AsMorxContextual()
			var gd [4]byte
			binary.BigEndian.PutUint16(gd[:], mark)
			binary.BigEndian.PutUint16(gd[2:], current)
			if got.NewState != want.newState || got.Flags != want.flags || (data && gd != want.data) {
				if bad++; bad < 5 {
					t.Errorf("%s: state %d class %d: got %d %#x %v, want %d %#x %v", name, s, c, got.NewState, got.Flags, gd, want.newState, want.flags, want.data)
				}
			}
		}
	}
	// A class out of range reads class 1, a state out of range nothing.
	for s := 0; s < r.nStates; s++ {
		if got, want := st.GetEntry(uint16(s), uint16(r.nClass)), st.GetEntry(uint16(s), 1); got != want {
			t.Errorf("%s: state %d: class %d reads %v, not class 1's %v", name, s, r.nClass, got, want)
		}
	}
	if got := st.GetEntry(uint16(r.nStates), 0); got != (tables.AATStateEntry{}) {
		t.Errorf("%s: state %d past the last reads %v", name, r.nStates, got)
	}
	return r.nStates * r.nClass
}

// ligatureLists are a ligature subtable's actions, components and
// ligatures, as its accessors give them.
func ligatureLists(s MorxLigatureSubtable) (actions []uint32, components []uint16, ligatures []GID) {
	for i := 0; i < s.LigatureAction.Len(); i++ {
		actions = append(actions, s.LigatureAction.At(i))
	}
	for i := 0; i < s.Components.Len(); i++ {
		components = append(components, s.Components.At(i))
	}
	for i := 0; i < s.Ligatures.Len(); i++ {
		ligatures = append(ligatures, GID(s.Ligatures.At(i)))
	}
	return actions, components, ligatures
}

// TestMorxMatchesItsBytes checks the 'morx' tables of system fonts, as the
// font reads them, against their bytes decoded as the spec reads them:
// every subtable's every (state, class) cell, and every ligature action,
// component and ligature.
func TestMorxMatchesItsBytes(t *testing.T) {
	tagMorx := ot.MustNewTag("morx")
	for _, path := range aatFonts {
		cells, ligs := 0, 0
		for fi, ld := range mapLoaders(t, path) {
			raw, err := ld.RawTable(tagMorx)
			if err != nil {
				continue
			}
			f, err := NewFont(ld)
			if err != nil {
				t.Fatal(err)
			}
			u32 := func(o int) int { return int(binary.BigEndian.Uint32(raw[o:])) }
			nChains := u32(4)
			if len(f.Morx) != nChains {
				t.Fatalf("%s %d: %d chains, want %d", path, fi, len(f.Morx), nChains)
			}
			chain := 8
			for ci := 0; ci < nChains; ci++ {
				chainLength, nFeatures, nSubtables := u32(chain+4), u32(chain+8), u32(chain+12)
				if len(f.Morx[ci].Subtables) != nSubtables {
					t.Fatalf("%s %d: chain %d has %d subtables, want %d", path, fi, ci, len(f.Morx[ci].Subtables), nSubtables)
				}
				sub := chain + 16 + 12*nFeatures
				for si := 0; si < nSubtables; si++ {
					length := u32(sub)
					kind := raw[sub+7]
					body := raw[sub+12 : sub+length]
					data := f.Morx[ci].Subtables[si].Data
					var machine *AATStateTable
					dataSize := 0
					switch d := data.(type) {
					case MorxRearrangementSubtable:
						m := AATStateTable(d)
						machine = &m
					case MorxContextualSubtable:
						machine, dataSize = &d.Machine, 4
					case MorxInsertionSubtable:
						machine, dataSize = &d.Machine, 4
					case MorxLigatureSubtable:
						machine, dataSize = &d.Machine, 2
					case MorxNonContextualSubtable:
					default:
						t.Fatalf("%s %d: subtable %d.%d of type %d read as %T", path, fi, ci, si, kind, data)
					}
					if machine == nil {
						sub += length
						continue
					}
					r := refStateTable(t, body, dataSize)
					cells += checkStates(t, path, machine, r, true)
					if lig, ok := data.(MorxLigatureSubtable); ok {
						ligs += checkLigatures(t, path, lig, body, r)
					}
					sub += length
				}
				chain += chainLength
			}
		}
		t.Logf("%s: %d state cells, %d ligature list values", path, cells, ligs)
	}
}

// checkLigatures compares a ligature subtable's lists with its bytes.
func checkLigatures(t *testing.T, path string, lig MorxLigatureSubtable, body []byte, r refStates) int {
	t.Helper()
	u32 := func(o int) int { return int(binary.BigEndian.Uint32(body[o:])) }
	ligActionOffset, componentOffset, ligatureOffset := u32(16), u32(20), u32(24)

	// The actions run from the last an entry starts at to the first that
	// ends a list, or to the subtable's end.
	maxIndex := -1
	for _, e := range r.entries {
		if e.flags&tables.MLPerformAction != 0 {
			maxIndex = max(maxIndex, int(binary.BigEndian.Uint16(e.data[:])))
		}
	}
	var wantActions []uint32
	for o := ligActionOffset; o+4 <= len(body); o += 4 {
		a := binary.BigEndian.Uint32(body[o:])
		wantActions = append(wantActions, a)
		if len(wantActions) > maxIndex && a&tables.MLActionLast != 0 {
			break
		}
	}
	var wantComponents []uint16
	for o := componentOffset; o < ligatureOffset; o += 2 {
		wantComponents = append(wantComponents, binary.BigEndian.Uint16(body[o:]))
	}
	var wantLigatures []GID
	for o := ligatureOffset; o+2 <= len(body); o += 2 {
		wantLigatures = append(wantLigatures, GID(binary.BigEndian.Uint16(body[o:])))
	}

	actions, components, ligatures := ligatureLists(lig)
	compare(t, path+" actions", actions, wantActions)
	compare(t, path+" components", components, wantComponents)
	compare(t, path+" ligatures", ligatures, wantLigatures)
	return len(wantActions) + len(wantComponents) + len(wantLigatures)
}

func compare[T comparable](t *testing.T, name string, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d values, want %d", name, len(got), len(want))
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: value %d is %v, want %v", name, i, got[i], want[i])
			return
		}
	}
}

// TestKern1MatchesItsBytes checks the state tables of the old 'kern' table
// format 1 against their bytes, as TestMorxMatchesItsBytes does 'morx'.
func TestKern1MatchesItsBytes(t *testing.T) {
	tagKern := ot.MustNewTag("kern")
	checked := 0
	for _, path := range aatFonts {
		for fi, ld := range mapLoaders(t, path) {
			raw, err := ld.RawTable(tagKern)
			if err != nil || len(raw) < 8 || binary.BigEndian.Uint32(raw) != 0x00010000 {
				continue
			}
			f, err := NewFont(ld)
			if err != nil {
				t.Fatal(err)
			}
			n := int(binary.BigEndian.Uint32(raw[4:]))
			if len(f.Kern) != n {
				t.Fatalf("%s %d: %d kern subtables, want %d", path, fi, len(f.Kern), n)
			}
			sub := 8
			for i := 0; i < n; i++ {
				length := int(binary.BigEndian.Uint32(raw[sub:]))
				format := raw[sub+5]
				if format == 1 {
					b := raw[sub+8 : sub+length]
					u16 := func(o int) int { return int(binary.BigEndian.Uint16(b[o:])) }
					stateSize, stateArray, entryTable := u16(0), u16(4), u16(6)
					r := refStates{nClass: stateSize, nStates: (entryTable - stateArray) / stateSize}
					maxIndex := 0
					for j := 0; j < r.nStates*stateSize; j++ {
						e := int(b[stateArray+j])
						r.cells = append(r.cells, e)
						maxIndex = max(maxIndex, e)
					}
					for j := 0; j <= maxIndex; j++ {
						o := entryTable + 4*j
						// The new state is an offset, which the font reads
						// as the state's index.
						r.entries = append(r.entries, refEntry{
							newState: uint16((u16(o) - stateArray) / stateSize),
							flags:    uint16(u16(o + 2)),
						})
					}
					k, ok := f.Kern[i].Data.(Kern1)
					if !ok {
						t.Fatalf("%s %d: kern subtable %d read as %T", path, fi, i, f.Kern[i].Data)
					}
					checked += checkStates(t, path, &k.Machine, r, false)
				}
				sub += length
			}
		}
	}
	if checked == 0 {
		t.Skip("no font with a 'kern' state table")
	}
	t.Logf("%d kern state cells", checked)
}

// sharedMorxCollections are collections whose first two faces share one
// 'morx' table, tried in order. macOS 26 sets
// Apple SD Gothic Neo with 'GSUB' and no 'morx' table.
var sharedMorxCollections = []string{
	"/System/Library/Fonts/AppleSDGothicNeo.ttc",
	"/System/Library/Fonts/Apple Color Emoji.ttc",
}

// TestMorxSharedByACollectionsFaces checks that the faces of a collection
// mapped into memory share their common 'morx' table, and that faces of
// bytes on the heap, which may move or go, read their own.
func TestMorxSharedByACollectionsFaces(t *testing.T) {
	var (
		path string
		lds  []*ot.Loader
	)
	morx := ot.MustNewTag("morx")
	for _, p := range sharedMorxCollections {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if l := mapLoaders(t, p); len(l) >= 2 && l[0].HasTable(morx) && l[1].HasTable(morx) {
			path, lds = p, l
			break
		}
	}
	if lds == nil {
		t.Skip("no collection whose first two faces have a 'morx' table")
	}
	first := func(m Morx) *MorxChain {
		if len(m) == 0 {
			t.Fatal("no morx table")
		}
		return &m[0]
	}
	a, err := NewFont(lds[0])
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewFont(lds[1])
	if err != nil {
		t.Fatal(err)
	}
	if first(a.Morx) != first(b.Morx) {
		t.Error("the mapped faces read their common table each")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	heap, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewFont(heap[0])
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewFont(heap[1])
	if err != nil {
		t.Fatal(err)
	}
	if first(c.Morx) == first(d.Morx) || first(c.Morx) == first(a.Morx) {
		t.Error("faces of bytes on the heap share a table")
	}
}
