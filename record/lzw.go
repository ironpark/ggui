package record

import "io"

// lzwEncoder compresses a frame's palette indices as a GIF holds them:
// LZW, codes least significant bit first, a clear code when the 4096
// codes run out, as compress/lzw writes. Unlike compress/lzw it is told,
// for each pixel, which indices would show what that pixel must show, and
// takes whichever continues the string it is matching:
//
//   - the index the pixel maps to;
//   - the frame's transparent index, where the pixel shows the same
//     index already, since a transparent pixel leaves the frame before;
//   - with lossy compression, any index whose color, or the one already
//     shown for the transparent index, is near enough the pixel's.
//
// The first two decode to exactly the same picture; it is how a frame
// whose changed rectangle holds much that did not change compresses to
// little. The third trades exactness for longer strings, as gifsicle's
// --lossy does.
type lzwEncoder struct {
	table [lzwTableSize]uint32
	w     io.ByteWriter
	bits  uint32
	nBits uint
	lit   uint
	width uint
	hi    uint32
	over  uint32
	err   error
}

const (
	lzwMaxCode   = 1<<12 - 1
	lzwTableSize = 4 * 1 << 12
	lzwTableMask = lzwTableSize - 1
	lzwInvalid   = 1<<32 - 1
)

// frameCodes is what a frame must show and what it may show it with.
type frameCodes struct {
	want   []byte // the index each pixel maps to
	under  []byte // the index each pixel shows now, or nil for a frame drawn whole
	clear  int    // the transparent index, or -1 for none
	lossy  *nearness
	chosen []byte // filled with the index written for each pixel
}

// nearness is which palette colors pass for which, for lossy
// compression.
type nearness struct {
	ok   [256][256]bool // ok[a][b]: a passes for b
	near [256][]byte    // the indices that pass for each, nearest first, itself left out
}

// encode writes fc's indices, with lit bits a literal, to w.
func (e *lzwEncoder) encode(w io.ByteWriter, lit int, fc frameCodes) error {
	e.w, e.bits, e.nBits, e.err = w, 0, 0, nil
	e.lit = uint(lit)
	e.reset()
	clearCode := uint32(1) << e.lit
	e.write(clearCode)
	n := len(fc.want)
	if n == 0 {
		e.write(clearCode + 1)
		return e.finish()
	}
	start := func(i int) byte {
		if fc.clear >= 0 && fc.under[i] == fc.want[i] {
			return byte(fc.clear)
		}
		return fc.want[i]
	}
	code := uint32(start(0))
	fc.chosen[0] = byte(code)
	for i := 1; i < n; i++ {
		p := fc.want[i]
		if c, ok := e.find(code, p); ok {
			code, fc.chosen[i] = c, p
			continue
		}
		if fc.clear >= 0 && fc.under[i] == p {
			if c, ok := e.find(code, byte(fc.clear)); ok {
				code, fc.chosen[i] = c, byte(fc.clear)
				continue
			}
		}
		if l := fc.lossy; l != nil {
			if fc.clear >= 0 && l.ok[fc.under[i]][p] {
				if c, ok := e.find(code, byte(fc.clear)); ok {
					code, fc.chosen[i] = c, byte(fc.clear)
					continue
				}
			}
			found := false
			for _, q := range l.near[p] {
				if c, ok := e.find(code, q); ok {
					code, fc.chosen[i], found = c, q, true
					break
				}
			}
			if found {
				continue
			}
		}
		// The string ends: write it, and start the next with this pixel.
		e.write(code)
		s := start(i)
		key := code<<8 | uint32(s)
		code = uint32(s)
		fc.chosen[i] = s
		if e.incHi() {
			e.insert(key)
		}
	}
	e.write(code)
	e.incHi()
	e.write(clearCode + 1)
	return e.finish()
}

func (e *lzwEncoder) reset() {
	e.width = e.lit + 1
	e.hi = 1<<e.lit + 1
	e.over = 1 << (e.lit + 1)
	for i := range e.table {
		e.table[i] = lzwInvalid
	}
}

// find returns the code of the string code followed by b, if there is
// one.
func (e *lzwEncoder) find(code uint32, b byte) (uint32, bool) {
	key := code<<8 | uint32(b)
	h := (key>>12 ^ key) & lzwTableMask
	for t := e.table[h]; t != lzwInvalid; t = e.table[h] {
		if key == t>>12 {
			return t & lzwMaxCode, true
		}
		h = (h + 1) & lzwTableMask
	}
	return 0, false
}

func (e *lzwEncoder) insert(key uint32) {
	h := (key>>12 ^ key) & lzwTableMask
	for e.table[h] != lzwInvalid {
		h = (h + 1) & lzwTableMask
	}
	e.table[h] = key<<12 | e.hi
}

// incHi counts the code the string just written made, and reports
// whether it is kept: when the codes run out, a clear code starts the
// table over instead.
func (e *lzwEncoder) incHi() bool {
	e.hi++
	if e.hi == e.over {
		e.width++
		e.over <<= 1
	}
	if e.hi == lzwMaxCode {
		e.write(1 << e.lit)
		e.reset()
		return false
	}
	return true
}

func (e *lzwEncoder) write(code uint32) {
	e.bits |= code << e.nBits
	e.nBits += e.width
	for e.nBits >= 8 {
		if err := e.w.WriteByte(byte(e.bits)); err != nil && e.err == nil {
			e.err = err
		}
		e.bits >>= 8
		e.nBits -= 8
	}
}

func (e *lzwEncoder) finish() error {
	if e.nBits > 0 {
		if err := e.w.WriteByte(byte(e.bits)); err != nil && e.err == nil {
			e.err = err
		}
	}
	return e.err
}
