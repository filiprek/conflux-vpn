package platform

import (
	"net/netip"
	"strings"
	"testing"
)

func TestNetworkPlanOwnedAndDeduplicated(t *testing.T) {
	steps, err := PlanNetwork(NetworkInput{Alias: "Conflux-123", PhysicalIndex: 4, Gateway: "192.0.2.1", Address: "10.250.0.2", MTU: 1280, Endpoints: []netip.Addr{netip.MustParseAddr("203.0.113.2"), netip.MustParseAddr("203.0.113.2")}})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, s := range steps {
		if s.Do.Name == "bypass" {
			n++
		}
		if s.Undo.Name == "" {
			t.Fatal("unreversible step")
		}
		if strings.Contains(strings.Join(s.Do.Args, " "), "Remove-NetRoute") {
			t.Fatal("shell injection")
		}
	}
	if n != 1 {
		t.Fatal("duplicate bypass routes", n)
	}
	if steps[len(steps)-1].Do.Name != "tunnel-routes" {
		t.Fatal("route switch must occur last")
	}
}
func TestNetworkPlanRejectsUnsafeEndpoints(t *testing.T) {
	for _, s := range []string{"::1", "127.0.0.1", "0.0.0.0", "224.0.0.1"} {
		_, err := PlanNetwork(NetworkInput{Alias: "Conflux-123", PhysicalIndex: 4, Gateway: "192.0.2.1", Address: "10.250.0.2", MTU: 1280, Endpoints: []netip.Addr{netip.MustParseAddr(s)}})
		if err == nil {
			t.Fatal(s)
		}
	}
}
