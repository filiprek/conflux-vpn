# Third-party notices

Conflux source is Apache-2.0; see [LICENSE](LICENSE). This does not relicense dependencies. Exact direct dependency versions are in `go.mod`, `ui/desktop/go.mod` and `ui/desktop/frontend/package-lock.json`. Release distributors must include the licenses of all direct and transitive components actually shipped, and inspect `go version -m` for every binary. No externally supplied runtime is included in this source checkout.

| Component | Pin | License | Authoritative source |
|---|---|---|---|
| Go | 1.27.2 | BSD-3-Clause | [tagged LICENSE](https://github.com/golang/go/blob/go1.27.2/LICENSE), [copy](docs/licenses/go.txt) |
| quic-go | 0.63.0 | MIT | [tagged LICENSE](https://github.com/quic-go/quic-go/blob/v0.63.0/LICENSE), [copy](docs/licenses/quic-go.txt) |
| Wails | 2.16.0 | MIT | [tagged LICENSE](https://github.com/wailsapp/wails/blob/v2.16.0/LICENSE), [copy](docs/licenses/Wails.txt) |
| golang.org/x/sys | 0.49.0 | BSD-3-Clause | [tagged LICENSE](https://github.com/golang/sys/blob/v0.49.0/LICENSE), [copy](docs/licenses/x-sys.txt) |
| go-winio | 0.6.4 | MIT | [tagged LICENSE](https://github.com/microsoft/go-winio/blob/v0.6.4/LICENSE), [copy](docs/licenses/go-winio.txt) |
| sing-box | 1.14.3 | GPL-3.0-or-later, additional naming restriction in upstream notice | [tagged LICENSE](https://github.com/SagerNet/sing-box/blob/v1.14.3/LICENSE), [copy](docs/licenses/sing-box.txt) |
| Wintun source | upstream source; binary selected separately | GPL-2.0 in source COPYING | [official source mirror COPYING](https://github.com/WireGuard/wintun/blob/master/COPYING), [copy](docs/licenses/wintun-source.txt) |

License files above were fetched from upstream on 2026-10-15. Wintun's official precompiled DLL distribution can have different redistribution terms from its source. The official download site was unavailable in this environment (HTTP 403); no DLL version, hash or precompiled license has been verified here. This is a release blocker for bundled Wintun. Obtain the official archive from https://www.wintun.net/, retain its license, independently verify its published provenance and SHA-256, and verify the DLL's Authenticode signature. The Windows installer requires a supplied DLL, expected SHA-256 and a valid WireGuard signature; it does not silently fetch one.

sing-box runs as a **separate executable** via a controlled configuration and loopback SOCKS protocol. Conflux does not link sing-box code. The operator supplies the official verified executable. If distributing it, comply with its GPL obligations, including corresponding source and license delivery for that exact build; retain the upstream naming restriction. Separate execution does not remove distribution obligations. Do not describe sing-box as Apache-licensed.

Frontend dependencies, including React/React DOM 19.3.1, TypeScript 6.0.3 and the locked build/test dependencies, must be inventoried from the committed lockfile before a binary release. React and TypeScript upstream use MIT and Apache-2.0 respectively; inspect exact package license files using `npm ci` and include all notices in shipped artifacts. See the [complete locked frontend inventory](docs/FRONTEND_DEPENDENCIES.md) and retained React/React DOM/TypeScript texts in `docs/licenses`. The lockfile is the complete version inventory; this table lists primary runtime boundaries, not a complete release SBOM.

Build/security tools are pinned: staticcheck `honnef.co/go/tools v0.8.2` (official tag 2026.2.2), govulncheck `golang.org/x/vuln v1.9.0`; module metadata verified with proxy.golang.org. NSIS3.11 is the packaging prerequisite; its installer stub is distributed in the generated setup executable. The authenticated Debian3.11-1 package copyright/license inventory is retained in [docs/licenses/NSIS-Debian-copyright.txt](docs/licenses/NSIS-Debian-copyright.txt). The selected zlib-compressed installer uses the NSIS zlib license; retain the notices for included plugins/stub. System WebView2 and Windows components are provided under their own Microsoft terms.
