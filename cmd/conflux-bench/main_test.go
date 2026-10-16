package main

import (
	"net/netip"
	"testing"
)

func TestBenchmarkPacketAndReply(t *testing.T) {
	src := netip.MustParseAddr("10.250.0.2")
	dst := netip.MustParseAddr("1.1.1.1")
	for _, mode := range []byte{upload, download, bidi} {
		p := makePacket(src, dst, 40000, 9000, mode, 1000, 42)
		if checksum(p[:20]) != 0 {
			t.Fatal("invalid IP checksum")
		}
		r, e := echoPacket(p)
		if e != nil {
			t.Fatal(e)
		}
		if string(r[12:16]) != string(dst.AsSlice()) || string(r[16:20]) != string(src.AsSlice()) {
			t.Fatal("addresses not reversed")
		}
		expected := 1028
		if mode == upload {
			expected = 44
		}
		if len(r) != expected {
			t.Fatalf("mode %d length %d", mode, len(r))
		}
	}
	if _, e := echoPayload([]byte("invalid")); e == nil {
		t.Fatal("invalid echo payload accepted")
	}
}
