#!/usr/bin/env bash
# Conflux owns only the named table, persistent TUN and recorded sysctl change.
set -euo pipefail
state=/var/lib/conflux-install
[[ $EUID == 0 && -f "$state/owner" ]] || { echo 'Conflux ownership record missing or not root' >&2; exit 1; }
case ${1:-} in
 up)
  iface=$(cat "$state/egress")
  [[ $iface =~ ^[a-zA-Z0-9_.:-]+$ ]] || exit 1
  if nft list table ip conflux >/dev/null 2>&1; then
   nft list table ip conflux | grep -Fq 'Conflux managed' || { echo 'Refusing unrelated nftables table' >&2; exit 1; }
  else
   nft -f - <<RULES
table ip conflux {
 comment "Conflux managed"
 chain forward {
  type filter hook forward priority 0; policy accept;
  iifname "conflux0" oifname "$iface" ip saddr 10.250.0.0/16 accept
  iifname "$iface" oifname "conflux0" ip daddr 10.250.0.0/16 ct state established,related accept
  iifname "conflux0" drop
  oifname "conflux0" drop
 }
 chain nat {
  type nat hook postrouting priority srcnat; policy accept;
  ip saddr 10.250.0.0/16 oifname "$iface" masquerade
 }
}
RULES
  fi
  if ! ip link show conflux0 >/dev/null 2>&1; then
   ip tuntap add dev conflux0 mode tun user conflux
   touch "$state/tun-created"
  elif [[ ! -f $state/tun-created ]]; then
   echo 'Refusing pre-existing unowned conflux0' >&2; exit 1
  fi
  ip address replace 10.250.0.1/16 dev conflux0
  ip link set dev conflux0 mtu 1100 up
  if [[ $(sysctl -n net.ipv4.ip_forward) != 1 ]]; then
   if [[ ! -f $state/ip-forward.previous ]]; then sysctl -n net.ipv4.ip_forward > "$state/ip-forward.previous"; fi
   sysctl -qw net.ipv4.ip_forward=1
  fi
 ;;
 down)
  if nft list table ip conflux >/dev/null 2>&1; then
   nft list table ip conflux | grep -Fq 'Conflux managed' || { echo 'Refusing unrelated nftables table' >&2; exit 1; }
   nft delete table ip conflux
  fi
  if [[ -f $state/tun-created ]]; then
   ip link delete conflux0 2>/dev/null || [[ ! -e /sys/class/net/conflux0 ]]
   rm -f "$state/tun-created"
  fi
  # Restore only if still equal to our assigned value; leave external changes intact.
  if [[ -f $state/ip-forward.previous ]]; then
   [[ $(sysctl -n net.ipv4.ip_forward) != 1 ]] || sysctl -qw "net.ipv4.ip_forward=$(cat "$state/ip-forward.previous")"
   rm -f "$state/ip-forward.previous"
  fi
 ;;
 *) echo 'Usage: network.sh up|down' >&2; exit 2;;
esac
