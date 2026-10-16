#!/bin/sh
set -eu
mkdir -p /dev/net
[ -e /dev/net/tun ] || mknod /dev/net/tun c 10 200
exec /usr/local/bin/conflux-lab-client
