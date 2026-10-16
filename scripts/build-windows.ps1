[CmdletBinding()]
param()
$ErrorActionPreference = 'Stop'
Set-Location (Join-Path $PSScriptRoot '..')
if ((go env GOVERSION) -ne 'go1.27.2') { throw 'Go 1.27.2 required.' }
New-Item -ItemType Directory -Force dist/windows | Out-Null
$revision = & git rev-parse HEAD
if ($LASTEXITCODE) { throw 'Git revision unavailable.' }
$version = & git describe --tags --always --dirty
if ($LASTEXITCODE) { throw 'Git version unavailable.' }
$ldflags = "-X github.com/filiprek/conflux-vpn/internal/version.Version=$version -X github.com/filiprek/conflux-vpn/internal/version.Commit=$revision -X main.version=$version -X main.commit=$revision"
$env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
foreach ($cmd in 'conflux-service','confluxctl','conflux-bench') {
 & go build -trimpath -ldflags $ldflags -o "dist/windows/$cmd.exe" "./cmd/$cmd"
 if ($LASTEXITCODE) { throw "Build failed: $cmd" }
}
Push-Location ui/desktop
try {
 & go run github.com/wailsapp/wails/v2/cmd/wails@v2.16.0 build -platform windows/amd64 -skipbindings -trimpath -ldflags "-X main.version=$version -X main.commit=$revision" -o Conflux.exe
 if ($LASTEXITCODE) { throw 'Wails build failed.' }
 Copy-Item build/bin/Conflux.exe ../../dist/windows/Conflux.exe -Force
} finally { Pop-Location }
Copy-Item -LiteralPath @('LICENSE','THIRD_PARTY_NOTICES.md') -Destination dist/windows -Force
Copy-Item docs/licenses dist/windows/licenses -Recurse -Force
Write-Output 'Application build complete. Run makensis deploy/windows/conflux.nsi for the unsigned installer.'
