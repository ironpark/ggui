package notify

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

// This file is the D-Bus wire format, as much of it as the freedesktop
// notification service needs: messages, and the values a signature names,
// with the basic types y b i u s o g and arrays, structs, dict entries and
// variants of them. It is a few hundred lines where a D-Bus library is a
// dependency of thousands, and it is pure, so it is tested anywhere.
//
// A value is given and returned as Go values by its signature:
//
//	y byte    b bool    i int32    u uint32    s, o, g string
//	a []any   (…) []any {kv} dictEntry   v variant
//
// "as" may also be given as a []string.

// variant is a value of type v: its own signature and the value.
type variant struct {
	sig string
	val any
}

// dictEntry is a {kv} value, an element of an a{…} array.
type dictEntry struct {
	key, val any
}

// Message types.
const (
	msgCall   = 1
	msgReturn = 2
	msgError  = 3
	msgSignal = 4
)

// Header field codes.
const (
	fieldPath        = 1
	fieldInterface   = 2
	fieldMember      = 3
	fieldErrorName   = 4
	fieldReplySerial = 5
	fieldDestination = 6
	fieldSender      = 7
	fieldSignature   = 8
)

// The limits the specification sets, which a message read is held to.
const (
	maxArray   = 64 << 20
	maxMessage = 128 << 20
	maxDepth   = 64
)

// message is a D-Bus message, its header fields and its body.
type message struct {
	typ, flags  byte
	serial      uint32
	path        string
	iface       string
	member      string
	errName     string
	replySerial uint32
	dest        string
	sender      string
	sig         string
	body        []any
}

// marshal is m as bytes, little-endian, with serial.
func (m *message) marshal(serial uint32) ([]byte, error) {
	var body encoder
	types, err := splitSig(m.sig)
	if err != nil {
		return nil, err
	}
	if len(types) != len(m.body) {
		return nil, fmt.Errorf("dbus: %d values for signature %q", len(m.body), m.sig)
	}
	for i, t := range types {
		if err := body.value(t, m.body[i], 0); err != nil {
			return nil, err
		}
	}
	var fields []any
	add := func(code byte, sig string, v any) {
		fields = append(fields, []any{code, variant{sig, v}})
	}
	if m.path != "" {
		add(fieldPath, "o", m.path)
	}
	if m.iface != "" {
		add(fieldInterface, "s", m.iface)
	}
	if m.member != "" {
		add(fieldMember, "s", m.member)
	}
	if m.errName != "" {
		add(fieldErrorName, "s", m.errName)
	}
	if m.replySerial != 0 {
		add(fieldReplySerial, "u", m.replySerial)
	}
	if m.dest != "" {
		add(fieldDestination, "s", m.dest)
	}
	if m.sig != "" {
		add(fieldSignature, "g", m.sig)
	}
	h := encoder{b: []byte{'l', m.typ, m.flags, 1}}
	h.u32(uint32(len(body.b)))
	h.u32(serial)
	if err := h.value("a(yv)", fields, 0); err != nil {
		return nil, err
	}
	h.pad(8)
	return append(h.b, body.b...), nil
}

// readMessage reads one message from r.
func readMessage(r io.Reader) (*message, error) {
	var fixed [16]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return nil, err
	}
	var order binary.ByteOrder
	switch fixed[0] {
	case 'l':
		order = binary.LittleEndian
	case 'B':
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("dbus: bad endianness %q", fixed[0])
	}
	bodyLen := order.Uint32(fixed[4:])
	fieldsLen := order.Uint32(fixed[12:])
	if fieldsLen > maxArray || bodyLen > maxMessage {
		return nil, errors.New("dbus: message too long")
	}
	headerLen := 16 + int(fieldsLen)
	padded := (headerLen + 7) &^ 7
	if padded+int(bodyLen) > maxMessage {
		return nil, errors.New("dbus: message too long")
	}
	buf := make([]byte, padded+int(bodyLen))
	copy(buf, fixed[:])
	if _, err := io.ReadFull(r, buf[16:]); err != nil {
		return nil, err
	}
	m := &message{typ: fixed[1], flags: fixed[2], serial: order.Uint32(fixed[8:])}
	d := decoder{b: buf[:headerLen], pos: 12, order: order}
	fields, err := d.value("a(yv)", 0)
	if err != nil {
		return nil, err
	}
	for _, f := range fields.([]any) {
		f := f.([]any)
		code, v := f[0].(byte), f[1].(variant)
		s, _ := v.val.(string)
		switch code {
		case fieldPath:
			m.path = s
		case fieldInterface:
			m.iface = s
		case fieldMember:
			m.member = s
		case fieldErrorName:
			m.errName = s
		case fieldReplySerial:
			m.replySerial, _ = v.val.(uint32)
		case fieldDestination:
			m.dest = s
		case fieldSender:
			m.sender = s
		case fieldSignature:
			m.sig = s
		}
	}
	types, err := splitSig(m.sig)
	if err != nil {
		return nil, err
	}
	bd := decoder{b: buf[padded:], order: order}
	for _, t := range types {
		v, err := bd.value(t, 0)
		if err != nil {
			return nil, err
		}
		m.body = append(m.body, v)
	}
	return m, nil
}

