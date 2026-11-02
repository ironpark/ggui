// SPDX-License-Identifier: Unlicense OR BSD-3-Clause

package font

import (
	"sync"
	"unsafe"

	ot "github.com/ironpark/ggui/internal/typesetting/font/opentype"
	"github.com/ironpark/ggui/internal/typesetting/font/opentype/tables"
)

type Morx []MorxChain

// sharedMorx are the 'morx' tables read from permanent bytes, by where
// they lie: the faces of a collection, as the weights of Apple SD Gothic
// Neo, have one table, which each face read anew. A table, once read, is
// only read from, and its bytes neither move nor change, so the faces now
// share what the first read.
var sharedMorx sync.Map // morxKey to Morx

type morxKey struct {
	data            uintptr // the table's first byte, outside Go's heap
	length, nGlyphs int
}

// loadMorx reads the font's 'morx' table, of a font with nGlyphs glyphs, or
// shares the one read from the same permanent bytes.
func loadMorx(ld *ot.Loader, nGlyphs int) Morx {
	raw, permanent, err := ld.PermanentTable(ot.MustNewTag("morx"))
	if err != nil {
		return nil
	}
	read := func() Morx {
		morx, _, _ := tables.ParseMorx(raw, nGlyphs)
		return newMorx(morx)
	}
	if !permanent || len(raw) == 0 {
		return read()
	}
	key := morxKey{uintptr(unsafe.Pointer(unsafe.SliceData(raw))), len(raw), nGlyphs}
	if m, ok := sharedMorx.Load(key); ok {
		return m.(Morx)
	}
	m, _ := sharedMorx.LoadOrStore(key, read())
	return m.(Morx)
}

func newMorx(table tables.Morx) Morx {
	if len(table.Chains) == 0 {
		return nil
	}
	out := make(Morx, len(table.Chains))
	for i, c := range table.Chains {
		out[i] = newMorxChain(c)
	}
	return out
}

type MorxChain struct {
	Features     []tables.AATFeature
	Subtables    []MorxSubtable
	DefaultFlags uint32
}

func newMorxChain(table tables.MorxChain) (out MorxChain) {
	out.DefaultFlags = table.Flags
	out.Features = table.Features
	out.Subtables = make([]MorxSubtable, len(table.Subtables))
	for i, s := range table.Subtables {
		out.Subtables[i] = newMorxSubtable(s)
	}
	return out
}

type MorxSubtable struct {
	Data     interface{ isMorxSubtable() }
	Coverage uint8  // high byte of the coverage flag
	Flags    uint32 // Mask identifying which subtable this is.
}

func (MorxRearrangementSubtable) isMorxSubtable() {}
func (MorxContextualSubtable) isMorxSubtable()    {}
func (MorxLigatureSubtable) isMorxSubtable()      {}
func (MorxNonContextualSubtable) isMorxSubtable() {}
func (MorxInsertionSubtable) isMorxSubtable()     {}

func newMorxSubtable(table tables.MorxChainSubtable) (out MorxSubtable) {
	out.Coverage = table.Coverage
	out.Flags = table.SubFeatureFlags
	switch data := table.Data.(type) {
	case tables.MorxSubtableRearrangement:
		out.Data = MorxRearrangementSubtable(newAATStableTable(data.AATStateTableExt))
	case tables.MorxSubtableContextual:
		out.Data = MorxContextualSubtable{
			Machine:       newAATStableTable(data.AATStateTableExt),
			Substitutions: data.Substitutions.Substitutions,
		}
	case tables.MorxSubtableLigature:
		out.Data = MorxLigatureSubtable{
			Machine:        newAATStableTable(data.AATStateTableExt),
			LigatureAction: data.LigActions,
			Components:     data.Components,
			Ligatures:      data.Ligatures,
		}
	case tables.MorxSubtableNonContextual:
		out.Data = MorxNonContextualSubtable{Class: data.Class}
	case tables.MorxSubtableInsertion:
		s := MorxInsertionSubtable{
			Machine:    newAATStableTable(data.AATStateTableExt),
			Insertions: make([]GID, len(data.Insertions)),
		}
		for i, g := range data.Insertions {
			s.Insertions[i] = GID(g)
		}
		out.Data = s
	}
	return out
}

type MorxRearrangementSubtable AATStateTable

type MorxContextualSubtable struct {
	Substitutions []tables.AATLookup
	Machine       AATStateTable
}

// MorxLigatureSubtable reads its lists in place, in the font's data.
type MorxLigatureSubtable struct {
	LigatureAction tables.BEUint32s
	Components     tables.BEUint16s
	Ligatures      tables.BEUint16s // glyph IDs
	Machine        AATStateTable
}

type MorxNonContextualSubtable struct {
	Class tables.AATLookup // the lookup value is interpreted as a GlyphIndex
}

type MorxInsertionSubtable struct {
	// After successul parsing, this array may be safely
	// indexed by the indexes and counts from Machine entries.
	Insertions []GID
	Machine    AATStateTable
}
