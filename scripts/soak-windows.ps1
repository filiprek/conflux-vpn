[CmdletBinding()]
param(
 [Parameter(Mandatory)][ValidatePattern('^Conflux-S-1-(5-21|12-1)-[0-9-]+$')][string]$Pipe,
 [Parameter(Mandatory)][uri]$DownloadURL,
 [ValidateRange(10,86400)][int]$DurationSeconds=3600,
 [ValidateRange(1,60)][int]$SampleSeconds=10,
 [string]$ControlPath=(Join-Path $env:ProgramFiles 'Conflux/confluxctl.exe'),
 [string]$EvidenceDirectory=(Join-Path $PWD ('conflux-soak-' + [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssZ'))),
 [string]$ExpectedRelayIPv4,
 [uri]$PublicIPEndpoint='https://api.ipify.org'
)
. (Join-Path $PSScriptRoot 'windows-acceptance-common.ps1')
Assert-ConfluxAdministrator
if ($DownloadURL.Scheme -ne 'https' -or $DownloadURL.UserInfo) { throw 'Use a controlled HTTPS download URL without embedded credentials.' }
$script:Pipe=$Pipe;$script:ControlPath=(Resolve-Path -LiteralPath $ControlPath).Path
$script:EvidenceDirectory=[IO.Path]::GetFullPath($EvidenceDirectory)
New-Item -ItemType Directory -Path $script:EvidenceDirectory -ErrorAction Stop | Out-Null
$record=[ordered]@{schema=1;started=[DateTime]::UtcNow.ToString('o');status='RUNNING';durationSeconds=$DurationSeconds;downloadHost=$DownloadURL.Host;receivedBytes=[long]0;requests=0;samples=@();error=$null;cleanup=$null;goroutines='NOT AVAILABLE';leakCapture='NOT RUN: physical adapter capture required';resourceAssessment='Raw RSS/handles/threads retained. No automatic long-term leak-free claim.'}
$http=$null;$clock=[Diagnostics.Stopwatch]::StartNew();$baseline=$null
try {
 $initial=Invoke-ConfluxControl status
 if ($initial.state -ne 'disconnected') { throw 'Start from a disconnected, recovered service.' }
 $baseline=Get-ConfluxNetworkSnapshot
 if ($baseline.ownedFirewall.Count -or $baseline.ownedAdapters.Count) { throw 'Stale Conflux network state must be recovered first.' }
 $record.baseline=$baseline
 $initialResources=Get-ConfluxResourceSample
 if ($initialResources.ownedProxyCount) { throw 'Proxy exists before connection.' }
 $record.initialResources=$initialResources
 $connected=Invoke-ConfluxControl connect
 if ($connected.state -ne 'connected') { throw 'Service did not confirm connected.' }
 $http=New-ConfluxHttpClient
 if ($ExpectedRelayIPv4) { $record.egressIPv4=Assert-ConfluxEgress $http $PublicIPEndpoint $ExpectedRelayIPv4 }
 $clock.Restart();$nextSample=0.0;$lastSampleTime=0.0;$lastBytes=[long]0
 $buffer=New-Object byte[] 65536
 # Both elapsed time and request count are bounded; no response is buffered as a whole.
 while ($clock.Elapsed.TotalSeconds -lt $DurationSeconds -and $record.requests -lt 1000000) {
  $record.requests++
  $cancel=[Threading.CancellationTokenSource]::new()
  $remaining=[Math]::Max(1,[Math]::Min(60,$DurationSeconds-$clock.Elapsed.TotalSeconds))
  $cancel.CancelAfter([TimeSpan]::FromSeconds($remaining))
  $response=$null;$stream=$null
  try {
   $response=$http.GetAsync($DownloadURL,[Net.Http.HttpCompletionOption]::ResponseHeadersRead,$cancel.Token).GetAwaiter().GetResult()
   $null=$response.EnsureSuccessStatusCode()
   if ([int]$response.StatusCode -ge 300) { throw 'Redirects are disabled; use the final HTTPS download URL.' }
   $stream=$response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
   while ($clock.Elapsed.TotalSeconds -lt $DurationSeconds) {
    $count=$stream.ReadAsync($buffer,0,$buffer.Length,$cancel.Token).GetAwaiter().GetResult()
    if ($count -eq 0) { break }
    $record.receivedBytes+=$count
    if ($clock.Elapsed.TotalSeconds -ge $nextSample) {
     $state=Invoke-ConfluxControl status
     if ($state.state -notin @('connected','degraded')) { throw 'Tunnel stopped during transfer.' }
     $sample=Get-ConfluxResourceSample
     if ($sample.orphanProxyCount) { throw 'Orphan proxy detected.' }
     if ($sample.servicePID -ne $initialResources.servicePID) { throw 'Service restarted during soak.' }
     $now=$clock.Elapsed.TotalSeconds
     $sample.elapsedSeconds=$now
     $sample.receivedBytes=$record.receivedBytes
     $sample.returnMbps=8.0*($record.receivedBytes-$lastBytes)/[Math]::Max(0.001,$now-$lastSampleTime)/1000000.0
     $sample.tunnelState=$state.state
     $sample.pathCounters=@($state.paths | Select-Object -Property @('id','state','txBytes','rxBytes','rttMs'))
     $record.samples+=,$sample
     Save-ConfluxEvidence $record 'soak.json'
     $lastSampleTime=$now;$lastBytes=$record.receivedBytes;$nextSample=$now+$SampleSeconds
    }
   }
  } catch [OperationCanceledException] {
   if ($clock.Elapsed.TotalSeconds -lt $DurationSeconds-0.5) { throw 'HTTPS transfer stalled or exceeded its bounded per-request timeout.' }
  } finally {
   if ($stream) { $stream.Dispose() }
   if ($response) { $response.Dispose() }
   $cancel.Dispose()
  }
 }
 if ($record.receivedBytes -eq 0) { throw 'No actual response bytes were received.' }
 if ($clock.Elapsed.TotalSeconds -lt $DurationSeconds-0.5) { throw 'Request budget exhausted before requested soak duration.' }
 $record.actualDurationSeconds=$clock.Elapsed.TotalSeconds
 $record.returnMbps=8.0*$record.receivedBytes/[Math]::Max(0.001,$clock.Elapsed.TotalSeconds)/1000000.0
 $null=Invoke-ConfluxControl disconnect
 $after=Get-ConfluxNetworkSnapshot
 $record.finalResources=Get-ConfluxResourceSample
 if (($baseline | ConvertTo-Json -Depth 8 -Compress) -ne ($after | ConvertTo-Json -Depth 8 -Compress)) { $record.networkAfter=$after;throw 'Network differs from baseline after soak.' }
 if ($record.finalResources.ownedProxyCount) { throw 'Proxy remained after disconnect.' }
 $record.status='PASS_TRANSFER_ONLY_REVIEW_RESOURCE_SAMPLES'
} catch {
 $record.status='FAIL';$record.error=$_.Exception.Message
 try { $null=Invoke-ConfluxControl disconnect;$null=Invoke-ConfluxControl recover;$record.cleanup='disconnect/recover succeeded; compare recorded baseline' } catch { $record.cleanup='FAILED: preserve protected journal and service' }
} finally {
 if ($http) { $http.Dispose() }
 $record.finished=[DateTime]::UtcNow.ToString('o')
 Save-ConfluxEvidence $record 'soak.json'
}
Write-Output (Join-Path $script:EvidenceDirectory 'soak.json')
if ($record.status -eq 'FAIL') { exit 1 }
