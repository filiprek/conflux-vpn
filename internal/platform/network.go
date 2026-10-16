package platform

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"net/netip"
	"regexp"
	"strconv"
)

type NetworkInput struct {
	Alias            string
	PhysicalIndex    int
	Gateway, Address string
	MTU              int
	Endpoints        []netip.Addr
}

var aliasPattern = regexp.MustCompile(`^Conflux-[A-Za-z0-9-]{1,48}$`)

// PlanNetwork describes only Conflux-owned changes; traffic switches after protection exists.
func PlanNetwork(n NetworkInput) ([]Step, error) {
	gateway, err := netip.ParseAddr(n.Gateway)
	if err != nil || !gateway.Is4() || n.PhysicalIndex < 1 || !aliasPattern.MatchString(n.Alias) || n.MTU < 576 || n.MTU > 1280 {
		return nil, errors.New("invalid physical route or tunnel configuration")
	}
	address, err := netip.ParseAddr(n.Address)
	if err != nil || !netip.MustParsePrefix("10.250.0.0/16").Contains(address) {
		return nil, errors.New("invalid assigned tunnel address")
	}
	steps := []Step{}
	seen := map[netip.Addr]bool{}
	for _, ip := range n.Endpoints {
		if !ip.Is4() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
			return nil, errors.New("proxy endpoint must be a routable IPv4 address")
		}
		if seen[ip] {
			continue
		}
		seen[ip] = true
		stamp := sha256.Sum256([]byte(n.Alias))
		metric := 20000 + int(binary.BigEndian.Uint16(stamp[:2]))%40000
		args := []string{ip.String() + "/32", strconv.Itoa(n.PhysicalIndex), gateway.String(), strconv.Itoa(metric)}
		steps = append(steps, Step{Command{"bypass", args}, Command{"remove-bypass", args}})
	}
	args := []string{n.Alias, address.String(), strconv.Itoa(n.MTU)}
	steps = append(steps, Step{Command{"address", args}, Command{"remove-address", args}})
	steps = append(steps, Step{Command{"protection", []string{n.Alias}}, Command{"remove-protection", []string{n.Alias}}})
	steps = append(steps, Step{Command{"dns", []string{n.Alias}}, Command{"remove-dns", []string{n.Alias}}})
	steps = append(steps, Step{Command{"tunnel-routes", []string{n.Alias}}, Command{"remove-tunnel-routes", []string{n.Alias}}})
	return steps, nil
}
