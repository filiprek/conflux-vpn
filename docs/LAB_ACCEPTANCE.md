# Controlled lab execution contract

Status: **PARTIAL PASS, 2026-10-15**. The isolated Docker two-VLESS→QUIC→real client/relay TUN→nftables NAT test executed successfully; see `lab/README.md` and the current verification report. This environment required an explicit egress HTTPS proxy and its existing trust anchor. Direct public-IP proof, controlled capped-throughput aggregation, Windows networking and long soak are separate unrun gates.

Use an expendable Ubuntu/WSL2/Docker host with `/dev/net/tun`, CAP_NET_ADMIN, netem/traffic-control support and a Windows 11 client. Obtain the exact official sing-box 1.14.3 executable with authenticated checksum and retained GPL notices. The runnable lab pins official Ubuntu24.04 digest `sha256:534baea6a22c03a63003dbc8dbe78fe34bc0d7e595d9a9dc9834884ff530eb55` and requires an expected sing-box executable hash; apt repository packages are resolved at build time, so it is not a bit-for-bit reproducible OS image. Unverified image tags or invented digests are unacceptable.

Topology: client→independent VLESS A/B→one Conflux relay→controlled IPv4 receiver. Keep the receiver and relay from bottlenecking the aggregate. Generate test-only UUIDs/tokens locally, bind management only to the lab and avoid publishing credentials.

Required scenarios:

| Scenario | Measurement and pass condition |
|---|---|
| Direct / single A / single B | Baselines from same client, receiver, bytes and duration |
| Equal caps | A and B independently 20 Mbps / 20 ms; both show real packet counters; A+B materially above one path, controlled target ≥1.5× |
| RTT skew | 50 Mbps/15 ms versus 50 Mbps/90 ms; bounded reorder/memory and useful delivery |
| Kill A during transfer | B continues, session remains recoverable, degraded state and A rejoin observed |
| Loss injection | Bound queue/reorder/RSS; report delivery and RTT rather than fake loss estimates |
| Relay restart | Clear disconnect/reconnect state and no stale sessions |
| sing-box crash | Owned child cleanup, bounded retries/circuit breaker and recovery |
| 100 connect/disconnect cycles | Exact route/DNS/IPv6 restoration each cycle, no orphan child, no monotonic handle/goroutine trend |
| Hours of transfer | Duration, interval samples, RSS/CPU/handles/reconnects/throughput, no monotonic memory growth |

Save the topology configuration, pinned provenance, exact commands, timestamps, raw counters and sanitized output alongside results. Each unexecuted scenario remains NOT RUN until its actual command and evidence exist; the basic smoke result above does not satisfy the capped/chaos/soak rows. Do not simulate Windows network success with an in-memory packet device.

Executed command (using the prepared Go1.27.2 PATH/GOPATH):

```sh
CONFLUX_SINGBOX_PATH=/tmp/sing-box-1.14.3-linux-amd64/sing-box \
CONFLUX_SINGBOX_SHA256=768d31fa7f68debc067bb4626539498a1e8751d36a7b99e1a54bee6a23815f93 \
./lab/run.sh
```

The supplied official sing-box hash above identifies the **Linux amd64 executable**, not a Windows archive or DLL. Use the corresponding verified path/hash for your environment. Actual result: assigned inner IPv4 `10.250.0.2`, HTTPS fetched 791 bytes matching SHA256 `650d5e3b99a446fb38e820fa87a49562e0c79eab868fff58618ac487a58e554c`; core sent1751 / received5003 bytes, path packet counters7/8 and8/7, and relay NAT counter one flow/60bytes. NAT-chain counters count the initial connection traversal, not the full payload. Both paths carried actual traffic. These small smoke-test counters are **not** an aggregation throughput benchmark. Public-IP echo endpoints returned403, so no direct public-IP result is asserted. No certificate verification was disabled.
