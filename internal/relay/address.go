package relay

import (
	"errors"
	"net/netip"
)

func nextAddress(prefix netip.Prefix, used map[netip.Addr]bool) (netip.Addr, error) {
	for ip := prefix.Addr().Next().Next(); prefix.Contains(ip) && prefix.Contains(ip.Next()); ip = ip.Next() {
		if !used[ip] {
			return ip, nil
		}
	}
	return netip.Addr{}, errors.New("relay address pool exhausted")
}
