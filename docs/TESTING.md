# Testing and evidence

Use Go 1.27.2 and Node.js 24. Unit/integration tests run with temporary files and loopback sockets; administrative networking tests require dedicated targets. Always preserve the command's real exit status. [ACCEPTANCE.md](ACCEPTANCE.md) records unexecuted release gates; the current run's actual output belongs in [VERIFICATION.md](VERIFICATION.md) when provided by the maintainer.

```sh
make check                 # format, root Go tests/vet/race, shell syntax
make frontend              # npm ci, typecheck, lint, unit tests, production build
make build                 # Linux relay/bench and Windows core cross-build
make security              # pinned govulncheck and frontend high-severity audit
python3 scripts/scan-secrets.py
go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
go test ./internal/protocol -run '^$' -fuzz FuzzFrame -fuzztime 10s
```

Check actual fuzz target names with `rg '^func Fuzz' internal` if they change. Run each untrusted boundary's target separately: frame/control, enrollment, VLESS and IPC/config where present. A fuzz smoke is bounded exploration, not proof of safety. Root `go test ./...` excludes the nested Wails module; test it with `cd ui/desktop && go test ./...`. Build GUI on Windows using `scripts/build-windows.ps1` and package with NSIS 3.11. CI runs these jobs but a checked-in workflow is not evidence it has executed. Security findings must be reviewed; a failed audit is not a pass.

For fresh relay acceptance, use a disposable Ubuntu 24.04 VM with TUN, root and public UDP access. Install, run `--check` and authenticated `--health`, connect externally, send real IPv4 traffic through NAT, restart, uninstall, and compare nftables/sysctl state to the baseline. Test an unrelated existing firewall rule and interface to ensure they survive. Test idempotent install/start/stop and an induced partial failure. Keep console access and sanitized evidence.

For Windows, capture physical-interface routes/DNS/IPv6 and handle/process counts before starting. Install under an explicit desktop SID, check pipe access as the owner, SYSTEM/admin and an unrelated local user. Import two real UDP-capable VLESS paths, connect, confirm the relay public egress IP, and inspect physical-adapter packet capture for direct DNS/IPv6 leaks. Restart GUI, crash/restart service, sleep/resume and reboot while connected. Disconnect and compare exact owned route/DNS/IPv6 restoration. Repeat 100 cycles, recording handles, goroutines, RSS and orphan processes. A script that merely exits zero is not acceptance.

Controlled throughput requires a real lab: two independently capped paths with client/relay spare capacity, an actual common relay and the shared benchmark transport. Measure direct, A, B and A+B under identical duration/packet conditions; record CPU/RSS and per-path bytes. Equal 20 Mbps paths should materially exceed one capped path (target ≥1.5× in a controlled environment). Repeat with 15/90 ms skew, loss, path kill/rejoin, sing-box crash and relay restart. Record the measured environment and values; never insert illustrative numbers as results.

The digest-pinned isolated Docker lab in `lab/run.sh` has passed an actual two-VLESS/TUN/NAT/external-HTTPS smoke test. This environment required its existing HTTPS egress proxy/trust anchor; direct public-IP and controlled throughput proof remain unrun. See `lab/README.md` for exact prerequisites/commands. A smoke test does not complete capped aggregation, 100 Windows cycles or long soak. See [LAB_ACCEPTANCE.md](LAB_ACCEPTANCE.md) for the execution contract. A transport loopback benchmark cannot substitute for the full Wintun→VLESS→relay→Internet chain.

The benchmark's self-test is a real QUIC transport smoke with bounded in-memory packet devices; it does not exercise operating-system TUN/NAT or VLESS:

```sh
./dist/linux/conflux-bench --self-test --paths 2 --duration 8s --mode bidi
./dist/linux/conflux-bench --self-test --paths 2 --duration 8s --fail-after 2s
```

For the actual relay chain, run a controlled UDP echo receiver on a separate reachable IPv4 host (`conflux-bench --echo-listen :9000`), save the enrollment privately in an owner-only file, start the verified proxy listeners, then use:

```sh
./dist/linux/conflux-bench --relay-file /private/enrollment.txt \
  --target 198.51.100.20:9000 --socks 127.0.0.1:13001,127.0.0.1:13002 \
  --duration 30s --mode bidi
```

Replace the documentation address with the real controlled receiver. Protect the enrollment file and delete it when finished. `--bytes` limits total bytes; `--rate-mbps` controls the offered rate. Capture actual per-path bytes, return Mbps, reconnects, RTT and QUIC loss; QUIC packet loss is not a promise of end-to-end inner application loss. A public UDP echo listener is lab-only and should be firewall-restricted to the relay to avoid unwanted use.

Real Windows harnesses are supplied but **NOT RUN** in this Linux environment. Configure and independently test two paths first, then run from an elevated session using the installation owner's pipe:

```powershell
./scripts/accept-windows.ps1 -Pipe 'Conflux-S-1-5-21-...' -Cycles 100 `
  -ConnectProof -ExpectedRelayIPv4 '203.0.113.20'
./scripts/soak-windows.ps1 -Pipe 'Conflux-S-1-5-21-...' `
  -DownloadURL 'https://your-controlled-server.example/large-test-file' `
  -DurationSeconds 14400 -ExpectedRelayIPv4 '203.0.113.20'
```

Replace documentation addresses with the actual relay and your controlled HTTPS receiver. The cycle harness requires two healthy paths, compares IPv4 routes, unchanged physical DNS settings/IPv6 bindings, owned firewall rules and adapters to a disconnected baseline after each cycle, rejects remaining owned sing-box processes and records real service handles/RSS/threads. A monotonic rising disconnected-handle trend fails. Raw samples still require review; goroutine counts are explicitly unavailable through the current service API. A network change by DHCP or another application also fails baseline comparison and must be investigated rather than ignored.

The soak harness streams and discards actual HTTPS response bytes, records return Mbps and service/path/resource samples, disables system HTTP proxy and redirects, bounds each request and total duration, and verifies cleanup. It does not manufacture useful payload throughput from probe counters. Use a large test object you control, not a small third-party URL. Both harnesses produce structured local JSON, attempt disconnect/recover on failure and retain the protected service journal if recovery fails. Review outputs before sharing.

DNS/IPv6 leak acceptance needs physical-adapter packet capture, separate from these scripts. On an expendable Windows target with console access, start the built-in capture with `pktmon start --capture --pkt-size 0 --file-name <absolute-path.etl>` (check `pktmon help` for the installed Windows version), record interface/route context, connect and generate ordinary DNS plus IPv4/IPv6 application traffic, then `pktmon stop` and convert the capture with `pktmon etl2pcap`. In Wireshark, select physical adapters and inspect `dns`, `udp.port == 53`, `tcp.port == 53` and `ipv6`; encrypted VLESS underlay traffic is expected, direct inner DNS/IPv6 egress is not. Repeat after new-adapter attachment, service crash/restart, sleep/resume and reboot. Verify resolver `1.1.1.1/32` follows the TUN and temporary rules disappear on disconnect. Record the actual capture commands, interface identifiers and findings. Capture can contain sensitive traffic; keep it local/protected. No capture result is claimed here.
