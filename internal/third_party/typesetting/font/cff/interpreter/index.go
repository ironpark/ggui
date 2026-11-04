// SPDX-License-Identifier: Unlicense OR BSD-3-Clause

package psinterpreter

// Index is a CFF INDEX: its objects, one after another, read where the
// font is. Only the offsets of an object asked for are decoded: a font's
// charstrings are one per glyph, tens of thousands in a CJK font, and kept
// as slices of their own they took 24 bytes each for as long as the font
// was loaded.
type Index struct {
	offsets []byte // count+1 offsets of offSize bytes, each one past its object's start in data
	data    []byte
	offSize int
}

// NewIndex is the INDEX of count objects in data, at the count+1 offsets
// of offSize bytes in offsets, as the INDEX stores them. The caller has
// checked them: they are nonzero, never decrease and stay within data.
func NewIndex(offsets, data []byte, offSize, count int) Index {
	if count <= 0 {
		return Index{}
	}
	return Index{offsets: offsets[:(count+1)*offSize], data: data, offSize: offSize}
}

// Len is the number of objects.
func (x Index) Len() int {
	if x.offSize == 0 {
		return 0
	}
	return len(x.offsets)/x.offSize - 1
}

// At is object i, which must be below Len.
func (x Index) At(i int) []byte {
	start, end := x.offset(i), x.offset(i+1)
	return x.data[start:end:end]
}

func (x Index) offset(i int) int {
	v := 0
	for _, b := range x.offsets[i*x.offSize : (i+1)*x.offSize] {
		v = v<<8 | int(b)
	}
	return v - 1
}
