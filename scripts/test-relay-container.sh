#!/usr/bin/env bash
# Starts the relay image with TUN, waits for an authenticated QUIC health check and
# confirms the enrollment line, NAT table and published UDP listener.
set -euo pipefail
image=${1:?usage: test-relay-container.sh IMAGE}
name=conflux-relay-smoke-$$
cleanup() { docker logs "$name" 2>&1 | tail -20 || true; docker rm -f "$name" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker run -d --name "$name" --cap-add NET_ADMIN --device /dev/net/tun \
  --sysctl net.ipv4.ip_forward=1 -p 127.0.0.1:44330:4433/udp \
  -e CONFLUX_PUBLIC_HOST=127.0.0.1 "$image" >/dev/null
for _ in $(seq 1 30); do
  if docker exec "$name" conflux-relay --config /etc/conflux/relay.json --health; then
    docker exec "$name" conflux-relay --config /etc/conflux/relay.json --enrollment | grep -q '^conflux://127\.0\.0\.1:4433?'
    docker exec "$name" nft list table ip conflux | grep -q masquerade
    docker exec "$name" ip -4 address show conflux0 | grep -q '10\.250\.0\.1/16'
    echo 'relay container smoke test passed'
    exit 0
  fi
  sleep 1
done
echo 'relay container did not become healthy' >&2
exit 1
