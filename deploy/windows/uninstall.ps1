[CmdletBinding()]
param([switch]$PurgeData)
$ErrorActionPreference = 'Stop'
$admin = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
if (!$admin.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Administrator required.' }
$install = Join-Path $env:ProgramFiles 'Conflux'
$data = Join-Path $env:ProgramData 'Conflux'
$sidPath = Join-Path $data 'owner-sid.txt'
if (!(Test-Path $sidPath)) { throw 'Ownership record missing; refusing cleanup.' }
$ownerSID = (Get-Content -LiteralPath $sidPath -Raw).Trim()
if ($ownerSID -notmatch '^S-1-(5-21|12-1)-[0-9-]+$') { throw 'Invalid ownership record.' }
$ctl = Join-Path $install 'confluxctl.exe'
$pipe = "Conflux-$ownerSID"
$service = Get-Service Conflux -ErrorAction SilentlyContinue
if ($service) {
 if ($service.Status -ne 'Running') { Start-Service Conflux }
 & $ctl --pipe $pipe disconnect
 if ($LASTEXITCODE) { throw 'Disconnect failed; service and files retained to avoid losing recovery state.' }
 & $ctl --pipe $pipe recover
 if ($LASTEXITCODE) { throw 'Recovery failed; do not delete state or reset unrelated networking.' }
 Stop-Service Conflux
 & sc.exe delete Conflux | Out-Null
 if ($LASTEXITCODE) { throw 'Service removal failed.' }
}
# Close only the application GUI installed at our exact protected path.
$guiPath = Join-Path $install 'Conflux.exe'
Get-Process -Name Conflux -ErrorAction SilentlyContinue | ForEach-Object {
 if ($_.Path -eq $guiPath) { Stop-Process -Id $_.Id -ErrorAction Stop }
}
# Service has joined owned workers and restored only its own network transaction.
foreach ($file in 'conflux-service.exe','confluxctl.exe','Conflux.exe','sing-box.exe','wintun.dll') {
 $path = Join-Path $install $file
 if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
}
foreach ($file in 'LICENSE','THIRD_PARTY_NOTICES.md') {
 $path=Join-Path $install $file
 if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
}
$licenses=Join-Path $install 'licenses'
if (Test-Path -LiteralPath $licenses) { Remove-Item -LiteralPath $licenses -Recurse -Force }
if ($PurgeData) { Remove-Item -LiteralPath $data -Recurse -Force }
Write-Output 'Service and binaries removed. Data preserved unless -PurgeData was specified. No unrelated routes or drivers were removed.'
