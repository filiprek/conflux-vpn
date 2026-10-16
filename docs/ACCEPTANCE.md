# Release acceptance matrix

This matrix is deliberately conservative. **NOT RUN** means no target-environment evidence is recorded here. **REQUIRES REAL ENVIRONMENT** identifies an unavailable system or hardware prerequisite, not a pass. Package tests/build evidence, when run, is recorded in `docs/VERIFICATION.md`; it does not automatically satisfy a system-wide VPN requirement.

| Requirement | Implementation / verification | Status |
|---|---|---|
| Bounded versioned codecs and malformed input | `internal/protocol`, unit/fuzz/race commands | See current verification; release fuzz review required |
| Scheduling, bounded reorder, recovery decisions | `internal/multipath`, deterministic tests | See current verification; real-path proof required |
| Authenticated multi-path relay, source ownership | `internal/relay`, `internal/transport`, loopback integration | See current verification; remote NAT gate remains |
| VLESS parsing, strict unsupported errors, secret redaction | `internal/proxy` tests/fuzz | See current verification; real proxy compatibility remains |
| Current dependency pins and license inventory | lockfiles, THIRD_PARTY_NOTICES | Partial: Wintun prebuilt terms/hash and full release SBOM require review |
| Fresh Ubuntu install, enrollment, remote NAT egress | `deploy/relay`, external authenticated client | REQUIRES REAL ENVIRONMENT |
| Relay restart, token rotation, safe uninstall | owned network units, before/after snapshots | REQUIRES REAL ENVIRONMENT |
| Fresh Windows install and unprivileged desktop | `deploy/windows`, signed runtime checks, NSIS | REQUIRES REAL ENVIRONMENT |
| Owner-only local IPC and encrypted secrets | `internal/platform`, unrelated-user rejection/DPAPI checks | REQUIRES REAL ENVIRONMENT |
| Import relay and two VLESS nodes; independent tests | desktop, service and actual VLESS providers | REQUIRES REAL ENVIRONMENT |
| Two simultaneous paths carry one IPv4 tunnel | Docker two-VLESS/TUN/NAT smoke: both path counters observed | PARTIAL PASS Linux lab; Windows/public-IP proof remains |
| DNS through tunnel / no IPv6 leak | physical-interface packet captures | REQUIRES REAL ENVIRONMENT |
| Failure survives / path rejoins / responsive UI | kill one path during real transfer | REQUIRES REAL ENVIRONMENT |
| GUI restart leaves tunnel alive | real Windows GUI restart | REQUIRES REAL ENVIRONMENT |
| Service crash/reboot/adapter/sleep recovery | before/after owned route/DNS/IPv6 snapshots | REQUIRES REAL ENVIRONMENT |
| Disconnect/uninstall restores only owned state | real Windows and Ubuntu baselines | REQUIRES REAL ENVIRONMENT |
| 100 cycles without orphan/handle/goroutine growth | cycle evidence, owned process/handle counters | NOT RUN |
| Controlled ≥1.5× equal-cap aggregation | `LAB_ACCEPTANCE.md`, actual measured baselines | NOT RUN |
| Long transfer memory/CPU bounded | hours of raw resource samples | NOT RUN |
| No normal-log or diagnostic secret leakage | fixture tests plus real export review | Unit evidence separate; live export NOT RUN |
| Two-proxy Docker lab with real TUN/NAT | `lab/run.sh`, digest-pinned Ubuntu and verified sing-box | PASS basic external HTTPS smoke; egress proxy required, capped benchmarks NOT RUN |
| Vulnerability, static analysis, secret scan | actual tool outputs and finding disposition | See current verification; unrun tool is not a pass |
| Core build / --version / GUI+installer packaging | build scripts, Linux→Windows Wails and NSIS3.11 | PASS cross-build and unsigned NSIS creation; native Windows runtime NOT RUN |
| Signed production delivery | real publisher certificate and verified runtime provenance | NOT AVAILABLE; installer explicitly unsigned |

The MVP is **not release-accepted** until every requirement in this matrix has fresh evidence, including unavailable-platform checks. No benchmark numbers are asserted by this document.
