# Troubleshooting

First check the authoritative service state and whether traffic is actually protected. Preserve the owned recovery journal; do not globally reset networking. Avoid posting full enrollment/VLESS URLs or protected JSON. Export local diagnostics, inspect redaction, then share only the needed sanitized facts.

| Symptom | Checks and safe action |
|---|---|
| Relay unreachable | Verify public hostname/IPv4, UDP 4433, cloud firewall, `systemctl status conflux-relay`, `ss -lun`, certificate expiry/name and out-of-band pin. TCP port tests are insufficient for QUIC. |
| VLESS import fails | Correct URI encoding, UUID, port and supported fields; do not silently discard unsupported security parameters. Never paste the full URI in tickets. |
| Path test fails / UDP unsupported | Confirm sing-box 1.14.3 verified hash, loopback SOCKS listener, UDP ASSOCIATE, remote proxy UDP support and QUIC handshake to relay. A working browser proxy is insufficient. |
| No aggregation | Require two eligible selected paths carrying counters; measure single baselines against capped independent paths with spare physical capacity. Unequal RTT/reordering can reduce throughput. |
| Route loop | Disconnect, inspect owned endpoint bypass routes and current physical adapter/gateway. Recover via service; avoid deleting arbitrary default routes. |
| DNS fails | Compare original recorded DNS, tunnel resolver reachability and physical-interface packet capture. Disconnect/recover and confirm restoration. |
| Wintun fails | Check x64 DLL, official signature/hash, service privileges, existing adapter and driver errors. Do not copy arbitrary DLLs from mirrors or delete unrelated drivers. |
| Windows service will not start | Check Service Control Manager events, non-secret config path, SYSTEM ACL, exact runtime hashes and owner SID. Preserve data; start it to run recovery before uninstall. |
| Relay active but no Internet | Inspect owned TUN/address, forwarding sysctl, nft table, and other firewall forward chains. An accept in one base chain does not override a drop elsewhere. |
| Installer failed | Review the concrete failed stage. Relay rollback retains owned files; Windows retains state on recovery failure. Resolve cause before removing ownership evidence. |

Windows recovery, from an authorized owner/admin session:

```powershell
$ctl = "$env:ProgramFiles/Conflux/confluxctl.exe"
$pipe = 'Conflux-' + [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& $ctl --pipe $pipe status
& $ctl --pipe $pipe disconnect
& $ctl --pipe $pipe recover
& $ctl --pipe $pipe diagnostics
```

If elevated as a different user, substitute the installation's desktop owner SID. A recovery error is a blocker: keep the service and journal, inspect its exact owned operations, and use console access. Do not run `netsh winsock reset` or remove all routes as a shortcut. CLI diagnostics are local output; inspect what the current command actually exports before assuming a ZIP exists.

Relay checks:

```sh
sudo /usr/local/bin/conflux-relay --config /etc/conflux/relay.json --check
sudo journalctl -u conflux-relay --since '10 minutes ago'
sudo nft list table ip conflux
ip address show conflux0
sysctl net.ipv4.ip_forward
```

A listener proves only binding; perform an authenticated client handshake and real egress request. If a path disappears, remaining-path traffic and recovery must be observed. Do not invent loss/throughput values when metrics are unavailable.
