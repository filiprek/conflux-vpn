[CmdletBinding()]
param(
 [Parameter(Mandatory)][ValidatePattern('^Conflux-S-1-(5-21|12-1)-[0-9-]+$')][string]$Pipe,
 [ValidateRange(1,1000)][int]$Cycles=100,
 [string]$ControlPath=(Join-Path $env:ProgramFiles 'Conflux/confluxctl.exe'),
 [string]$EvidenceDirectory=(Join-Path $PWD ('conflux-acceptance-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ'))),
 [switch]$ConnectProof,
 [string]$ExpectedRelayIPv4,
 [uri]$PublicIPEndpoint='https://api.ipify.org'
)
. (Join-Path $PSScriptRoot 'windows-acceptance-common.ps1')
Assert-ConfluxAdministrator
$script:Pipe=$Pipe; $script:ControlPath=(Resolve-Path -LiteralPath $ControlPath).Path
$script:EvidenceDirectory=[IO.Path]::GetFullPath($EvidenceDirectory)
New-Item -ItemType Directory -Path $script:EvidenceDirectory -ErrorAction Stop | Out-Null
if ($ConnectProof) {
 $address=$null
 if (![Net.IPAddress]::TryParse($ExpectedRelayIPv4,[ref]$address) -or $address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork) { throw '-ConnectProof requires -ExpectedRelayIPv4 with the real public relay IPv4.' }
}
$record=[ordered]@{schema=1;started=[DateTime]::UtcNow.ToString('o');platform=[Environment]::OSVersion.VersionString;requestedCycles=$Cycles;status='RUNNING';cycles=@();leakCapture='NOT RUN: operator physical-interface capture required';goroutineCheck='NOT AVAILABLE';error=$null;cleanup=$null}
$http=$null
try {
 $initial=Invoke-ConfluxControl status
 if ($initial.state -ne 'disconnected') { throw 'Start from a disconnected, recovered service.' }
 $baseline=Get-ConfluxNetworkSnapshot
 $record.baseline=$baseline
 if ($baseline.ownedFirewall.Count -ne 0 -or $baseline.ownedAdapters.Count -ne 0) { throw 'Stale Conflux network state detected; recover before acceptance.' }
 $first=Get-ConfluxResourceSample
 if ($first.ownedProxyCount -ne 0) { throw 'Proxy child exists while disconnected.' }
 $record.initialResources=$first
 if ($ConnectProof) { $http=New-ConfluxHttpClient }
 for ($cycle=1; $cycle -le $Cycles; $cycle++) {
  $entry=[ordered]@{cycle=$cycle;started=[DateTime]::UtcNow.ToString('o');status='RUNNING';connectedResources=$null;disconnectedResources=$null;egressIPv4=$null;networkRestored=$false}
  $record.cycles+=,$entry
  $connected=Invoke-ConfluxControl connect
  if ($connected.state -ne 'connected') { throw "Cycle ${cycle}: service did not confirm connected." }
  $healthy=@($connected.paths | Where-Object {$_.state -eq 'healthy' -or $_.state -eq 'Healthy'})
  if ($healthy.Count -lt 2) { throw "Cycle ${cycle}: fewer than two healthy paths." }
  $entry.connectedResources=Get-ConfluxResourceSample
  if ($entry.connectedResources.orphanProxyCount -ne 0) { throw "Cycle ${cycle}: orphan proxy detected." }
  if ($ConnectProof) { $entry.egressIPv4=Assert-ConfluxEgress $http $PublicIPEndpoint $ExpectedRelayIPv4 }
  $disconnected=Invoke-ConfluxControl disconnect
  if ($disconnected.state -ne 'disconnected') { throw "Cycle ${cycle}: disconnect incomplete." }
  $after=Get-ConfluxNetworkSnapshot
  if (($baseline | ConvertTo-Json -Depth 8 -Compress) -ne ($after | ConvertTo-Json -Depth 8 -Compress)) {
   $entry.networkAfter=$after
   throw "Cycle ${cycle}: routes/DNS/owned firewall/adapters/IPv6 bindings differ from baseline. Network drift also causes failure; inspect evidence."
  }
  $entry.networkRestored=$true
  $entry.disconnectedResources=Get-ConfluxResourceSample
  if ($entry.disconnectedResources.ownedProxyCount -ne 0) { throw "Cycle ${cycle}: owned proxy remains after disconnect." }
  if ($entry.disconnectedResources.servicePID -ne $first.servicePID) { throw 'Service restarted during the cycle test; handle trend is not comparable.' }
  $entry.status='PASS'
  Save-ConfluxEvidence $record 'cycles.json'
 }
 $counts=@($record.cycles | ForEach-Object {$_.disconnectedResources.handles})
 $strictlyIncreasing=$counts.Count -ge 3 -and $counts[-1] -gt $counts[0]
 for ($i=1; $i -lt $counts.Count; $i++) { if ($counts[$i] -lt $counts[$i-1]) { $strictlyIncreasing=$false } }
 $record.handleEvidence=[ordered]@{initial=$first.handles;minimum=($counts | Measure-Object -Minimum).Minimum;maximum=($counts | Measure-Object -Maximum).Maximum;final=$counts[-1];strictlyIncreasing=$strictlyIncreasing;assessment='Raw samples retained; broader leak/trend analysis still requires review.'}
 if ($strictlyIncreasing) { throw 'Disconnected handle counts showed a monotonic rising trend; investigate possible leak.' }
 $record.status='PASS_CYCLES_ONLY'
} catch {
 $record.status='FAIL';$record.error=$_.Exception.Message
 try { $null=Invoke-ConfluxControl disconnect; $null=Invoke-ConfluxControl recover; $record.cleanup='disconnect/recover succeeded; compare saved baseline' } catch { $record.cleanup='FAILED: preserve service and protected journal for recovery' }
} finally {
 if ($http) { $http.Dispose() }
 $record.finished=[DateTime]::UtcNow.ToString('o')
 Save-ConfluxEvidence $record 'cycles.json'
}
Write-Output (Join-Path $script:EvidenceDirectory 'cycles.json')
if ($record.status -eq 'FAIL') { exit 1 }
