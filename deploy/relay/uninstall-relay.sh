#!/usr/bin/env bash
set -euo pipefail
[[ $EUID == 0 ]] || { echo 'Run as root.' >&2; exit 1; }
state=/var/lib/conflux-install
[[ -f $state/owner ]] || { echo 'No owned installation found; nothing removed.'; exit 0; }
[[ ! -f $state/uninstalled ]] || { echo "Already uninstalled; protected data remains."; exit 0; }
systemctl disable --now conflux-relay conflux-network
/usr/local/libexec/conflux/network.sh down
rm -f /etc/systemd/system/conflux-relay.service /etc/systemd/system/conflux-network.service /usr/local/bin/conflux-relay
rm -f /usr/local/libexec/conflux/network.sh
rmdir /usr/local/libexec/conflux
systemctl daemon-reload
touch "$state/uninstalled"
# Preserve credentials and ownership evidence until the operator explicitly purges them.
echo 'Service, binary, owned nftables table and TUN removed. Restored forwarding only where still owned.'
echo 'Protected credentials remain in /etc/conflux; installation records remain in /var/lib/conflux-install.'
echo 'After backups, remove those exact directories to purge secrets and permit a fresh installation.'
