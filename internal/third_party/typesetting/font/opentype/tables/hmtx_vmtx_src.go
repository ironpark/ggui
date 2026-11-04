// SPDX-License-Identifier: Unlicense OR BSD-3-Clause

package tables

import "encoding/binary"

// https://learn.microsoft.com/en-us/typography/opentype/spec/hmtx
//
// The table is read where it lies in the font's data, which is mapped from
// its file, rather than copied out glyph by glyph: a font of 60,000 glyphs
// keeps 240 KB of metrics a face asks of a few.
type Hmtx struct {
	// metrics are the long metrics, 4 bytes each: an advance and a side
	// bearing. The advances are padded with the last value
	metrics []byte
	// and side bearings, 2 bytes each, are given.
	bearings []byte
}

func (table Hmtx) IsEmpty() bool {
	return len(table.metrics)+len(table.bearings) == 0
}

// Len is the number of long metrics, each with its own advance.
func (table Hmtx) Len() int { return len(table.metrics) / 4 }

// Metric is the i-th long metric, for i below Len.
func (table Hmtx) Metric(i int) LongHorMetric {
	m := table.metrics[i*4 : i*4+4]
	return LongHorMetric{AdvanceWidth: int16(binary.BigEndian.Uint16(m)), LeftSideBearing: int16(binary.BigEndian.Uint16(m[2:]))}
}

// Advance returns the base side bearing, defaulting to 0 for invalid glyph index
func (table Hmtx) Advance(gid GlyphID) int16 {
	LM, LS := table.Len(), len(table.bearings)/2
	index := int(gid)
	if index < LM {
		return table.Metric(index).AdvanceWidth
	} else if index < LS+LM { // return the last value
		return table.Metric(LM - 1).AdvanceWidth
	}
	return 0
}

// SideBearing returns the base side bearing, defaulting to 0 for invalid glyph index
func (table Hmtx) SideBearing(gid GlyphID) int16 {
	LM, LS := table.Len(), len(table.bearings)/2
	index := int(gid)
	if index < LM {
		return table.Metric(index).LeftSideBearing
	} else if index < LS+LM {
		return int16(binary.BigEndian.Uint16(table.bearings[(index-LM)*2:]))
	} else {
		return 0
	}
}

type LongHorMetric struct {
	AdvanceWidth, LeftSideBearing int16
}

// https://learn.microsoft.com/en-us/typography/opentype/spec/vmtx
type Vmtx = Hmtx
