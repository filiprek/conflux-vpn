# Ubuntu 24.04 relay deployment

Target: a dedicated Ubuntu 24.04 x86_64 VPS with root, systemd, `/dev/net/tun`, a public IPv4 address, UDP 4433 reachable and outbound IPv4. Use an expendable host with console access for first installation. The installer is not a verified production release: [acceptance](ACCEPTANCE.md) distinguishes code checks from privileged deployment.

Build the binary with Go 1.27.2 using `make build`, transfer `dist/linux/conflux-relay` plus the complete `deploy/relay` directory by your authenticated administrative channel, inspect the scripts, and run on the VPS:

```sh
sudo bash deploy/relay/install-relay.sh \
  --binary /absolute/path/to/conflux-relay \
  --public-host relay.example.org
```

Replace the hostname with your real public DNS name or IPv4 address. No domain, private key or server credential is included. Do not execute unreviewed scripts through curl-pipe-shell. There is no published release URL to download from in this repository.

The installer installs nftables/iproute2/CA certificates, creates a dedicated non-login `conflux` account, generates certificate/key/token through `--init`, restricts secret files to owner mode 0600, configures `conflux0` at `10.250.0.1/16` MTU 1100 and enables IPv4 forwarding. The relay runs as `conflux` with CAP_NET_ADMIN, restrictive systemd sandbox and `/dev/net/tun` device allowance. A separate root oneshot unit owns network setup/cleanup and boot persistence. It never replaces `/etc/nftables.conf` or unrelated rules.

Only `table ip conflux` is created; marker and install state protect ownership. Existing `conflux0`, unowned directories, binary or service files cause installation to stop. A repeat install preserves credentials and checks the existing service. Existing host firewall forward/input chains, cloud firewall and upstream network policy remain authoritative; explicitly allow UDP 4433 and the Conflux forwarding paths where required. An accept in Conflux's own chain cannot override a later drop in another chain. Do not flush the ruleset.

At successful first install the enrollment `conflux://...` is printed once, after local checks. Store it in a password manager and transfer it privately; it grants relay access. It is never put in systemd arguments. Local checks validate config, active service, UDP listener and an authenticated loopback QUIC handshake plus ping/pong. They do **not** prove authenticated remote connectivity, NAT or Internet traffic. Test the client from outside the VPS.

```sh
sudo /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --check
sudo /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --health
sudo systemctl status conflux-network conflux-relay
sudo journalctl -u conflux-relay --since '10 minutes ago'
sudo nft list table ip conflux
ip address show conflux0
sudo ss -lun 'sport = :4433'
```

Do not print `/etc/conflux/relay.json` into tickets: it contains the bearer token. Configure journald retention according to the host's disk budget; the service applies journal rate limiting. There is no public HTTP admin or health endpoint.

For upgrades, back up protected configuration privately, verify the new binary, stop the relay, replace only `/usr/local/bin/conflux-relay`, run `--check`, then start and perform an authenticated client smoke test. Keep a verified old binary for rollback. Installer repeat invocation intentionally does not replace credentials or silently upgrade.

For token rotation, disconnect clients, stop the relay, then run the protected rotation command. It replaces the token atomically and prints the new enrollment once; transfer it privately and restart. Existing clients must be re-enrolled. Do not use `--init` over an existing configuration.

```sh
sudo systemctl stop conflux-relay
sudo -u conflux /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --rotate-token
sudo systemctl start conflux-relay
sudo /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --health
```

```sh
sudo bash deploy/relay/uninstall-relay.sh
```

Uninstall stops and disables owned units, removes their binary/helper, the marked table and owned TUN, and restores forwarding only when its current value still equals the assigned value and a prior value was recorded. Concurrent forwarding users cannot be inferred; coordinate changes on shared hosts. The installer never changes persistent global sysctl files. Protected `/etc/conflux` credentials and `/var/lib/conflux-install` evidence remain by default. After backing up or intentionally discarding them, delete those exact directories to purge and permit fresh installation. The dedicated non-login user is retained to avoid deleting a shared/reused identity. If install rollback fails, inspect those owned records before reattempting; no global network reset is appropriate.
