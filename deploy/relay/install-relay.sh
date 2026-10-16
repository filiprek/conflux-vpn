#!/usr/bin/env bash
set -euo pipefail
umask 077
fail() { echo "$*" >&2; exit 1; }
[[ $EUID == 0 ]] || fail 'Run as root on the target Ubuntu host.'
binary= host=
while (($#)); do
 case $1 in
  --binary) [[ $# -ge 2 ]] || fail 'Missing --binary value'; binary=$2; shift 2;;
  --public-host) [[ $# -ge 2 ]] || fail 'Missing --public-host value'; host=$2; shift 2;;
  *) fail 'Usage: install-relay.sh --binary /absolute/conflux-relay --public-host relay.example.org';;
 esac
done
[[ -n $binary && -x $binary && $binary == /* ]] || fail 'An explicit executable absolute --binary is required.'
[[ $host =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*$ && $host != localhost && $host != 127.* ]] || fail 'Supply an explicit public IPv4 address or DNS hostname.'
source /etc/os-release
[[ $ID == ubuntu && $VERSION_ID == 24.04 && $(uname -m) == x86_64 ]] || fail 'Supported target: Ubuntu 24.04 x86_64.'
[[ -c /dev/net/tun ]] || fail '/dev/net/tun is required; check VPS TUN support.'
assets=$(cd -- "$(dirname -- "$0")" && pwd)
state=/var/lib/conflux-install
if [[ -d $state && ! -f $state/owner ]]; then fail "Refusing existing unowned $state."; fi
if [[ -d /etc/conflux && ! -f $state/owner ]]; then fail 'Refusing existing unowned /etc/conflux.'; fi
if [[ -f $state/owner ]]; then
 [[ ! -f $state/uninstalled ]] || fail "Previous uninstall preserved data. Follow the purge/reinstall documentation."
 echo 'Already installed. Configuration and credentials preserved; use documented upgrade procedure.'
 /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --check
 systemctl is-active --quiet conflux-relay
 /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --health
 exit 0
fi
for path in /usr/local/bin/conflux-relay /usr/local/libexec/conflux /etc/systemd/system/conflux-relay.service /etc/systemd/system/conflux-network.service; do
 [[ ! -e $path ]] || fail "Refusing existing unowned $path"
done
ip link show conflux0 >/dev/null 2>&1 && fail 'Refusing existing conflux0.'
nft list table ip conflux >/dev/null 2>&1 && fail 'Refusing existing nftables table ip conflux.'
DEBIAN_FRONTEND=noninteractive apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends nftables iproute2 ca-certificates
iface=$(ip -4 route show default | awk '$1=="default" {for(i=1;i<=NF;i++) if($i=="dev") {print $(i+1); exit}}')
[[ $iface =~ ^[a-zA-Z0-9_.:-]+$ ]] || fail 'No usable IPv4 default egress interface.'
if id conflux >/dev/null 2>&1; then
 [[ $(getent passwd conflux | cut -d: -f6-7) == /var/lib/conflux:/usr/sbin/nologin && $(id -nG conflux) == conflux ]] || fail 'Existing conflux identity is not a dedicated non-login account.'
else
 useradd --system --home-dir /var/lib/conflux --shell /usr/sbin/nologin conflux
fi
install -d -m 700 "$state"
printf 'Conflux installer v1\n' > "$state/owner"
printf '%s\n' "$iface" > "$state/egress"
rollback() {
 rc=$?
 if ((rc)); then
  systemctl stop conflux-relay 2>/dev/null || true
  /usr/local/libexec/conflux/network.sh down 2>/dev/null || true
  echo 'Install failed; owned files retained for inspection. Run uninstall-relay.sh after resolving the error.' >&2
 fi
}
trap rollback EXIT
install -d -o conflux -g conflux -m 700 /etc/conflux
install -d -m 755 /usr/local/libexec/conflux
install -m 755 "$binary" /usr/local/bin/conflux-relay
install -m 700 "$assets/network.sh" /usr/local/libexec/conflux/network.sh
install -m 644 "$assets/conflux-relay.service" /etc/systemd/system/conflux-relay.service
install -m 644 "$assets/conflux-network.service" /etc/systemd/system/conflux-network.service
# Enrollment is held in memory and printed only after successful checks; never journaled.
enrollment=$(/usr/local/bin/conflux-relay --init "$host" --config /etc/conflux/relay.json)
chown -R conflux:conflux /etc/conflux
find /etc/conflux -type f -exec chmod 600 {} +
/usr/local/bin/conflux-relay --config /etc/conflux/relay.json --check
/usr/local/libexec/conflux/network.sh up
systemctl daemon-reload
systemctl enable --now conflux-network conflux-relay
systemctl is-active --quiet conflux-relay
ss -H -lun 'sport = :4433' | grep -q . || fail 'Relay did not bind UDP 4433.'
/usr/local/bin/conflux-relay --config /etc/conflux/relay.json --health
echo 'Local config/service/listener and authenticated QUIC ping checks passed. Remote connectivity and NAT still require client testing.'
echo 'Allow inbound UDP 4433 in your VPS firewall. Existing firewall policy may need an explicit forwarding allowance.'
printf '%s\n' "$enrollment"
