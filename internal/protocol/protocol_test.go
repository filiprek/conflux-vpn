package protocol

import (
	"bytes"
	"testing"
)

func TestFrame(t *testing.T) {
	b, err := Encode(Frame{Sequence: 42, Payload: []byte{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := Decode(b)
	if err != nil || f.Sequence != 42 || !bytes.Equal(f.Payload, []byte{1, 2, 3}) {
		t.Fatalf("roundtrip: %+v %v", f, err)
	}
	for _, bad := range [][]byte{nil, b[:13], append(append([]byte{}, b...), 0), {2, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0}} {
		if _, err := Decode(bad); err == nil {
			t.Fatal("accepted malformed frame")
		}
	}
	if _, err := Encode(Frame{Payload: make([]byte, MaxMTU+1)}); err == nil {
		t.Fatal("accepted oversized frame")
	}
	for _, offset := range []int{0, 1, 2, 3, 12, 13} {
		bad := append([]byte{}, b...)
		bad[offset] = 255
		if _, err := Decode(bad); err == nil {
			t.Fatalf("accepted bad header offset %d", offset)
		}
	}
}
func TestControl(t *testing.T) {
	h := Hello{Version: Version, SessionID: "01234567890123456789012345678901", InstanceID: "01234567890123456789012345678901", PathID: "a", Token: "secret", MTU: 1100, Datagrams: true}
	var b bytes.Buffer
	if err := WriteControl(&b, h); err != nil {
		t.Fatal(err)
	}
	var got Hello
	if err := ReadControl(&b, &got); err != nil || got != h {
		t.Fatalf("control roundtrip %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatal(err)
	}
	got.Version++
	if err := got.Validate(); err == nil {
		t.Fatal("version accepted")
	}
	if err := ReadControl(bytes.NewReader([]byte{255, 255, 255, 255}), &got); err == nil {
		t.Fatal("oversized accepted")
	}
}
func FuzzFrame(f *testing.F) {
	f.Add([]byte{1, 0})
	f.Fuzz(func(t *testing.T, b []byte) {
		v, e := Decode(b)
		if e == nil {
			out, e := Encode(v)
			if e != nil || !bytes.Equal(out, b) {
				t.Fatal("noncanonical")
			}
		}
	})
}
func FuzzControl(f *testing.F) {
	f.Add([]byte{0, 0, 0, 2, '{', '}'})
	f.Fuzz(func(t *testing.T, b []byte) { var h Hello; _ = ReadControl(bytes.NewReader(b), &h) })
}

type shortWriter struct{}

func (shortWriter) Write(b []byte) (int, error) { return len(b) - 1, nil }
func TestControlRejectsShortWrite(t *testing.T) {
	if err := WriteControl(shortWriter{}, Welcome{}); err == nil {
		t.Fatal("short control write ignored")
	}
}