// splitSig splits a signature into its complete types.
func splitSig(sig string) ([]string, error) {
	var types []string
	for sig != "" {
		t, rest, err := nextType(sig, 0)
		if err != nil {
			return nil, err
		}
		types = append(types, t)
		sig = rest
	}
	return types, nil
}

// nextType splits the first complete type off sig.
func nextType(sig string, depth int) (string, string, error) {
	if sig == "" {
		return "", "", errors.New("dbus: signature ends inside a type")
	}
	if depth > maxDepth {
		return "", "", errors.New("dbus: signature nested too deep")
	}
	switch c := sig[0]; c {
	case 'y', 'b', 'n', 'q', 'i', 'u', 'x', 't', 'd', 's', 'o', 'g', 'v', 'h':
		return sig[:1], sig[1:], nil
	case 'a':
		elem, rest, err := nextType(sig[1:], depth+1)
		if err != nil {
			return "", "", err
		}
		return "a" + elem, rest, nil
	case '(', '{':
		end := byte(')')
		if c == '{' {
			end = '}'
		}
		i, n := 1, 0
		for {
			if i < len(sig) && sig[i] == end {
				break
			}
			_, rest, err := nextType(sig[i:], depth+1)
			if err != nil {
				return "", "", err
			}
			i = len(sig) - len(rest)
			n++
		}
		if n == 0 || c == '{' && n != 2 {
			return "", "", fmt.Errorf("dbus: bad signature %q", sig)
		}
		return sig[:i+1], sig[i+1:], nil
	}
	return "", "", fmt.Errorf("dbus: bad signature %q", sig)
}

// alignment is the boundary a value of type t starts on.
func alignment(t string) int {
	switch t[0] {
	case 'y', 'g', 'v':
		return 1
	case 'n', 'q':
		return 2
	case 'x', 't', 'd', '(', '{':
		return 8
	}
	return 4
}

// encoder writes values in little-endian order.
type encoder struct{ b []byte }

func (e *encoder) pad(n int) {
	for len(e.b)%n != 0 {
		e.b = append(e.b, 0)
	}
}

func (e *encoder) u32(v uint32) {
	e.pad(4)
	e.b = binary.LittleEndian.AppendUint32(e.b, v)
}

func (e *encoder) str(s string) {
	e.u32(uint32(len(s)))
	e.b = append(e.b, s...)
	e.b = append(e.b, 0)
}

// value writes v as a value of the complete type t.
func (e *encoder) value(t string, v any, depth int) error {
	if depth > maxDepth {
		return errors.New("dbus: value nested too deep")
	}
	bad := func() error { return fmt.Errorf("dbus: %T is not a value of type %s", v, t) }
	switch t[0] {
	case 'y':
		b, ok := v.(byte)
		if !ok {
			return bad()
		}
		e.b = append(e.b, b)
	case 'b':
		b, ok := v.(bool)
		if !ok {
			return bad()
		}
		n := uint32(0)
		if b {
			n = 1
		}
		e.u32(n)
	case 'i':
		n, ok := v.(int32)
		if !ok {
			return bad()
		}
		e.u32(uint32(n))
	case 'u':
		n, ok := v.(uint32)
		if !ok {
			return bad()
		}
		e.u32(n)
	case 's', 'o':
		s, ok := v.(string)
		if !ok || strings.IndexByte(s, 0) >= 0 {
			return bad()
		}
		e.str(s)
	case 'g':
		s, ok := v.(string)
		if !ok || len(s) > 255 {
			return bad()
		}
		e.b = append(e.b, byte(len(s)))
		e.b = append(e.b, s...)
		e.b = append(e.b, 0)
	case 'a':
		elem := t[1:]
		var items []any
		switch v := v.(type) {
		case []any:
			items = v
		case []string:
			for _, s := range v {
				items = append(items, s)
			}
		default:
			return bad()
		}
		e.u32(0)
		at := len(e.b) - 4
		e.pad(alignment(elem))
		start := len(e.b)
		for _, item := range items {
			if err := e.value(elem, item, depth+1); err != nil {
				return err
			}
		}
		binary.LittleEndian.PutUint32(e.b[at:], uint32(len(e.b)-start))
	case '(':
		fields, ok := v.([]any)
		if !ok {
			return bad()
		}
		types, err := splitSig(t[1 : len(t)-1])
		if err != nil {
			return err
		}
		if len(types) != len(fields) {
			return bad()
		}
		e.pad(8)
		for i, ft := range types {
			if err := e.value(ft, fields[i], depth+1); err != nil {
				return err
			}
		}
	case '{':
		kv, ok := v.(dictEntry)
		if !ok {
			return bad()
		}
		types, err := splitSig(t[1 : len(t)-1])
		if err != nil {
			return err
		}
		e.pad(8)
		if err := e.value(types[0], kv.key, depth+1); err != nil {
			return err
		}
		return e.value(types[1], kv.val, depth+1)
	case 'v':
		vv, ok := v.(variant)
		if !ok {
			return bad()
		}
		if _, rest, err := nextType(vv.sig, 0); err != nil || rest != "" {
			return fmt.Errorf("dbus: variant of signature %q", vv.sig)
		}
		if err := e.value("g", vv.sig, depth+1); err != nil {
			return err
		}
		return e.value(vv.sig, vv.val, depth+1)
	default:
		return fmt.Errorf("dbus: type %s is not supported", t)
	}
	return nil
}

