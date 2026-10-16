#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
go_cmd=${GO:-go}
[[ $($go_cmd env GOVERSION) == go1.27.2 ]] || { echo 'Go 1.27.2 required; set GO to the pinned executable.' >&2; exit 1; }
version=$(git describe --tags --always --dirty 2>/dev/null || printf 'dev')
version=${version#v}
commit=$(git rev-parse HEAD 2>/dev/null || printf 'unknown')
ldflags="-X github.com/filiprek/conflux-vpn/internal/version.Version=$version -X github.com/filiprek/conflux-vpn/internal/version.Commit=$commit -X main.version=$version -X main.commit=$commit"
mkdir -p dist/linux dist/windows
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$go_cmd" build -trimpath -ldflags "$ldflags" -o dist/linux/conflux-relay ./cmd/conflux-relay
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 "$go_cmd" build -trimpath -ldflags "$ldflags" -o dist/linux/conflux-bench ./cmd/conflux-bench
for cmd in conflux-service confluxctl conflux-bench; do
 CGO_ENABLED=0 GOOS=windows GOARCH=amd64 "$go_cmd" build -trimpath -ldflags "$ldflags" -o "dist/windows/$cmd.exe" "./cmd/$cmd"
done

cp LICENSE THIRD_PARTY_NOTICES.md dist/windows/
mkdir -p dist/windows/licenses
cp docs/licenses/* dist/windows/licenses/
