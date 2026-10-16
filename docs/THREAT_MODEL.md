# Threat model

Assets are proxy credentials, relay enrollment tokens, tunnel traffic, local networking integrity and availability. Trust boundaries are GUI→privileged pipe, service→child process, local host→untrusted proxy, proxy→relay and relay→Internet. Importing a VLESS link does not make its provider trusted.

| Observer | Can ordinarily observe |
|---|---|
| Local ISP/access network | Proxy or direct relay endpoints, timing, volume and transport metadata |
| VLESS provider | Client address, relay endpoint, timing and volume; QUIC tunnel payload remains encrypted |
| Relay operator | Client session, inner IP destinations, DNS and unencrypted application traffic; HTTPS application contents remain encrypted |
| Destination | Relay public IP, application requests and browser/account identity |
| Local administrator/kernel | Process memory, credentials and ability to alter binaries/networking |

| Threat | Mitigation / verification boundary |
|---|---|
| Compromised proxy, relay impersonation | End-to-end TLS verification, explicit anchor, pin and out-of-band enrollment; malicious-path and wrong-pin tests |
| Stolen enrollment | Secret storage, restricted relay files, token rotation and re-enrollment; bearer tokens grant access until rotated |
| Malicious local process / IPC message | Owner-SID ACL, local named pipe, message limits, strict methods, no public admin socket; test on real Windows |
| Malformed QUIC/frame/VLESS/config | Mature QUIC, strict bounded codecs and parser fuzzing; QUIC resource limits and session caps |
| Route hijack / loop | Endpoint bypass routes before default tunnel routing, ownership journal and rollback; physical-network changes require target acceptance |
| DNS / IPv6 leak | TUN DNS `1.1.1.1` with pinned route, temporary global port-53 restriction and IPv6 block; packet capture on physical adapters required |
| Log/diagnostic disclosure | Bounded structured logs, redaction and known-secret fixture scanning; operator reviews exports |
| Path/session/memory exhaustion | Bounded packet and byte queues, session caps, deadlines/backoff, reorder gap expiry; soak/chaos gates |
| Disk exhaustion | System journal rate limiting and host retention policy, bounded diagnostic/log retention |
| Command injection | Structured argument execution, strict import validation, no user-controlled shell fragments |
| Config tampering / runtime replacement | SYSTEM/admin-owned directories, exact runtime hashes, no arbitrary PATH executable lookup |
| Dependency compromise | Exact lockfiles, official sources, checksum/signature validation, notices and vulnerability review |

A malicious relay can observe or drop traffic; pinning authenticates identity rather than making the relay benevolent. VPN traffic correlation and browser fingerprinting remain possible. A single physical uplink remains a bottleneck and point of failure. No kill-switch guarantee beyond the implemented and tested connection lifecycle is claimed. Acceptance must establish behavior across partial activation, adapter change, sleep/resume, crash and reboot before release.
