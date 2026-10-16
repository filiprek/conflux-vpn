# Contributing

Read the [architecture](docs/ARCHITECTURE.md) and [testing guide](docs/TESTING.md). Contributions remain under the project's Apache-2.0 license; preserve upstream notices.

Use Go 1.27.2 and Node.js 24 with the committed npm lockfile. Run `make check`, `make frontend`, `make security` and the checks relevant to your change. Wails uses its own module under `ui/desktop`; test it separately. Test Windows network behavior in an expendable Windows 11 machine with console access and real administrator privileges. Never use a developer workstation's unrelated routes as disposable test data.

For behavioral changes, first add and run a test which fails for the intended reason. Prefer real loopback transport and deterministic clocks; fake metrics and broad cleanup commands are unacceptable. Every network mutation needs ownership evidence and recovery. Document unexecuted platform tests as NOT RUN. Keep credentials out of source, logs, command-line arguments and issue attachments.

Submit focused pull requests describing the problem, resulting behavior, commands executed and limitations. Do not mark the MVP complete based on compilation. Dependency changes require exact versions, authoritative license review, vulnerability review and updated lockfiles/notices. Release artifacts require review of signatures, checksums, runtime redistribution obligations and the full acceptance matrix.
