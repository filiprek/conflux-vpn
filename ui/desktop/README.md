# Conflux desktop

Unprivileged Windows 11 x64 Wails 2.16 application. The GUI uses the installed Conflux service through `\\.\pipe\Conflux-<current-user-SID>`. It verifies the pipe owner is LocalSystem before transmitting credentials. The service must create the pipe with explicit owner `O:SY` and grant its intended user access mask `0x120003` (read/write data, `READ_CONTROL`, synchronization; no create-instance permission). No TCP administration listener or browser fallback is provided.

## Build

Requires Go 1.27.2, Node.js 24, npm, and WebView2 on the target Windows machine. From this directory:

```sh
cd frontend
npm ci
npm test
npm run lint
npm run build
cd ..
go test -race .
go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 build -platform windows/amd64 -skipbindings
```

Wails packages the `asInvoker` manifest under `build/windows`; the GUI must not request elevation. Windows networking operations belong to the service. Wails generates Windows resources during packaging. Signing requires the distributor's real certificate.

For a Linux cross-compilation check after building frontend assets:

```sh
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -tags desktop,production -ldflags '-H windowsgui' -o conflux-desktop.exe .
```

This direct cross-build checks the Go/Windows API boundary. Use the Wails packaging command for Windows resources and the application manifest. Supply release version and commit with Go linker variables `main.version` and `main.commit`; `--version` writes version, commit, Go runtime and IPC protocol version to standard output.

For local frontend development run `npm run dev` under `frontend`. An ordinary browser intentionally shows Service unavailable. Advanced imports accept a single outbound JSON object; the service validates its strict supported schema. URI/JSON fields remain masked and summaries contain only endpoint details. Only language and appearance preferences use local storage; imported URIs live in component memory until successful submission and are never persisted there. In environments with an unwritable npm home, set `npm_config_cache=/tmp/conflux-npm`. Do not disable TLS, package integrity, or dependency checks.

## Service API

Requests and responses are newline-delimited JSON, protocol 1, maximum 64 KiB including the newline. Requests include `version`, unique `id`, `method` and object `params`; responses must echo `version` and `id`. The typed frontend boundary is `frontend/src/bridge.ts`. Unknown methods, malformed replies, mismatched IDs and versions, oversized frames and unverified pipe owners fail closed. Fixed bridge errors prevent accidental credential disclosure through service error strings. The UI translates operation guidance and shows sanitized path errors from status.

The implemented screens expose status, connection lifecycle, masked VLESS and advanced sing-box outbound JSON import, endpoint preview, path test/edit/delete/enable, relay enrollment with explicit fingerprint verification and handshake test, sanitized diagnostics ZIP export, EN/PL language, light/dark/system appearance, and service MTU configuration. MTU defaults to 1100, is limited to 576–1100, and must match the relay. The service is authoritative for actual health, counters and session state. Missing metrics use a dash; unavailable diagnostics use an explicit message. No bandwidth estimate or public tunnel IP is invented.

Settings unsupported by the service (login startup, auto-connect, log retention, profile selection and log level) are intentionally absent. Diagnostics versions, route snapshots and adapter fields are shown when supplied by the service. A successful path test confirms a relay handshake, not measured production bonding throughput.

## Windows acceptance still required

Run the installed GUI as a standard user and verify pipe owner checks, ACL denial for another user, absent service behavior, real import and connect/disconnect, actual counter changes, relay/path tests, readable sanitized ZIP export, WebView2 startup, keyboard navigation, DPI scaling and light/dark system changes. Restarting the GUI must leave a service-owned active tunnel intact. Linux cross-builds and Chromium frontend tests do not establish these Windows runtime outcomes.
