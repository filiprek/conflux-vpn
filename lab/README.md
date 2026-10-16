# Disposable Linux TUN / VLESS / NAT lab

Requires Linux Docker, Go from `go.mod`, Python 3, `rg`, `sha256sum`, and permission to run containers with `NET_ADMIN`. The Ubuntu 24.04 image is pinned by official manifest digest. The script builds the actual relay and core client, using a separately installed, hash-pinned sing-box 1.14.3 executable. No mock proxy or mock packet device is used in this lab.

```bash
export CONFLUX_SINGBOX_PATH=/absolute/path/to/sing-box
export CONFLUX_SINGBOX_SHA256=<independently-verified-official-binary-sha256>
./lab/run.sh
# Optional controlled equal-cap benchmark and bounded packet soak:
CONFLUX_LAB_BENCH=1 ./lab/run.sh
```

The containers have independent network namespaces. The client creates an actual Linux TUN device, enrolls over two SOCKS UDP associations carried by separate VLESS TCP paths, then routes external HTTPS through that TUN. The actual relay reads and writes its own TUN; its namespace enables IPv4 forwarding and nftables masquerade. HTTPS retrieves the upstream versioned sing-box license and verifies its known SHA-256. Both measured core path counters must show bidirectional IPv4 traffic. The script prints TUN and NAT counters and removes its containers, network, transient identities and build staging on exit. It leaves the local Docker test image cached. It changes no host routes or firewall rules.

The environment may require an HTTP egress proxy. In that case the script forwards that proxy setting into the lab, resolves its endpoint for the isolated containers, and copies the current host public CA bundle into the test image. TLS validation remains enabled. Traffic to the egress proxy still traverses client TUN, VLESS, relay TUN and relay masquerade. This mode proves that chain and external HTTPS retrieval; it does not prove that a destination sees the relay's direct public IP. Public-IP echo sites were blocked by the execution environment, so that acceptance remains separate.

The optional benchmark shapes each of the two VLESS TCP ports to 2 Mbit/s in both directions, uses the actual benchmark responder bound to the relay TUN IPv4 address, compares a single path against two paths for 12 seconds each, and reports measured delivered return Mbps and the ratio. A ratio below 1.5 fails acceptance and remains visible. It then removes shaping inside the containers and offers 70,000 real benchmark packets during a bounded soak; the test requires at least 60,000 real replies. Resource counts are measured, never fabricated. This is a short functional soak, not the specification's multi-hour stability acceptance.

The image is a local test artifact. sing-box remains an unmodified, separately installed GPL-3.0-or-later executable; Conflux does not link it. Do not publish this image as a product artifact. Use the project's release packaging and third-party notices for distribution. Lab credentials are disposable fixtures; no production credentials belong here.

The separate opt-in Go test `TestRealTwoVLESSPathTunnel` uses real loopback VLESS servers, real QUIC and a core relay with bounded memory packet devices. It kills and restarts one upstream process and verifies the session survives and both paths carry traffic. It complements this real TUN/NAT lab.
