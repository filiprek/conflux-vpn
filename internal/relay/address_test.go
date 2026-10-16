package relay

import (
	"net/netip"
	"testing"
)

func TestAddressPoolAcrossOctetAndBroadcast(t *testing.T) {
	prefix := netip.MustParsePrefix("10.250.0.0/16")
	used := map[netip.Addr]bool{}
	for range 300 {
		ip, e := nextAddress(prefix, used)
		if e != nil {
			t.Fatal(e)
		}
		if used[ip] || ip == prefix.Addr().Next() {
			t.Fatal("duplicate or gateway address")
		}
		used[ip] = true
	}
	if !used[netip.MustParseAddr("10.250.1.0")] {
		t.Fatal("truncated /16 to /24")
	}
	prefix = netip.MustParsePrefix("10.250.0.0/24")
	used = map[netip.Addr]bool{}
	for range 253 {
		ip, e := nextAddress(prefix, used)
		if e != nil {
			t.Fatal(e)
		}
		used[ip] = true
	}
	if _, e := nextAddress(prefix, used); e == nil {
		t.Fatal("allocated broadcast")
	}
}
