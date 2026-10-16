# Changelog

## 1.0.0 - 2026-10-16

First public release: versioned IPv4 multipath tunnel over QUIC, Linux relay (systemd or Docker), Windows service and desktop app with English/Polish interface, and one-command setup for both sides. See [acceptance status](docs/ACCEPTANCE.md) for what has been verified on real deployments.

### Added

- Release packages: `Conflux-windows-amd64.zip`, `conflux-relay-linux-amd64.tar.gz` and `SHA256SUMS`, built by CI from the tag.
- Relay container image `ghcr.io/filiprek/conflux-relay` in GitHub Packages, smoke-tested with TUN in CI.
- `conflux-relay --enrollment` prints the enrollment line again for an installed relay.
- The setup scripts install the verified release by default; `-FromSource` / `--from-source` builds from the repository.

### Fixed

- Relay: long-lived authenticated paths no longer occupy the 64 handshake slots, which capped the relay at 64 concurrent paths regardless of `max_sessions`.
- Relay identities are valid for 10 years instead of 1. Clients pin the certificate, so expiry silently broke every enrolled client after a year.
- Path throughput could become NaN on coarse clocks, after which every status response failed JSON encoding and the desktop showed the service as unavailable.
- Windows service status no longer blocks for up to 60 seconds while a connection attempt or path test is in progress.
- An oversized packet from Wintun no longer tears down the tunnel; it is dropped. Closing the adapter twice no longer signals a closed handle.
- A malformed datagram from the local SOCKS relay is skipped instead of failing the QUIC path.
- Diagnostics archives are deleted if their owner ACL cannot be applied.
- Line endings are pinned to LF, so relay shell scripts checked out on Windows still run on Ubuntu.

### Security

- The loopback SOCKS inbounds used for each VLESS path now require per-session random credentials, so other local processes and users cannot route traffic through your proxies.
- Desktop module dependencies updated (x/net 0.61.0, x/text 0.43.0, x/crypto 0.58.0) to clear known advisories.

### Changed

- One-command setup: `scripts/setup-windows.ps1` installs the toolchain, builds and installs or upgrades the Windows client with verified sing-box and Wintun; `scripts/setup-relay.sh` builds and installs or upgrades the Ubuntu relay.
- The desktop shows the service's reason when an operation fails (credentials, links and file paths are still redacted) and offers **Restore networking** when the service reports an incomplete network recovery.
- `scripts/dev.ps1` provides the `test`, `frontend`, `check`, `build` and `security` tasks on Windows without `make` or bash.
- The SOCKS UDP transport reuses one receive buffer per association instead of allocating 64 KiB per packet.
- The relay looks up sessions by tunnel address instead of scanning all sessions for every packet.
