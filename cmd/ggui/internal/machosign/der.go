package machosign

import (
	"bytes"
	"fmt"
	"math/big"
	"slices"

	"github.com/ironpark/zapp/pkg/plist"
)

// parseEntitlements reads an entitlements property list, whose root is
// a dictionary.
func parseEntitlements(data []byte) (map[string]any, error) {
	return plist.ParseDict(data)
}

// xmlEntitlements returns entitlements as the XML property list the
// signature holds: as given when they are XML, as codesign keeps them,
// and converted when they are a binary property list.
func xmlEntitlements(data []byte) ([]byte, error) {
	d, err := parseEntitlements(data)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("bplist")) {
		return data, nil
	}
	return plist.MarshalXML(d)
}

// DER tags of the entitlements encoding.
const (
	derBoolean  = 0x01
	derInteger  = 0x02
	derUTF8     = 0x0c
	derSequence = 0x30
	derDict     = 0xb0 // [CONTEXT 16], constructed
	derEnvelope = 0x70 // [APPLICATION 16], constructed
	derVersion  = 1
)

// DEREntitlements returns the DER encoding of an entitlements property
// list, as macOS 12 and later read entitlements: a version, 1, and the
// dictionary, whose entries are sequences of a UTF-8 key and its value,
// sorted by key. Values are dictionaries, arrays, strings, booleans and
// integers; entitlements hold nothing else.
func DEREntitlements(data []byte) ([]byte, error) {
	d, err := parseEntitlements(data)
	if err != nil {
		return nil, err
	}
	body, err := derValue(d)
	if err != nil {
		return nil, err
	}
	return derTLV(derEnvelope, append(derTLV(derInteger, []byte{derVersion}), body...)), nil
}

func derValue(v any) ([]byte, error) {
	switch v := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		var body []byte
		for _, k := range keys {
			e, err := derValue(v[k])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", k, err)
			}
			body = append(body, derTLV(derSequence, append(derTLV(derUTF8, []byte(k)), e...))...)
		}
		return derTLV(derDict, body), nil
	case []any:
		var body []byte
		for i, e := range v {
			b, err := derValue(e)
			if err != nil {
				return nil, fmt.Errorf("item %d: %w", i, err)
			}
			body = append(body, b...)
		}
		return derTLV(derSequence, body), nil
	case string:
		return derTLV(derUTF8, []byte(v)), nil
	case bool:
		if v {
			return derTLV(derBoolean, []byte{0xff}), nil
		}
		return derTLV(derBoolean, []byte{0}), nil
	case int64:
		return derTLV(derInteger, derInt(big.NewInt(v))), nil
	case uint64:
		return derTLV(derInteger, derInt(new(big.Int).SetUint64(v))), nil
	}
	return nil, fmt.Errorf("entitlements cannot hold a %T", v)
}

// derInt is n in the fewest bytes of two's complement.
func derInt(n *big.Int) []byte {
	if n.Sign() >= 0 {
		b := n.Bytes()
		if len(b) == 0 || b[0]&0x80 != 0 {
			b = append([]byte{0}, b...)
		}
		return b
	}
	// -n-1 complemented is n; one more byte than it takes keeps the sign.
	m := new(big.Int).Not(n) // -n-1, not negative
	b := m.Bytes()
	if len(b) == 0 || b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	for i := range b {
		b[i] = ^b[i]
	}
	return b
}

// derTLV encodes a tag, the length of body in DER's form, and body.
func derTLV(tag byte, body []byte) []byte {
	n := len(body)
	out := []byte{tag}
	switch {
	case n < 0x80:
		out = append(out, byte(n))
	case n <= 0xff:
		out = append(out, 0x81, byte(n))
	case n <= 0xffff:
		out = append(out, 0x82, byte(n>>8), byte(n))
	case n <= 0xffffff:
		out = append(out, 0x83, byte(n>>16), byte(n>>8), byte(n))
	default:
		out = append(out, 0x84, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	return append(out, body...)
}
