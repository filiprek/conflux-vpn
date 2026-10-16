# Security policy

Security fixes are made on `main` and shipped in the next release; only the latest release is supported. Do not rely on Conflux for safety-critical traffic until the [acceptance matrix](docs/ACCEPTANCE.md) has passed on your own deployment.

Report a vulnerability privately at https://github.com/filiprek/conflux-vpn/security/advisories/new. Do not open a public issue with exploit details or credentials. Never attach a relay enrollment string, VLESS URI, unredacted configuration or decrypted storage. Share the commit, affected component, platform and a sanitized reproduction.

See [security architecture](docs/SECURITY.md), [threat model](docs/THREAT_MODEL.md), [dependency notices](THIRD_PARTY_NOTICES.md) and [acceptance status](docs/ACCEPTANCE.md). No code-signing identity or audited security certification is claimed.
