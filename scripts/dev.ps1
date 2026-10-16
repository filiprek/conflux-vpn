# Windows equivalent of the Makefile targets, for machines without make/bash.
#   ./scripts/dev.ps1 test       Go unit and integration tests (root and desktop modules)
#   ./scripts/dev.ps1 frontend   npm ci, typecheck, lint, unit tests, production build
#   ./scripts/dev.ps1 check      gofmt, vet, tests and PowerShell syntax
#   ./scripts/dev.ps1 build      core binaries and the desktop GUI (scripts/build-windows.ps1)
#   ./scripts/dev.ps1 security   govulncheck and npm audit
#   ./scripts/dev.ps1 all        frontend, check and build
[CmdletBinding()]
param(
  [Parameter(Position = 0)]
  [ValidateSet('test', 'frontend', 'check', 'build', 'security', 'all')]
  [string]$Task = 'test'
)
$ErrorActionPreference = 'Stop'
$root = Resolve-Path (Join-Path $PSScriptRoot '..')
Set-Location $root

function Invoke-Native([string]$File, [string[]]$Arguments) {
  & $File @Arguments
  if ($LASTEXITCODE) { throw "$File $($Arguments -join ' ') failed with exit code $LASTEXITCODE" }
}

function Assert-Tool([string]$Name, [string]$Hint) {
  if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) { throw "$Name not found. $Hint" }
}

function Assert-Go {
  Assert-Tool go 'Install Go 1.27.2 from https://go.dev/dl/ and reopen the terminal.'
  $actual = & go env GOVERSION
  if ($actual -ne 'go1.27.2') { throw "Go 1.27.2 required, found $actual." }
}

# The desktop module embeds frontend/dist, so Go tooling for it needs a built frontend.
function Assert-FrontendBuilt {
  if (-not (Test-Path 'ui/desktop/frontend/dist/index.html')) { Invoke-Frontend }
}

function Invoke-Test {
  Assert-Go
  Invoke-Native go @('test', './...')
  Assert-FrontendBuilt
  Push-Location ui/desktop
  try { Invoke-Native go @('test', './...') } finally { Pop-Location }
}

function Invoke-Frontend {
  Assert-Tool npm 'Install Node.js 24 from https://nodejs.org/ and reopen the terminal.'
  Push-Location ui/desktop/frontend
  try {
    foreach ($step in @(@('ci'), @('run', 'typecheck'), @('run', 'lint'), @('test'), @('run', 'build'))) {
      Invoke-Native npm $step
    }
  } finally { Pop-Location }
}

function Invoke-Check {
  Assert-Go
  $gofmt = Join-Path (& go env GOROOT) 'bin/gofmt.exe'
  $sources = Get-ChildItem cmd, internal, tests, lab, ui/desktop -Recurse -Filter *.go |
    Where-Object { $_.FullName -notmatch '[\\/](node_modules|wailsjs)[\\/]' }
  $unformatted = & $gofmt -l $sources.FullName
  if ($unformatted) { throw "gofmt required:`n$($unformatted -join "`n")" }
  Invoke-Native go @('vet', './...')
  $env:GOOS = 'linux'
  try { Invoke-Native go @('vet', './...') } finally { Remove-Item Env:GOOS }
  Invoke-Test
  Push-Location ui/desktop
  try { Invoke-Native go @('vet', './...') } finally { Pop-Location }
  $failed = $false
  Get-ChildItem scripts, deploy/windows -Filter *.ps1 | ForEach-Object {
    $tokens = $null; $errors = $null
    $null = [System.Management.Automation.Language.Parser]::ParseFile($_.FullName, [ref]$tokens, [ref]$errors)
    if ($errors.Count) { $failed = $true; $errors | ForEach-Object { Write-Warning "$($_.Extent.File): $($_.Message)" } }
  }
  if ($failed) { throw 'PowerShell syntax errors found.' }
}

function Invoke-Build {
  Assert-Go
  Assert-FrontendBuilt
  & (Join-Path $PSScriptRoot 'build-windows.ps1')
}

function Invoke-Security {
  Assert-Go
  $tools = Join-Path $root '.tools'
  $env:GOBIN = $tools
  try { Invoke-Native go @('install', 'golang.org/x/vuln/cmd/govulncheck@v1.9.0') } finally { Remove-Item Env:GOBIN }
  $govulncheck = Join-Path $tools 'govulncheck.exe'
  Invoke-Native $govulncheck @('./...')
  Push-Location ui/desktop
  try { Invoke-Native $govulncheck @('-tags', 'production', './...') } finally { Pop-Location }
  Push-Location ui/desktop/frontend
  try { Invoke-Native npm @('audit', '--audit-level=high') } finally { Pop-Location }
}

switch ($Task) {
  'test' { Invoke-Test }
  'frontend' { Invoke-Frontend }
  'check' { Invoke-Check }
  'build' { Invoke-Build }
  'security' { Invoke-Security }
  'all' { Invoke-Frontend; Invoke-Check; Invoke-Build }
}
Write-Output "Task '$Task' completed."
