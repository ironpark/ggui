package psinterpreter

import (
	"bytes"
	"testing"
)

// An INDEX reads each object where it lies, at offsets of any size the
// format allows, and an empty one holds nothing.
func TestIndexReadsObjectsAtTheirOffsets(t *testing.T) {
	objects := [][]byte{[]byte("ab"), {}, []byte("cde")}
	data := []byte("abcde")
	for offSize := 1; offSize <= 4; offSize++ {
		var offsets []byte
		for _, o := range []int{1, 3, 3, 6} { // one past each start, as stored
			for b := offSize - 1; b >= 0; b-- {
				offsets = append(offsets, byte(o>>(8*b)))
			}
		}
		x := NewIndex(offsets, data, offSize, len(objects))
		if x.Len() != len(objects) {
			t.Fatalf("offSize %d: %d objects, want %d", offSize, x.Len(), len(objects))
		}
		for i, want := range objects {
			if got := x.At(i); !bytes.Equal(got, want) || cap(got) != len(got) {
				t.Fatalf("offSize %d: object %d is %q (cap %d), want %q", offSize, i, got, cap(got), want)
			}
		}
	}
	if (Index{}).Len() != 0 || NewIndex(nil, nil, 2, 0).Len() != 0 {
		t.Fatal("an empty INDEX holds objects")
	}
}
