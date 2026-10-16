# Verification status

## Native Windows 11 build — 2026-10-16

| Check | Result | Scope |
|---|---|---|
| `go test ./...` (root module) | PASS | Unit, loopback QUIC integration and lab shaper packages on Windows |
| `go test ./...` (`ui/desktop`) | PASS | Desktop bridge, including service error redaction |
| `go vet ./...` (Windows and `GOOS=linux`) | PASS | Root and desktop modules |
| `gofmt -l` | PASS | No unformatted sources |
| `staticcheck@v0.8.2 ./...` | PASS | No findings |
| `govulncheck@v1.9.0` (root, Windows and Linux) | PASS | No reachable or imported vulnerabilities; one module-only advisory in x/crypto without a fix and not called |
| `govulncheck@v1.9.0 -tags production` (`ui/desktop`) | PASS | No vulnerabilities after updating x/net, x/text and x/crypto |
| Frontend typecheck, lint, tests, build | PASS | 17 tests |
| `npm audit --audit-level=high` | PASS | 0 vulnerabilities |
| `./scripts/dev.ps1 build` | PASS | `conflux-service.exe`, `confluxctl.exe`, `conflux-bench.exe` and Wails `Conflux.exe` |
| `bash scripts/build.sh` | PASS | Linux relay and bench cross-build |

## Earlier lab evidence — 2026-10-15

Loopback QUIC integration covered authenticated paths, invalid credentials, source ownership, failover/recovery and 100 clean connection cycles. An opt-in integration exercised two sing-box 1.14.3 VLESS servers. An isolated privileged Ubuntu container lab exercised client and relay TUN devices, two VLESS paths, QUIC and nftables masquerading, then verified external HTTPS content with its expected SHA-256. That lab used an HTTPS egress proxy, so it did not prove direct relay public-IP egress.

## Not yet verified

Installer execution, Wintun distribution verification, real Windows route/DNS/IPv6 behavior, crash/reboot restoration, handle-leak acceptance, fresh Ubuntu host install/uninstall, controlled aggregation and extended soak. See [the acceptance matrix](ACCEPTANCE.md) and [limitations](LIMITATIONS.md).
