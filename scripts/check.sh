#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")/.."
go_cmd=${GO:-go}
# Wails generated output and dependencies are excluded from source formatting checks.
gofmt_cmd="$($go_cmd env GOROOT)/bin/gofmt"
unformatted=$(find cmd internal tests ui/desktop -name '*.go' -not -path '*/node_modules/*' -not -path '*/wailsjs/*' -print0 | xargs -0 "$gofmt_cmd" -l)
[[ -z $unformatted ]] || { printf '%s\n' "$unformatted" >&2; exit 1; }
"$go_cmd" test ./...
"$go_cmd" vet ./...
"$go_cmd" test -race ./...
for script in scripts/*.sh deploy/relay/*.sh; do bash -n "$script"; done
