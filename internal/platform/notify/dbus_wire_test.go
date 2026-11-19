package notify

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

// roundTrip encodes args by sig and decodes them again.
func roundTrip(t *testing.T, sig string, args []any) []any {
	t.Helper()
	m := &message{typ: msgCall, path: "/p", member: "M", sig: sig, body: args}
	b, err := m.marshal(1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := readMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	return got.body
}

// The bytes of a call are laid out as the specification lays them out,
// checked here against a message worked out by hand.
func TestMarshalHello(t *testing.T) {
	m := &message{typ: msgCall, dest: "org.freedesktop.DBus", path: "/org/freedesktop/DBus", iface: "org.freedesktop.DBus", member: "Hello"}
	b, err := m.marshal(1)
	if err != nil {
		t.Fatal(err)
	}
	var want []byte
	want = append(want, 'l', 1, 0, 1)
	want = binary.LittleEndian.AppendUint32(want, 0) // body
	want = binary.LittleEndian.AppendUint32(want, 1) // serial
	field := func(code byte, sig, s string) []byte {
		f := []byte{code, 1, sig[0], 0}
		f = binary.LittleEndian.AppendUint32(f, uint32(len(s)))
		f = append(f, s...)
		return append(f, 0)
	}
	var fields []byte
	for _, f := range [][]byte{
		field(fieldPath, "o", "/org/freedesktop/DBus"),
		field(fieldInterface, "s", "org.freedesktop.DBus"),
		field(fieldMember, "s", "Hello"),
		field(fieldDestination, "s", "org.freedesktop.DBus"),
	} {
		for len(fields)%8 != 0 {
			fields = append(fields, 0)
		}
		fields = append(fields, f...)
	}
	want = binary.LittleEndian.AppendUint32(want, uint32(len(fields)))
	want = append(want, fields...)
	for len(want)%8 != 0 {
		want = append(want, 0)
	}
	if !bytes.Equal(b, want) {
		t.Errorf("Hello\n% x\nwant\n% x", b, want)
	}
}

func TestWireRoundTrip(t *testing.T) {
	sig := "ybiusogasa{sv}(us)v"
	args := []any{
		byte(7), true, int32(-3), uint32(1 << 31), "héllo", "/a/b", "a{sv}",
		[]string{"x", "", "yz"},
		[]any{dictEntry{"k", variant{"y", byte(1)}}, dictEntry{"l", variant{"as", []any{"q"}}}},
		[]any{uint32(9), "s"},
		variant{"(ii)", []any{int32(1), int32(2)}},
	}
	got := roundTrip(t, sig, args)
	args[7] = []any{"x", "", "yz"} // decoded arrays are []any
	if !reflect.DeepEqual(got, args) {
		t.Errorf("round trip\n%#v\nwant\n%#v", got, args)
	}
	// An empty array of 8-aligned elements still pads to them, and its
	// length counts none of the padding.
	got = roundTrip(t, "ya{sv}u", []any{byte(1), []any{}, uint32(5)})
	if !reflect.DeepEqual(got, []any{byte(1), []any{}, uint32(5)}) {
		t.Errorf("empty array round trip: %#v", got)
	}
}

func TestMarshalRejectsWrongValues(t *testing.T) {
	for _, c := range []struct {
		sig  string
		args []any
	}{
		{"s", []any{1}},
		{"s", []any{"a\x00b"}},
		{"u", []any{int32(1)}},
		{"ss", []any{"a"}},
		{"(us)", []any{[]any{uint32(1)}}},
		{"v", []any{variant{"ss", []any{}}}},
		{"a{", []any{nil}},
	} {
		m := &message{typ: msgCall, path: "/", member: "M", sig: c.sig, body: c.args}
		if _, err := m.marshal(1); err == nil {
			t.Errorf("marshal of %v as %q succeeded", c.args, c.sig)
		}
	}
}

// A message cut anywhere, or with a byte changed anywhere, is an error,
// never a panic: it comes from another process.
func TestReadMessageSurvivesDamage(t *testing.T) {
	m := &message{typ: msgSignal, path: fdPath, iface: fdService, member: "ActionInvoked", sig: "usa{sv}v", body: []any{
		uint32(3), "default", []any{dictEntry{"a", variant{"u", uint32(1)}}}, variant{"as", []any{"x"}},
	}}
	b, err := m.marshal(4)
	if err != nil {
		t.Fatal(err)
	}
	for n := range len(b) {
		if _, err := readMessage(bytes.NewReader(b[:n])); err == nil {
			t.Fatalf("a message cut to %d of %d bytes was read", n, len(b))
		}
	}
	for i := range b {
		for _, v := range []byte{0, 0xff, 'a', '(', 0x80} {
			c := bytes.Clone(b)
			c[i] = v
			_, _ = readMessage(bytes.NewReader(c))
		}
	}
	got, err := readMessage(bytes.NewReader(b))
	if err != nil || got.serial != 4 || got.member != "ActionInvoked" || got.body[1] != "default" {
		t.Errorf("readMessage = %+v, %v", got, err)
	}
}

func TestReadsBigEndian(t *testing.T) {
	// A reply as a big-endian peer sends it: "u" 42 in reply to serial 9.
	var b []byte
	b = append(b, 'B', msgReturn, 0, 1)
	b = binary.BigEndian.AppendUint32(b, 4)
	b = binary.BigEndian.AppendUint32(b, 2)
	fields := []byte{fieldReplySerial, 1, 'u', 0}
	fields = binary.BigEndian.AppendUint32(fields, 9)
	fields = append(fields, fieldSignature, 1, 'g', 0, 1, 'u', 0)
	b = binary.BigEndian.AppendUint32(b, uint32(len(fields)))
	b = append(b, fields...)
	for len(b)%8 != 0 {
		b = append(b, 0)
	}
	b = binary.BigEndian.AppendUint32(b, 42)
	m, err := readMessage(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.replySerial != 9 || m.sig != "u" || !reflect.DeepEqual(m.body, []any{uint32(42)}) {
		t.Errorf("big-endian reply %+v", m)
	}
}