// decoder reads values from b, whose first byte is aligned on 8.
type decoder struct {
	b     []byte
	pos   int
	order binary.ByteOrder
}

var errShort = errors.New("dbus: message ends inside a value")

func (d *decoder) align(n int) error {
	p := (d.pos + n - 1) &^ (n - 1)
	if p > len(d.b) {
		return errShort
	}
	d.pos = p
	return nil
}

func (d *decoder) take(n int) ([]byte, error) {
	if n < 0 || len(d.b)-d.pos < n {
		return nil, errShort
	}
	b := d.b[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

func (d *decoder) u32() (uint32, error) {
	if err := d.align(4); err != nil {
		return 0, err
	}
	b, err := d.take(4)
	if err != nil {
		return 0, err
	}
	return d.order.Uint32(b), nil
}

// value reads a value of the complete type t.
func (d *decoder) value(t string, depth int) (any, error) {
	if depth > maxDepth {
		return nil, errors.New("dbus: value nested too deep")
	}
	switch t[0] {
	case 'y':
		b, err := d.take(1)
		if err != nil {
			return nil, err
		}
		return b[0], nil
	case 'b':
		n, err := d.u32()
		return n != 0, err
	case 'i':
		n, err := d.u32()
		return int32(n), err
	case 'u', 'h':
		return d.u32()
	case 'n', 'q':
		if err := d.align(2); err != nil {
			return nil, err
		}
		b, err := d.take(2)
		if err != nil {
			return nil, err
		}
		if t[0] == 'n' {
			return int16(d.order.Uint16(b)), nil
		}
		return d.order.Uint16(b), nil
	case 'x', 't', 'd':
		if err := d.align(8); err != nil {
			return nil, err
		}
		b, err := d.take(8)
		if err != nil {
			return nil, err
		}
		return d.order.Uint64(b), nil
	case 's', 'o':
		n, err := d.u32()
		if err != nil {
			return nil, err
		}
		if uint64(n) >= uint64(len(d.b)-d.pos) {
			return nil, errShort
		}
		b, err := d.take(int(n) + 1)
		if err != nil {
			return nil, err
		}
		return string(b[:n]), nil
	case 'g':
		l, err := d.take(1)
		if err != nil {
			return nil, err
		}
		b, err := d.take(int(l[0]) + 1)
		if err != nil {
			return nil, err
		}
		return string(b[:l[0]]), nil
	case 'a':
		n, err := d.u32()
		if err != nil {
			return nil, err
		}
		if n > maxArray {
			return nil, errors.New("dbus: array too long")
		}
		elem := t[1:]
		if err := d.align(alignment(elem)); err != nil {
			return nil, err
		}
		if int(n) > len(d.b)-d.pos {
			return nil, errShort
		}
		end := d.pos + int(n)
		items := []any{}
		for d.pos < end {
			v, err := d.value(elem, depth+1)
			if err != nil {
				return nil, err
			}
			items = append(items, v)
		}
		if d.pos != end {
			return nil, errors.New("dbus: array elements overrun its length")
		}
		return items, nil
	case '(', '{':
		if err := d.align(8); err != nil {
			return nil, err
		}
		types, err := splitSig(t[1 : len(t)-1])
		if err != nil {
			return nil, err
		}
		var fields []any
		for _, ft := range types {
			v, err := d.value(ft, depth+1)
			if err != nil {
				return nil, err
			}
			fields = append(fields, v)
		}
		if t[0] == '{' {
			return dictEntry{fields[0], fields[1]}, nil
		}
		return fields, nil
	case 'v':
		s, err := d.value("g", depth+1)
		if err != nil {
			return nil, err
		}
		sig := s.(string)
		if _, rest, err := nextType(sig, depth+1); err != nil || rest != "" {
			return nil, fmt.Errorf("dbus: variant of signature %q", sig)
		}
		v, err := d.value(sig, depth+1)
		if err != nil {
			return nil, err
		}
		return variant{sig, v}, nil
	}
	return nil, fmt.Errorf("dbus: type %s is not supported", t)
}
