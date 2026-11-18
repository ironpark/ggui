package machosign

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
)

// slice is one architecture of a universal file.
type slice struct {
	cpu, subtype uint32
	align        uint32 // log2
	data         []byte
}

// fatSlices splits a universal file, whose header is big-endian, into
// its slices.
func fatSlices(data []byte) ([]slice, error) {
	be := binary.BigEndian
	n := int(be.Uint32(data[4:]))
	if 8+20*n > len(data) {
		return nil, errors.New("universal header runs past the end of the file")
	}
	out := make([]slice, n)
	for i := range n {
		h := data[8+20*i:]
		off, size := uint64(be.Uint32(h[8:])), uint64(be.Uint32(h[12:]))
		if off+size > uint64(len(data)) {
			return nil, fmt.Errorf("slice %d runs past the end of the file", i)
		}
		out[i] = slice{cpu: be.Uint32(h), subtype: be.Uint32(h[4:]), align: be.Uint32(h[16:]), data: data[off : off+size]}
	}
	return out, nil
}

// writeFat lays slices out as a universal file, in the order given, each
// at an offset aligned as it says.
func writeFat(slices []slice) []byte {
	be := binary.BigEndian
	end := uint64(8 + 20*len(slices))
	offs := make([]uint64, len(slices))
	for i, s := range slices {
		offs[i] = alignUp(end, 1<<s.align)
		end = offs[i] + uint64(len(s.data))
	}
	out := make([]byte, end)
	be.PutUint32(out, fatMagic)
	be.PutUint32(out[4:], uint32(len(slices)))
	for i, s := range slices {
		h := out[8+20*i:]
		be.PutUint32(h, s.cpu)
		be.PutUint32(h[4:], s.subtype)
		be.PutUint32(h[8:], uint32(offs[i]))
		be.PutUint32(h[12:], uint32(len(s.data)))
		be.PutUint32(h[16:], s.align)
		copy(out[offs[i]:], s.data)
	}
	return out
}

// Fat joins thin 64-bit images into a universal file, as lipo -create
// does: each slice aligned to its architecture's page size, 16 KB for
// arm64 and 4 KB for x86_64, and the slices sorted by alignment, the
// smallest first.
func Fat(thins ...[]byte) ([]byte, error) {
	if len(thins) == 0 {
		return nil, errors.New("nothing to join")
	}
	var out []slice
	for i, data := range thins {
		t, err := parseThin(data)
		if err != nil {
			return nil, fmt.Errorf("input %d: %w", i+1, err)
		}
		for _, s := range out {
			if s.cpu == t.cpu {
				return nil, fmt.Errorf("two inputs are for %s", archName(t.cpu))
			}
		}
		subtype := binary.LittleEndian.Uint32(data[8:])
		out = append(out, slice{cpu: t.cpu, subtype: subtype, align: uint32(pageShift(t.cpu)), data: data})
	}
	slices.SortStableFunc(out, func(a, b slice) int { return int(a.align) - int(b.align) })
	return writeFat(out), nil
}

// Lipo writes the universal file joining the thin programs inputs to
// out, with the mode of the first.
func Lipo(out string, inputs ...string) error {
	var thins [][]byte
	var mode os.FileMode = 0o755
	for i, in := range inputs {
		data, err := os.ReadFile(in)
		if err != nil {
			return err
		}
		if i == 0 {
			if fi, err := os.Stat(in); err == nil {
				mode = fi.Mode().Perm()
			}
		}
		thins = append(thins, data)
	}
	data, err := Fat(thins...)
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, data, mode); err != nil {
		return err
	}
	return os.Chmod(out, mode)
}
