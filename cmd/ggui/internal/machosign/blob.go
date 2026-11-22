package machosign

import (
	"crypto/sha256"
	"encoding/binary"
)

// Magic numbers of the blobs in a signature.
const (
	magicSuperBlob     = 0xfade0cc0
	magicCodeDirectory = 0xfade0c02
	magicRequirements  = 0xfade0c01
	magicEntitlements  = 0xfade7171
	magicDER           = 0xfade7172
	magicCMS           = 0xfade0b01
)

// Slots of the superblob's index, and of the code directory's special
// hashes, which are the hashes of what the other slots hold.
const (
	slotCodeDirectory = 0
	slotInfo          = 1
	slotRequirements  = 2
	slotResources     = 3
	slotEntitlements  = 5
	slotDER           = 7
	slotCMS           = 0x10000
)

const (
	cdVersion    = 0x20400 // with the executable segment fields
	cdHeaderSize = 88
	cdFlagAdHoc  = 0x2
	hashSHA256   = 2
	hashSize     = sha256.Size
)

// emptyRequirements is a requirement set with no requirements, which is
// what codesign writes for ad hoc signatures.
var emptyRequirements = []byte{0xfa, 0xde, 0x0c, 0x01, 0, 0, 0, 12, 0, 0, 0, 0}

// codeDirectory is the part of a signature that is hashed into the
// CDHash: what is signed and the hashes of its pages and special slots.
type codeDirectory struct {
	identifier string
	pageShift  uint8
	codeLimit  uint64
	execBase   uint64
	execLimit  uint64
	execFlags  uint64
	// special holds what the special slots hash, indexed by slot; the
	// requirements and entitlements slots are filled in by superBlob.
	special [slotDER + 1][]byte
}

// nSpecial is the number of special slots: up to the highest one used.
func (cd *codeDirectory) nSpecial() int {
	for n := len(cd.special) - 1; n > 0; n-- {
		if cd.special[n] != nil {
			return n
		}
	}
	return 0
}

func (cd *codeDirectory) nCode() int {
	page := uint64(1) << cd.pageShift
	return int((cd.codeLimit + page - 1) / page)
}

func (cd *codeDirectory) size() int {
	return cdHeaderSize + len(cd.identifier) + 1 + (cd.nSpecial()+cd.nCode())*hashSize
}

// encode returns the code directory, hashing code, the image up to the
// signature.
func (cd *codeDirectory) encode(code []byte) []byte {
	be := binary.BigEndian
	ns, nc := cd.nSpecial(), cd.nCode()
	b := make([]byte, cd.size())
	identOff := cdHeaderSize
	hashOff := identOff + len(cd.identifier) + 1 + ns*hashSize
	be.PutUint32(b[0:], magicCodeDirectory)
	be.PutUint32(b[4:], uint32(len(b)))
	be.PutUint32(b[8:], cdVersion)
	be.PutUint32(b[12:], cdFlagAdHoc)
	be.PutUint32(b[16:], uint32(hashOff))
	be.PutUint32(b[20:], uint32(identOff))
	be.PutUint32(b[24:], uint32(ns))
	be.PutUint32(b[28:], uint32(nc))
	be.PutUint32(b[32:], uint32(cd.codeLimit))
	b[36] = hashSize
	b[37] = hashSHA256
	b[38] = 0 // platform
	b[39] = cd.pageShift
	// spare2, scatterOffset, teamOffset, spare3 and codeLimit64 stay 0.
	be.PutUint64(b[64:], cd.execBase)
	be.PutUint64(b[72:], cd.execLimit)
	be.PutUint64(b[80:], cd.execFlags)
	copy(b[identOff:], cd.identifier)
	for slot := 1; slot <= ns; slot++ {
		if v := cd.special[slot]; v != nil {
			h := sha256.Sum256(v)
			copy(b[hashOff-slot*hashSize:], h[:])
		}
	}
	page := 1 << cd.pageShift
	for i := range nc {
		end := min((i+1)*page, len(code))
		h := sha256.Sum256(code[i*page : end])
		copy(b[hashOff+i*hashSize:], h[:])
	}
	return b
}

// superBlob is a whole embedded signature.
type superBlob struct {
	cd                *codeDirectory
	entitlements, der []byte // nil without entitlements
}

// blobs returns the slots and contents of the blobs after the code
// directory, in the order codesign writes them, and puts them in the
// code directory's special slots, which hash them.
func (s *superBlob) blobs() (slots []uint32, blobs [][]byte) {
	slots = []uint32{slotRequirements}
	blobs = [][]byte{emptyRequirements}
	if s.entitlements != nil {
		slots = append(slots, slotEntitlements, slotDER)
		blobs = append(blobs, wrap(magicEntitlements, s.entitlements), wrap(magicDER, s.der))
	}
	for i, slot := range slots {
		s.cd.special[slot] = blobs[i]
	}
	return append(slots, slotCMS), append(blobs, wrap(magicCMS, nil))
}

// space returns the room codesign makes for the signature: 8 bytes more
// than it takes, rounded up to 16.
func (s *superBlob) space() int {
	slots, blobs := s.blobs()
	n := 12 + 8*(len(slots)+1) + s.cd.size()
	for _, b := range blobs {
		n += len(b)
	}
	return int(alignUp(uint64(n+8), 16))
}

// encode returns the signature of code, the image up to it.
func (s *superBlob) encode(code []byte) []byte {
	be := binary.BigEndian
	slots, blobs := s.blobs()
	slots = append([]uint32{slotCodeDirectory}, slots...)
	blobs = append([][]byte{s.cd.encode(code)}, blobs...)
	header := make([]byte, 12+8*len(slots))
	be.PutUint32(header[0:], magicSuperBlob)
	be.PutUint32(header[8:], uint32(len(slots)))
	offset := len(header)
	for i, slot := range slots {
		be.PutUint32(header[12+8*i:], slot)
		be.PutUint32(header[16+8*i:], uint32(offset))
		offset += len(blobs[i])
	}
	be.PutUint32(header[4:], uint32(offset))
	b := make([]byte, 0, offset)
	b = append(b, header...)
	for _, blob := range blobs {
		b = append(b, blob...)
	}
	return b
}

// wrap puts data in a blob with magic's header.
func wrap(magic uint32, data []byte) []byte {
	b := make([]byte, 8, 8+len(data))
	binary.BigEndian.PutUint32(b, magic)
	binary.BigEndian.PutUint32(b[4:], uint32(8+len(data)))
	return append(b, data...)
}
