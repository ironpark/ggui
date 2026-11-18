package machosign

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"slices"
)

// archHash is the CDHash of one architecture of a signed program.
type archHash struct {
	cpu  uint32
	hash []byte
}

// cdHashes returns the CDHashes of a signed program's architectures: the
// first 20 bytes of the SHA-256 hash of each code directory, arm64 first.
func cdHashes(data []byte) ([]archHash, error) {
	thins := [][]byte{data}
	if isFat(data) {
		ss, err := fatSlices(data)
		if err != nil {
			return nil, err
		}
		thins = thins[:0]
		for _, s := range ss {
			thins = append(thins, s.data)
		}
	}
	var out []archHash
	for _, d := range thins {
		t, err := parseThin(d)
		if err != nil {
			return nil, err
		}
		cd, err := codeDirectoryOf(d, t)
		if err != nil {
			return nil, err
		}
		h := sha256.Sum256(cd)
		out = append(out, archHash{t.cpu, h[:20]})
	}
	slices.SortStableFunc(out, func(a, b archHash) int {
		return cmp.Compare(boolInt(a.cpu != cpuARM64), boolInt(b.cpu != cpuARM64))
	})
	return out, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// codeDirectoryOf returns the code directory blob of a signed image.
func codeDirectoryOf(data []byte, t *thin) ([]byte, error) {
	if t.codeSig == 0 {
		return nil, errors.New("not signed")
	}
	le, be := binary.LittleEndian, binary.BigEndian
	off, size := int(le.Uint32(data[t.codeSig+8:])), int(le.Uint32(data[t.codeSig+12:]))
	if off+size > len(data) || size < 12 {
		return nil, errors.New("the signature runs past the end of the file")
	}
	sig := data[off : off+size]
	if be.Uint32(sig) != magicSuperBlob {
		return nil, errors.New("the signature is not a superblob")
	}
	n := int(be.Uint32(sig[8:]))
	for i := range n {
		e := 12 + 8*i
		if e+8 > len(sig) {
			break
		}
		if be.Uint32(sig[e:]) != slotCodeDirectory {
			continue
		}
		o := int(be.Uint32(sig[e+4:]))
		if o+8 > len(sig) || be.Uint32(sig[o:]) != magicCodeDirectory {
			break
		}
		l := int(be.Uint32(sig[o+4:]))
		if o+l > len(sig) {
			break
		}
		return sig[o : o+l], nil
	}
	return nil, errors.New("the signature has no code directory")
}
