#!/usr/bin/env bash
set -euo pipefail
: "${CONFLUX_SINGBOX_PATH:?Set path to official separately installed sing-box 1.14.3}"
: "${CONFLUX_SINGBOX_SHA256:?Set trusted SHA256 of that executable}"
repo_root=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_root"
actual_hash=$(sha256sum "$CONFLUX_SINGBOX_PATH" | cut -d ' ' -f 1)
[[ "$actual_hash" == "$CONFLUX_SINGBOX_SHA256" ]] || { echo 'sing-box hash mismatch' >&2; exit 1; }
lab_id="conflux-lab-$$"
lab_network="$lab_id-net"
lab_relay="$lab_id-relay"
lab_client="$lab_id-client"
lab_shared=$(mktemp -d)
export DOCKER_CONFIG="$lab_shared/docker"
mkdir -p "$DOCKER_CONFIG" "$lab_shared/coord"
cleanup() {
 lab_exit=$?
 if [[ "$lab_exit" != 0 ]]; then docker logs "$lab_relay" >&2 2>/dev/null || true; fi
 docker rm -f "$lab_client" "$lab_relay" >/dev/null 2>&1 || true
 docker network rm "$lab_network" >/dev/null 2>&1 || true
 rm -rf "$lab_shared" lab/.build
}
trap cleanup EXIT
mkdir -p lab/.build
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o lab/.build/conflux-relay ./cmd/conflux-relay
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o lab/.build/conflux-bench ./cmd/conflux-bench
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o lab/.build/conflux-lab-shaper ./lab/shaper
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o lab/.build/conflux-lab-client ./lab/client
cp "$CONFLUX_SINGBOX_PATH" lab/.build/sing-box
cp "${SSL_CERT_FILE:-/etc/ssl/certs/ca-certificates.crt}" lab/.build/ca-certificates.crt
chmod +x lab/.build/*
build_proxy_args=()
lab_proxy=""
if [[ -n "${HTTP_PROXY:-}" ]]; then
 proxy_host=$(python3 -c 'import os,urllib.parse; print(urllib.parse.urlparse(os.environ["HTTP_PROXY"]).hostname)')
 proxy_ip=$(getent ahostsv4 "$proxy_host" | awk 'NR==1{print $1}')
 build_proxy_args+=(--add-host "$proxy_host:$proxy_ip" --build-arg "http_proxy=$HTTP_PROXY" --build-arg "https_proxy=${HTTPS_PROXY:-$HTTP_PROXY}")
 lab_proxy=$(python3 -c 'import os,socket,urllib.parse; u=urllib.parse.urlparse(os.environ["HTTP_PROXY"]); print(urllib.parse.urlunparse(u._replace(netloc=socket.gethostbyname(u.hostname)+":"+str(u.port or 80))))')
fi
docker build "${build_proxy_args[@]}" -t conflux-privileged-lab:local lab
docker network create "$lab_network" >/dev/null
docker run -d --name "$lab_relay" --network "$lab_network" --network-alias relay --cap-add NET_ADMIN --sysctl net.ipv4.ip_forward=1 -e "CONFLUX_LAB_BENCH=${CONFLUX_LAB_BENCH:-0}" -v "$lab_shared:/shared" conflux-privileged-lab:local /bin/sh /lab/relay-entry.sh >/dev/null
for attempt in $(seq 1 100); do
 if docker exec "$lab_relay" ip addr show conflux0 2>/dev/null | rg -q '10.250.0.1'; then break; fi
 sleep 0.1
done
printf '%s\n' 'Relay external HTTPS reference (raw upstream versioned license):'
docker exec -e "https_proxy=$lab_proxy" "$lab_relay" curl --fail --silent --show-error --max-time 20 --output /tmp/reference https://raw.githubusercontent.com/SagerNet/sing-box/v1.14.3/LICENSE
docker exec "$lab_relay" sha256sum /tmp/reference
printf '%s\n' 'Client HTTPS through two VLESS paths + QUIC + real TUN + relay NAT:'
docker run --name "$lab_client" --network "$lab_network" --cap-add NET_ADMIN -e "CONFLUX_SINGBOX_SHA256=$CONFLUX_SINGBOX_SHA256" -e "CONFLUX_LAB_EGRESS_PROXY=$lab_proxy" -e "CONFLUX_LAB_BENCH=${CONFLUX_LAB_BENCH:-0}" -v "$lab_shared/enrollment:/shared/enrollment:ro" -v "$lab_shared/coord:/coord" conflux-privileged-lab:local /bin/sh /lab/client-entry.sh
printf '%s\n' 'Relay NAT counters:'
docker exec "$lab_relay" nft list table ip conflux_lab
