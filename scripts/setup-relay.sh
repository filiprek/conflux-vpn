#!/usr/bin/env bash
# One-step Conflux relay setup for a fresh Ubuntu 24.04 x86_64 server:
#
#   curl -fsSL https://raw.githubusercontent.com/filiprek/conflux-vpn/main/scripts/setup-relay.sh | sudo bash
#   curl -fsSL https://raw.githubusercontent.com/filiprek/conflux-vpn/main/scripts/setup-relay.sh | sudo bash -s -- --public-host relay.example.org
#
# Builds the relay from source with a SHA-256-verified Go toolchain, installs it with
# deploy/relay/install-relay.sh and prints the secret enrollment once. Run it again to
# upgrade the relay binary in place; configuration and credentials are kept.
#
# Without --public-host, the server's public IPv4 is looked up via https://api.ipify.org.
set -euo pipefail

readonly REPOSITORY=https://github.com/filiprek/conflux-vpn.git
readonly GO_URL=https://go.dev/dl/go1.27.2.linux-amd64.tar.gz
readonly GO_SHA256=ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5

fail() { echo "error: $*" >&2; exit 1; }
step() { echo "==> $*" >&2; }

# Everything runs from main so that `curl | bash` reads the whole script first.
main() {
  local host='' branch=main
  while (($#)); do
    case $1 in
      --public-host) [[ $# -ge 2 ]] || fail 'missing --public-host value'; host=$2; shift 2 ;;
      --branch) [[ $# -ge 2 ]] || fail 'missing --branch value'; branch=$2; shift 2 ;;
      -h | --help) sed -n '2,11p' "$0" 2>/dev/null || true; exit 0 ;;
      *) fail "unknown argument $1 (usage: setup-relay.sh [--public-host HOST] [--branch BRANCH])" ;;
    esac
  done

  [[ $EUID == 0 ]] || fail 'run as root, e.g. with sudo'
  # shellcheck source=/dev/null
  source /etc/os-release
  [[ $ID == ubuntu && $VERSION_ID == 24.04 && $(uname -m) == x86_64 ]] || fail 'supported target: Ubuntu 24.04 x86_64'
  [[ -c /dev/net/tun ]] || fail '/dev/net/tun is missing; enable TUN for this VPS'

  step 'Installing build prerequisites'
  DEBIAN_FRONTEND=noninteractive apt-get update -qq </dev/null
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends git curl ca-certificates </dev/null

  work=$(mktemp -d /var/tmp/conflux-setup.XXXXXX)
  trap 'rm -rf -- "$work"' EXIT

  step 'Downloading and verifying Go 1.27.2'
  curl -fsSL --proto '=https' -o "$work/go.tgz" "$GO_URL"
  echo "$GO_SHA256  $work/go.tgz" | sha256sum -c --quiet - || fail 'Go archive SHA-256 mismatch'
  tar -C "$work" -xzf "$work/go.tgz"

  step "Fetching source ($branch)"
  git clone --quiet --depth 1 --branch "$branch" "$REPOSITORY" "$work/src" </dev/null

  step 'Building conflux-relay'
  local version commit
  version=$(git -C "$work/src" describe --tags --always)
  commit=$(git -C "$work/src" rev-parse HEAD)
  (cd "$work/src" && HOME=$work GOPATH=$work/gopath GOCACHE=$work/gocache CGO_ENABLED=0 \
    "$work/go/bin/go" build -trimpath \
    -ldflags "-X main.version=$version -X main.commit=$commit" \
    -o "$work/conflux-relay" ./cmd/conflux-relay) </dev/null
  "$work/conflux-relay" --version >&2

  if [[ -f /var/lib/conflux-install/owner && ! -f /var/lib/conflux-install/uninstalled ]]; then
    step 'Existing installation found: upgrading the relay binary'
    systemctl stop conflux-relay
    install -m 755 "$work/conflux-relay" /usr/local/bin/conflux-relay
    /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --check
    systemctl start conflux-relay
    systemctl is-active --quiet conflux-relay || fail 'relay did not restart; see journalctl -u conflux-relay'
    /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --health
    step 'Upgrade complete. Existing enrollments remain valid.'
    return
  fi

  if [[ -z $host ]]; then
    host=$(curl -4 -fsS --max-time 10 https://api.ipify.org </dev/null || true)
    [[ $host =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || fail 'could not detect the public IPv4; pass --public-host'
    step "Detected public IPv4 $host"
  fi

  step 'Installing the relay service'
  bash "$work/src/deploy/relay/install-relay.sh" --binary "$work/conflux-relay" --public-host "$host" </dev/null

  if command -v ufw >/dev/null && ufw status | grep -q '^Status: active'; then
    step 'ufw is active: allowing inbound UDP 4433'
    ufw allow 4433/udp >&2
  fi
  step 'Done. Import the conflux:// line above in the Conflux desktop app and keep it private.'
  step 'If your cloud provider has its own firewall, allow inbound UDP 4433 there as well.'
}

main "$@"
