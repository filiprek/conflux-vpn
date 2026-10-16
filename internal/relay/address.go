package relay

import (
	"errors"
	"net/netip"
)

func nextAddress[V any](prefix netip.Prefix, used map[netip.Addr]V) (netip.Addr, error) {
	for ip := prefix.Addr().Next().Next(); prefix.Contains(ip) && prefix.Contains(ip.Next()); ip = ip.Next() {
		if _, taken := used[ip]; !taken {
			return ip, nil
		}
	}
	return netip.Addr{}, errors.New("relay address pool exhausted")
}
