#!/bin/sh
set -eu
umask 077
mkdir -p /dev/net
[ -e /dev/net/tun ] || mknod /dev/net/tun c 10 200
/usr/local/bin/conflux-relay --config /shared/relay.json --init relay --listen :4433 > /shared/enrollment
/usr/local/bin/conflux-relay --config /shared/relay.json &
relay_pid=$!
trap 'kill "$relay_pid" 2>/dev/null || true' EXIT INT TERM
for attempt in $(seq 1 100); do
 if ip link show conflux0 >/dev/null 2>&1; then break; fi
 sleep 0.1
done
ip addr add 10.250.0.1/16 dev conflux0
nft add table ip conflux_lab
nft 'add chain ip conflux_lab postrouting { type nat hook postrouting priority srcnat; policy accept; }'
nft add rule ip conflux_lab postrouting ip saddr 10.250.0.0/16 oifname eth0 counter masquerade
/usr/local/bin/conflux-bench --echo-listen 10.250.0.1:9000 &
first_port=19841
second_port=19842
if [ "${CONFLUX_LAB_BENCH:-0}" = 1 ]; then
 first_port=19851
 second_port=19852
 /usr/local/bin/conflux-lab-shaper &
fi
# The fixture credentials are disposable and never used outside this lab.
exec /usr/local/bin/sing-box run -c stdin <<JSON
{"log":{"level":"error"},"inbounds":[{"type":"vless","listen":"0.0.0.0","listen_port":$first_port,"users":[{"uuid":"123e4567-e89b-12d3-a456-426614174000"}]},{"type":"vless","listen":"0.0.0.0","listen_port":$second_port,"users":[{"uuid":"123e4567-e89b-12d3-a456-426614174000"}]}],"outbounds":[{"type":"direct","tag":"direct"}],"route":{"final":"direct"}}
JSON
