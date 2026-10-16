# Limitations and release blockers

- Pre-release source, with no published binary download or signed installer declared. See [ACCEPTANCE.md](ACCEPTANCE.md); source existence, compilation and unit tests do not establish a complete working MVP.
- Target platforms: Windows 11 x64 desktop/service, Ubuntu 24.04 x86_64 relay. Linux desktop/macOS/mobile clients are outside MVP. Windows 10 is unverified.
- IPv4 tunneling only. IPv6 must be reversibly blocked while connected; actual leak-prevention and restoration remain Windows acceptance checks. No IPv6 transport guarantee is made.
- Multiple proxies sharing one physical access link cannot multiply its line rate. Aggregation is possible only when independent underlay/server limits leave spare client uplink capacity; relay, CPU and path skew can reduce performance.
- Bond requires real end-to-end UDP/QUIC via each SOCKS/VLESS path. TCP-only nodes and blocked UDP are ineligible. Parser acceptance does not imply network eligibility.
- Only VLESS fields/transports supported by both the parser and pinned sing-box can be used. Reject unknown or unsupported security/transport fields explicitly. VMess, Trojan, Hysteria2, TUIC, plugin executables and arbitrary outbound JSON are not automatically supported. Check the current parser error before assuming gRPC or other upstream features exist.
- No Wintun runtime is bundled. Operators must independently verify the official binary and its redistribution terms.
- Project installer is unsigned. No fake signing identity, automatic update, analytics, telemetry or remote crash upload is included.
- The real isolated Docker two-VLESS/TUN/NAT smoke passed, through an HTTPS egress proxy. Direct public-IP proof and controlled aggregation throughput remain unproven. Long soak, 100 Windows networking cycles, Windows crash/reboot restoration and fresh root Ubuntu systemd install/uninstall require target-environment execution. Unrun checks are not passes.
- A VPN does not provide anonymity against accounts, application fingerprinting, compromised endpoints or the relay operator. No independent security audit is claimed.
