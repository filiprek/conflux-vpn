[CmdletBinding()]
param([Parameter(Mandatory)][string]$Path,[Parameter(Mandatory)][ValidatePattern('^[A-Fa-f0-9]{64}$')][string]$SHA256,[switch]$RequireWireGuardSignature)
$ErrorActionPreference='Stop'
if ((Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash -ne $SHA256) { throw 'SHA-256 mismatch.' }
if ($RequireWireGuardSignature) {
 $sig=Get-AuthenticodeSignature -LiteralPath $Path
 if ($sig.Status -ne 'Valid' -or $sig.SignerCertificate.Subject -notmatch 'WireGuard') { throw 'Valid official WireGuard signature required.' }
}
Write-Output 'Artifact integrity checks passed. Hash provenance and license review remain the operator responsibility.'
