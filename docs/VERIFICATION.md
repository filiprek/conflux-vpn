# Verification status

## Checks before the source push — 2026-10-16

| Check | Result | Scope |
|---|---|---|
| `go test ./...` | PASS | Root module, including integration and lab shaper packages; opt-in external-binary tests remain conditional |
| `GOOS=windows GOARCH=amd64 go build ./cmd/conflux-service` | PASS | Cross-compilation, not Windows runtime execution |
| `GOOS=windows GOARCH=amd64 go build ./cmd/confluxctl` | PASS | Cross-compilation, not Windows runtime execution |
| Frontend `npm run typecheck` | PASS | Strict TypeScript |
| Frontend `npm run lint` | PASS | ESLint |
| Frontend `npm test` | PASS | 15 tests |
| Frontend `npm run build` | PASS | Production Vite bundle |

## Earlier local evidence — 2026-10-15

Real loopback QUIC integration covered authenticated paths, invalid credentials, source ownership, failover/recovery and 100 clean connection cycles. A separate opt-in integration exercised two actual sing-box 1.14.3 VLESS servers. An isolated privileged Ubuntu container lab exercised client and relay TUN devices, two VLESS paths, QUIC and nftables masquerading, then verified external HTTPS content with its expected SHA-256. The environment's egress proxy was used, so this did not prove direct relay public-IP egress.

Go vulnerability scans used the official offline golang/vulndb snapshot at commit `d5e0cfac5d260523df858cd4c0f849e248468fb6`, containing advisories through 2026-10-08. After updating x/net to v0.61.0 and x/crypto to v0.58.0, Linux and Windows root scans reported no reachable or imported-package vulnerabilities. The sole remaining module-only advisory concerned unimported deprecated OpenPGP code. This does not assert live vulnerability-database freshness.

The Wails Windows executable and unsigned NSIS installer were built locally. Installer execution, Wintun distribution verification, real Windows route/DNS/IPv6 behavior, crash/reboot restoration, handle-leak acceptance and fresh Ubuntu host install/uninstall remain unverified. Controlled aggregation and extended soak acceptance are not claimed by this report.

## Release gate

This is pre-release source. Passing these checks does not satisfy every requirement in the master specification. See [the acceptance matrix](ACCEPTANCE.md) and [limitations](LIMITATIONS.md). Build outputs and local credentials are excluded from Git. No published binary release or complete-MVP acceptance is claimed.
