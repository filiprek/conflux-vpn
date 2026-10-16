# Contributing

Start with the [architecture](docs/ARCHITECTURE.md), [protocol](docs/PROTOCOL.md) and [testing guide](docs/TESTING.md). Contributions are accepted under the project's Apache-2.0 license; keep upstream notices intact.

## Development setup

Use Go 1.27.2 and Node.js 24 with the committed npm lockfile.

- Linux/macOS: `make check`, `make frontend`, `make build`, `make security`.
- Windows (no `make` needed): `./scripts/dev.ps1 check`, `frontend`, `build`, `security` or `all`.

The Wails desktop is a separate Go module under `ui/desktop`; both helpers test it too. Line endings are normalized to LF by `.gitattributes`, so relay shell scripts stay runnable when checked out on Windows.

Test Windows networking only on an expendable Windows 11 machine with console access and administrator rights, never on a workstation whose routes you care about.

## Changes

- Add a test that fails for the intended reason before changing behavior. Prefer real loopback transports and deterministic clocks over mocks.
- Every network mutation needs an ownership check and a recovery path.
- Keep credentials out of source, logs, command-line arguments and issue attachments.
- Mark platform checks you could not execute as NOT RUN in [ACCEPTANCE.md](docs/ACCEPTANCE.md).

Pull requests should describe the problem, the new behavior, the commands you ran and known limitations. Dependency changes need exact versions, license and vulnerability review, and updated lockfiles and [third-party notices](THIRD_PARTY_NOTICES.md).
