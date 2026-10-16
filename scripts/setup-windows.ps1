<#
.SYNOPSIS
  One-step Conflux setup for Windows 11 x64: toolchain, source, build and service install.

.DESCRIPTION
  Run from an ordinary (non-elevated) PowerShell window as the user who will use Conflux:

    & ([scriptblock]::Create((irm https://raw.githubusercontent.com/filiprek/conflux-vpn/main/scripts/setup-windows.ps1)))

  The script
    1. installs Git (winget), Go 1.27.2 and Node.js 24 for the current user when missing;
       Go and Node archives are verified against pinned SHA-256 values;
    2. clones or fast-forwards the repository into -Directory;
    3. runs the frontend checks and builds the service, CLI and desktop GUI;
    4. downloads sing-box 1.14.3 and Wintun 0.14.1 from their official sources, verifies
       pinned SHA-256 values, and installs the Conflux service after one UAC prompt.
       An existing installation is upgraded in place and keeps its data.

  Run it again at any time to update. -BuildOnly stops after step 3.

.PARAMETER Directory
  Where the source checkout lives. Default: %USERPROFILE%\conflux-vpn.

.PARAMETER OwnerSID
  SID of the desktop user allowed to control the service. Default: the current user.
#>
[CmdletBinding()]
param(
  [string]$Directory = (Join-Path $HOME 'conflux-vpn'),
  [string]$Branch = 'main',
  [switch]$BuildOnly,
  [string]$OwnerSID,
  # Internal: the elevated second stage started by this script.
  [switch]$ElevatedInstall
)
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$Repository = 'https://github.com/filiprek/conflux-vpn.git'
$Pinned = @{
  Go      = @{ Version = 'go1.27.2'; Url = 'https://go.dev/dl/go1.27.2.windows-amd64.zip'; SHA256 = '1314008898bd40df77af4b014f777f08873dbdfbcd3d92308728ee03304fe04f' }
  Node    = @{ Version = 'v24.22.0'; Url = 'https://nodejs.org/dist/v24.22.0/node-v24.22.0-win-x64.zip'; SHA256 = 'f9683a19877e12a9b167305c8db71040d812876359de21f0d2e97c3f321f80c3' }
  SingBox = @{ Version = '1.14.3'; Url = 'https://github.com/SagerNet/sing-box/releases/download/v1.14.3/sing-box-1.14.3-windows-amd64.zip'; SHA256 = 'd8b86ef58936dba9230ec6a57ffa4243933f3c14ef4809828b6c553440ed1518' }
  Wintun  = @{ Version = '0.14.1'; Url = 'https://www.wintun.net/builds/wintun-0.14.1.zip'; SHA256 = '07c256185d6ee3652e09fa55c0b673e2624b565e02c4b9091c79ca7d2f24ef51' }
}

function Write-Step([string]$Message) { Write-Host "==> $Message" -ForegroundColor Cyan }

function Test-Administrator {
  $principal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
  return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-Native([string]$File, [string[]]$Arguments) {
  & $File @Arguments
  if ($LASTEXITCODE) { throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE" }
}

function Get-VerifiedArchive([hashtable]$Item, [string]$Destination) {
  $name = Split-Path $Item.Url -Leaf
  $archive = Join-Path $Destination $name
  New-Item -ItemType Directory -Force $Destination | Out-Null
  if (-not (Test-Path $archive) -or (Get-FileHash $archive -Algorithm SHA256).Hash -ne $Item.SHA256) {
    Write-Step "Downloading $name"
    Invoke-WebRequest -UseBasicParsing -Uri $Item.Url -OutFile $archive
  }
  if ((Get-FileHash $archive -Algorithm SHA256).Hash -ne $Item.SHA256) {
    Remove-Item $archive -Force
    throw "SHA-256 mismatch for $name; download rejected."
  }
  return $archive
}

function Add-UserPath([string]$Path) {
  $env:Path = "$Path;$env:Path"
  $user = [Environment]::GetEnvironmentVariable('Path', 'User')
  if (($user -split ';') -notcontains $Path) {
    [Environment]::SetEnvironmentVariable('Path', ($Path + ';' + $user).TrimEnd(';'), 'User')
  }
}

function Update-SessionPath {
  $env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')
}

function Install-Git {
  if (Get-Command git -ErrorAction SilentlyContinue) { return }
  Write-Step 'Installing Git'
  if (-not (Get-Command winget -ErrorAction SilentlyContinue)) { throw 'Git is missing and winget is unavailable. Install Git from https://git-scm.com/download/win and rerun.' }
  Invoke-Native winget @('install', '--id', 'Git.Git', '--exact', '--source', 'winget', '--silent', '--accept-package-agreements', '--accept-source-agreements')
  Update-SessionPath
  if (-not (Get-Command git -ErrorAction SilentlyContinue)) { throw 'Git installed but not on PATH yet. Open a new PowerShell window and rerun.' }
}

function Install-Go {
  if ((Get-Command go -ErrorAction SilentlyContinue) -and (& go env GOVERSION) -eq $Pinned.Go.Version) { return }
  $root = Join-Path $env:LOCALAPPDATA "Programs\$($Pinned.Go.Version)"
  if (-not (Test-Path (Join-Path $root 'go\bin\go.exe'))) {
    Write-Step "Installing Go $($Pinned.Go.Version) for the current user"
    $archive = Get-VerifiedArchive $Pinned.Go (Join-Path $env:TEMP 'conflux-setup')
    if (Test-Path $root) { Remove-Item $root -Recurse -Force }
    Expand-Archive $archive -DestinationPath $root
  }
  Add-UserPath (Join-Path $root 'go\bin')
}

function Install-Node {
  if ((Get-Command node -ErrorAction SilentlyContinue) -and (& node --version) -like 'v24.*') { return }
  $root = Join-Path $env:LOCALAPPDATA 'Programs'
  $home24 = Join-Path $root "node-$($Pinned.Node.Version)-win-x64"
  if (-not (Test-Path (Join-Path $home24 'node.exe'))) {
    Write-Step "Installing Node.js $($Pinned.Node.Version) for the current user"
    $archive = Get-VerifiedArchive $Pinned.Node (Join-Path $env:TEMP 'conflux-setup')
    Expand-Archive $archive -DestinationPath $root -Force
  }
  Add-UserPath $home24
}

function Update-Source {
  if (Test-Path (Join-Path $Directory '.git')) {
    Write-Step "Updating $Directory"
    Invoke-Native git @('-C', $Directory, 'pull', '--ff-only', 'origin', $Branch)
  } else {
    Write-Step "Cloning into $Directory"
    Invoke-Native git @('clone', '--branch', $Branch, $Repository, $Directory)
  }
}

function Get-Runtimes([string]$Destination) {
  $singArchive = Get-VerifiedArchive $Pinned.SingBox $Destination
  $wintunArchive = Get-VerifiedArchive $Pinned.Wintun $Destination
  $singDir = Join-Path $Destination 'sing-box'
  $wintunDir = Join-Path $Destination 'wintun'
  foreach ($dir in $singDir, $wintunDir) { if (Test-Path $dir) { Remove-Item $dir -Recurse -Force } }
  Expand-Archive $singArchive -DestinationPath $singDir
  Expand-Archive $wintunArchive -DestinationPath $wintunDir
  $singBox = Get-ChildItem $singDir -Recurse -Filter sing-box.exe | Select-Object -First 1
  $wintun = Join-Path $wintunDir 'wintun\bin\amd64\wintun.dll'
  if (-not $singBox -or -not (Test-Path $wintun)) { throw 'Unexpected runtime archive layout.' }
  return @{ SingBox = $singBox.FullName; Wintun = $wintun }
}

# Elevated stage: fresh install through deploy/windows/install.ps1, or in-place upgrade.
function Install-Service([string]$Sid) {
  $dist = Join-Path $Directory 'dist\windows'
  $runtime = Get-Runtimes (Join-Path $Directory '.runtime')
  $singHash = (Get-FileHash $runtime.SingBox -Algorithm SHA256).Hash.ToLowerInvariant()
  $wintunHash = (Get-FileHash $runtime.Wintun -Algorithm SHA256).Hash.ToLowerInvariant()
  $install = Join-Path $env:ProgramFiles 'Conflux'
  $data = Join-Path $env:ProgramData 'Conflux'
  $service = Get-Service Conflux -ErrorAction SilentlyContinue

  if ($service) {
    Write-Step 'Upgrading the existing Conflux service'
    $owner = (Get-Content (Join-Path $data 'owner-sid.txt') -Raw).Trim()
    if ($owner -ne $Sid) { throw "Conflux is installed for $owner, not $Sid. Uninstall it first or pass -OwnerSID $owner." }
    $wintunSignature = Get-AuthenticodeSignature -LiteralPath $runtime.Wintun
    if ($wintunSignature.Status -ne 'Valid' -or $wintunSignature.SignerCertificate.Subject -notmatch 'WireGuard') { throw 'Wintun requires a valid official WireGuard signature.' }
    if ($service.Status -eq 'Running') {
      & (Join-Path $install 'confluxctl.exe') --pipe "Conflux-$Sid" disconnect
      if ($LASTEXITCODE) { throw 'Disconnect failed; upgrade aborted with the current version left running.' }
      Stop-Service Conflux
    }
    Get-Process -Name Conflux -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq (Join-Path $install 'Conflux.exe') } | Stop-Process -Force
    foreach ($file in 'conflux-service.exe', 'confluxctl.exe', 'Conflux.exe', 'LICENSE', 'THIRD_PARTY_NOTICES.md') {
      Copy-Item -LiteralPath (Join-Path $dist $file) -Destination $install -Force
    }
    Copy-Item -Path (Join-Path $dist 'licenses\*') -Destination (Join-Path $install 'licenses') -Recurse -Force
    Copy-Item -LiteralPath (Join-Path $Directory 'deploy\windows\uninstall.ps1') -Destination $install -Force
    Copy-Item -LiteralPath $runtime.SingBox -Destination (Join-Path $install 'sing-box.exe') -Force
    Copy-Item -LiteralPath $runtime.Wintun -Destination (Join-Path $install 'wintun.dll') -Force
    $config = @{ version = 1; singBoxPath = (Join-Path $install 'sing-box.exe'); singBoxSHA256 = $singHash; singBoxVersion = $Pinned.SingBox.Version }
    [IO.File]::WriteAllText((Join-Path $data 'config.json'), ($config | ConvertTo-Json), (New-Object Text.UTF8Encoding($false)))
    Start-Service Conflux
  } else {
    Write-Step 'Installing the Conflux service'
    & (Join-Path $Directory 'deploy\windows\install.ps1') -OwnerSID $Sid -SourceDirectory $dist `
      -SingBox $runtime.SingBox -SingBoxSHA256 $singHash -Wintun $runtime.Wintun -WintunSHA256 $wintunHash
  }

  $shell = New-Object -ComObject WScript.Shell
  $shortcut = $shell.CreateShortcut((Join-Path $env:ProgramData 'Microsoft\Windows\Start Menu\Programs\Conflux.lnk'))
  $shortcut.TargetPath = Join-Path $install 'Conflux.exe'
  $shortcut.WorkingDirectory = $install
  $shortcut.Save()
}

if ($ElevatedInstall) {
  $log = Join-Path $Directory '.runtime\install.log'
  New-Item -ItemType Directory -Force (Split-Path $log) | Out-Null
  Start-Transcript -Path $log -Force | Out-Null
  try { Install-Service $OwnerSID; exit 0 }
  catch { Write-Host $_ -ForegroundColor Red; exit 1 }
  finally { Stop-Transcript | Out-Null }
}

if (-not [Environment]::Is64BitOperatingSystem) { throw 'Conflux requires 64-bit Windows.' }
if ([Environment]::OSVersion.Version.Build -lt 22000) { Write-Warning 'Conflux targets Windows 11; this Windows version is untested.' }
if (-not $OwnerSID) { $OwnerSID = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value }

Install-Git
Install-Go
Install-Node
Update-Source

Write-Step 'Building (frontend checks, service, CLI and desktop GUI)'
$dev = Join-Path $Directory 'scripts\dev.ps1'
& $dev frontend
& $dev build

if ($BuildOnly) {
  Write-Host "Build complete: $(Join-Path $Directory 'dist\windows')" -ForegroundColor Green
  return
}

if (Test-Administrator) {
  Install-Service $OwnerSID
} else {
  Write-Step 'Installing the service (approve the administrator prompt)'
  $arguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', "`"$(Join-Path $Directory 'scripts\setup-windows.ps1')`"",
    '-ElevatedInstall', '-Directory', "`"$Directory`"", '-OwnerSID', $OwnerSID)
  $process = Start-Process powershell.exe -Verb RunAs -ArgumentList $arguments -Wait -PassThru
  Get-Content (Join-Path $Directory '.runtime\install.log') -ErrorAction SilentlyContinue |
    Where-Object { $_ -notmatch '^\*{10,}|^(Windows PowerShell transcript|Start time|End time|Username|RunAs User|Configuration Name|Machine|Host Application|Process ID|PSVersion|PSEdition|PSCompatibleVersions|BuildVersion|CLRVersion|WSManStackVersion|PSRemotingProtocolVersion|SerializationVersion)' }
  if ($process.ExitCode) { throw "Service installation failed (see $(Join-Path $Directory '.runtime\install.log'))." }
}

$gui = Join-Path $env:ProgramFiles 'Conflux\Conflux.exe'
Write-Host ''
Write-Host 'Conflux is installed and the service is running.' -ForegroundColor Green
Write-Host "Next: open Conflux (Start menu), import your relay enrollment and at least two vless:// links, test each path, then Connect."
Start-Process $gui
