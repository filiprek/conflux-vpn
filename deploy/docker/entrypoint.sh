#!/bin/sh
# Runs the relay inside its own network namespace: NAT and forwarding apply to the
# container only, and Docker publishes UDP 4433 on the host.
set -eu
umask 077
config=/etc/conflux/relay.json

fail() { echo "conflux-relay: $*" >&2; exit 1; }
[ -c /dev/net/tun ] || fail 'start the container with --device /dev/net/tun'
[ "$(cat /proc/sys/net/ipv4/ip_forward)" = 1 ] || fail 'start the container with --sysctl net.ipv4.ip_forward=1'

if [ ! -f "$config" ]; then
  [ -n "${CONFLUX_PUBLIC_HOST:-}" ] || fail "set CONFLUX_PUBLIC_HOST to this server's public IPv4 or DNS name for the first start"
  conflux-relay --config "$config" --init "$CONFLUX_PUBLIC_HOST" --listen :4433 >/dev/null
  echo 'conflux-relay: identity created. Print the secret enrollment line with:' >&2
  echo '  docker exec <container> conflux-relay --config /etc/conflux/relay.json --enrollment' >&2
fi
conflux-relay --config "$config" --check >/dev/null

iface=$(ip -4 route show default | awk '{for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit }}')
[ -n "$iface" ] || fail 'no IPv4 default route'

conflux-relay --config "$config" &
pid=$!
trap 'kill -TERM "$pid" 2>/dev/null || true' INT TERM

# The relay creates conflux0 itself; address it and masquerade the tunnel subnet.
tries=0
until ip link show conflux0 >/dev/null 2>&1; do
  kill -0 "$pid" 2>/dev/null || { wait "$pid"; exit $?; }
  tries=$((tries + 1))
  [ "$tries" -lt 100 ] || fail 'relay did not create conflux0'
  sleep 0.1
done
ip address replace 10.250.0.1/16 dev conflux0
nft list table ip conflux >/dev/null 2>&1 || nft -f - <<RULES
table ip conflux {
  chain nat {
    type nat hook postrouting priority srcnat; policy accept;
    ip saddr 10.250.0.0/16 oifname "$iface" masquerade
  }
}
RULES
echo "conflux-relay: listening on UDP 4433 via $iface" >&2

status=0
wait "$pid" || status=$?
# A signal interrupts the first wait; collect the relay's real exit status.
if kill -0 "$pid" 2>/dev/null; then wait "$pid" || status=$?; fi
exit "$status"
