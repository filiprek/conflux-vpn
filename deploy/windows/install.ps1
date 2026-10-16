# Run elevated, with the SID of the desktop user (not an unrelated elevated account).
[CmdletBinding()]
param(
 [Parameter(Mandatory)][ValidatePattern('^S-1-(5-21|12-1)-[0-9-]+$')][string]$OwnerSID,
 [Parameter(Mandatory)][string]$SourceDirectory,
 [Parameter(Mandatory)][string]$SingBox,
 [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$SingBoxSHA256,
 [Parameter(Mandatory)][string]$Wintun,
 [Parameter(Mandatory)][ValidatePattern('^[0-9a-fA-F]{64}$')][string]$WintunSHA256
)
$ErrorActionPreference = 'Stop'
$admin = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (!$admin.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator required.' }
$null = ([Security.Principal.SecurityIdentifier]$OwnerSID).Translate([Security.Principal.NTAccount])
if (Get-Service Conflux -ErrorAction SilentlyContinue) { throw 'Service already exists; uninstall or follow the upgrade guide.' }
$install = Join-Path $env:ProgramFiles 'Conflux'
$data = Join-Path $env:ProgramData 'Conflux'
if (Test-Path $data) { throw 'Existing data directory: preserve and recover it before reinstalling.' }
function Assert-Hash($Path, $Expected) {
 if (!(Test-Path -LiteralPath $Path -PathType Leaf)) { throw "Missing runtime: $Path" }
 if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $Expected) { throw 'Runtime SHA-256 mismatch.' }
}
Assert-Hash $SingBox $SingBoxSHA256
Assert-Hash $Wintun $WintunSHA256
$signature = Get-AuthenticodeSignature -LiteralPath $Wintun
if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'WireGuard') { throw 'Wintun requires a valid official WireGuard signature.' }
$version = & $SingBox version
if ($LASTEXITCODE -ne 0 -or ($version -join "`n") -notmatch 'sing-box version 1\.14\.3(\s|$)') { throw 'Requires verified sing-box 1.14.3.' }
foreach ($file in 'conflux-service.exe','confluxctl.exe','Conflux.exe','LICENSE','THIRD_PARTY_NOTICES.md') {
 if (!(Test-Path -LiteralPath (Join-Path $SourceDirectory $file))) { throw "Missing built artifact: $file" }
}
New-Item -ItemType Directory -Force $install,$data | Out-Null
# Privileged service binaries must never be writable by the desktop owner.
& icacls.exe $install /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-32-545:(OI)(CI)RX' | Out-Null
if ($LASTEXITCODE) { throw 'Install ACL failed.' }
& icacls.exe $data /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' | Out-Null
if ($LASTEXITCODE) { throw 'Data ACL failed.' }
foreach ($file in 'conflux-service.exe','confluxctl.exe','Conflux.exe') { Copy-Item -LiteralPath (Join-Path $SourceDirectory $file) -Destination $install -Force }
Copy-Item -LiteralPath (Join-Path $SourceDirectory 'LICENSE') -Destination $install -Force
Copy-Item -LiteralPath (Join-Path $SourceDirectory 'THIRD_PARTY_NOTICES.md') -Destination $install -Force
Copy-Item -LiteralPath (Join-Path $SourceDirectory 'licenses') -Destination (Join-Path $install 'licenses') -Recurse -Force
Copy-Item -LiteralPath $SingBox -Destination (Join-Path $install 'sing-box.exe') -Force
Copy-Item -LiteralPath $Wintun -Destination (Join-Path $install 'wintun.dll') -Force
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'uninstall.ps1') -Destination $install -Force
$config = @{version=1;singBoxPath=(Join-Path $install 'sing-box.exe');singBoxSHA256=$SingBoxSHA256.ToLowerInvariant();singBoxVersion='1.14.3'}
$configPath = Join-Path $data 'config.json'
[IO.File]::WriteAllText($configPath, ($config | ConvertTo-Json), (New-Object Text.UTF8Encoding($false)))
[IO.File]::WriteAllText((Join-Path $data 'owner-sid.txt'), $OwnerSID)
$pipe = "Conflux-$OwnerSID"
$binaryPath = '"' + (Join-Path $install 'conflux-service.exe') + '" --config "' + $configPath + '" --owner-sid ' + $OwnerSID + ' --pipe ' + $pipe
& sc.exe create Conflux binPath= $binaryPath start= auto obj= LocalSystem DisplayName= 'Conflux VPN Service' | Out-Null
if ($LASTEXITCODE) { throw 'Service registration failed; protected files retained for inspection.' }
& sc.exe failure Conflux reset= 86400 actions= restart/5000/restart/15000 | Out-Null
if ($LASTEXITCODE) { throw 'Service recovery configuration failed.' }
Start-Service Conflux
if ((Get-Service Conflux).Status -ne 'Running') { throw 'Service did not start.' }
Write-Output "Installed for $OwnerSID. Launch $install\Conflux.exe as that user. Runtime network acceptance still required."
