# Windows installation and networking

Target Windows 11 x64, official WebView2 runtime and administrator rights for installation/service networking. The Wails GUI runs as the selected unprivileged desktop user. No signed public installer is provided. Build and runtime acceptance on real Windows remain release gates.

Install Go 1.27.2, Node.js 24/npm and the stable Wails v2.16.0 prerequisites from their official sources. From the checkout, in PowerShell:

```powershell
./scripts/build-windows.ps1
# NSIS 3.11, installed independently from its official distribution:
makensis deploy/windows/conflux.nsi
```

Output: core executables and `Conflux.exe` under `dist/windows`; unsigned wrapper `dist/Conflux-unsigned-setup.exe`. A Linux cross-build produces core Windows executables, not a native GUI installer. Do not equate cross-compilation with running the service/Wintun/network stack.

Supply sing-box 1.14.3 as a separate official executable. Verify the official archive's published checksum through a trusted channel, retain its license and matching source information, extract locally and compute the expected executable SHA-256. A locally computed hash alone does not authenticate an untrusted download. Supply the amd64 Wintun DLL from https://www.wintun.net/; review the prebuilt distribution license, independently authenticate its hash, and inspect `Get-AuthenticodeSignature`. Installation requires a valid signature whose subject identifies WireGuard. No runtime is downloaded or bundled automatically.

Before elevation, determine the intended desktop user's SID with `[Security.Principal.WindowsIdentity]::GetCurrent().User.Value`. If elevating as a different account, keep that original SID. Run the source installer as administrator (replace paths and both hash values):

```powershell
./deploy/windows/install.ps1 -OwnerSID 'S-1-5-21-...' `
  -SourceDirectory "$PWD/dist/windows" `
  -SingBox 'C:/verified/sing-box.exe' -SingBoxSHA256 '<64 hex characters>' `
  -Wintun 'C:/verified/wintun.dll' -WintunSHA256 '<64 hex characters>'
```

The NSIS wrapper takes equivalent `/OWNER_SID=`, `/SINGBOX=`, `/SINGBOX_SHA256=`, `/WINTUN=` and `/WINTUN_SHA256=` command-line options. Use full paths without embedded quote characters. The installer places privileged binaries in `%ProgramFiles%/Conflux` writable only by administrators/SYSTEM, gives users read/execute access, confines `%ProgramData%/Conflux` to administrators/SYSTEM, writes non-secret runtime config, and registers the LocalSystem service. It records the owner SID and uses pipe `Conflux-<SID>`. The GUI never edits protected files directly. Installation fails on existing service/data to preserve recoverable state.

The service must record previous network state atomically before mutation. It resolves proxy endpoints and adds owned physical-adapter bypass routes before redirecting default IPv4 traffic, sets the new TUN adapter's DNS to `1.1.1.1` and pins that resolver with an explicit `/32` tunnel route. Physical adapters' DNS configurations remain unchanged. A global DNS-port-53 firewall restriction permits only this resolver, covering newly appearing adapters, and global IPv6 block rules apply while active. Disconnect removes the owned resolver route, TUN and temporary firewall rules; physical DNS settings need no rewrite. This resolver choice is an operational default, not a privacy guarantee. Application-specific encrypted DNS is separate from ordinary port-53 DNS. Bypass routes carry a per-session metric token and undo matches the exact owned route. DNS, IPv6 and route restoration require real packet capture and restart/reboot testing. The GUI state alone does not prove complete protection. MTU defaults to 1100; IPv6 is blocked, not tunneled.

With the service running, launch `%ProgramFiles%/Conflux/Conflux.exe` as the owner, import relay enrollment privately, add at least two VLESS nodes, test each for real UDP/QUIC eligibility, select them and connect. Status is service-authoritative. A parsed URI or TCP handshake alone does not establish Bond eligibility.

Useful CLI commands (the owner's actual SID replaces the example):

```powershell
$ctl = "$env:ProgramFiles/Conflux/confluxctl.exe"
$pipe = 'Conflux-S-1-5-21-...'
& $ctl --pipe $pipe status
& $ctl --pipe $pipe disconnect
& $ctl --pipe $pipe recover
& $ctl --pipe $pipe diagnostics
```

Imports are read from stdin rather than command-line secrets; prefer GUI paste to avoid shell history. Consult `confluxctl --help` for exact payload shapes before scripted imports.

To uninstall, run elevated:

```powershell
& "$env:ProgramFiles/Conflux/uninstall.ps1"
# Add -PurgeData only to intentionally discard protected credentials/recovery state.
```

The script requires successful disconnect and recovery before stopping/deleting the service. Failure preserves files/state rather than hiding a broken network. If the service is stopped, it is started to recover. Default uninstall keeps data; no global Winsock reset, unrelated route deletion, firewall flush or driver-store deletion occurs. Close the GUI first. If installed by the NSIS wrapper, use its registered uninstaller for wrapper/registry cleanup too. Runtime failure, crash, sleep/resume and uninstall acceptance require an actual Windows administrator environment.
