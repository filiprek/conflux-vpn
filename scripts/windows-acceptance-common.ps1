# Shared real-system acceptance helpers. Dot-source from the harnesses.
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function Assert-ConfluxAdministrator {
 $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
 $principal = New-Object Security.Principal.WindowsPrincipal($identity)
 if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'Real Windows administrator session required.' }
}
function Invoke-ConfluxControl([string]$Command) {
 if ($Command -notin @('status','connect','disconnect','recover','diagnostics')) { throw 'Unsupported harness command.' }
 $stdout = Join-Path $script:EvidenceDirectory ('ctl-' + [guid]::NewGuid().ToString('N') + '.out')
 $stderr = $stdout + '.err'
 try {
  $process = Start-Process -FilePath $script:ControlPath -ArgumentList @('--pipe',$script:Pipe,$Command) -NoNewWindow -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
  if (!$process.WaitForExit(90000)) { $process.Kill(); throw "Control command timed out: $Command" }
  $process.WaitForExit()
  if ($process.ExitCode -ne 0) { throw "Control command failed: $Command (exit $($process.ExitCode)); protected recovery state retained." }
  return (Get-Content -LiteralPath $stdout -Raw | ConvertFrom-Json)
 } finally {
  Remove-Item -LiteralPath $stdout,$stderr -Force -ErrorAction SilentlyContinue
 }
}
function Get-ConfluxNetworkSnapshot {
 $routes = @(Get-NetRoute -AddressFamily IPv4 -PolicyStore ActiveStore | Sort-Object -Property @('DestinationPrefix','InterfaceIndex','NextHop','RouteMetric') | ForEach-Object {
  '{0}|{1}|{2}|{3}|{4}' -f $_.DestinationPrefix,$_.InterfaceIndex,$_.NextHop,$_.RouteMetric,$_.Protocol
 })
 $dns = @(Get-DnsClientServerAddress | Where-Object {$_.InterfaceAlias -notlike 'Conflux-*'} | Sort-Object -Property @('InterfaceIndex','AddressFamily') | ForEach-Object {
  '{0}|{1}|{2}' -f $_.InterfaceIndex,$_.AddressFamily,($_.ServerAddresses -join ',')
 })
 $firewall = @(Get-NetFirewallRule -PolicyStore ActiveStore | Where-Object {$_.Name -like 'Conflux-*'} | Sort-Object Name | ForEach-Object {
  '{0}|{1}|{2}|{3}|{4}' -f $_.Name,$_.Enabled,$_.Direction,$_.Action,$_.Profile
 })
 $adapters = @(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -like 'Conflux-*'} | Sort-Object Name | ForEach-Object { $_.Name })
 $ipv6 = @(Get-NetAdapterBinding -ComponentID ms_tcpip6 | Where-Object {$_.Name -notlike 'Conflux-*'} | Sort-Object Name | ForEach-Object {'{0}|{1}' -f $_.Name,$_.Enabled})
 return [ordered]@{routes=$routes;physicalDNS=$dns;ownedFirewall=$firewall;ownedAdapters=$adapters;physicalIPv6Bindings=$ipv6}
}
function Get-ConfluxResourceSample {
 $service = Get-CimInstance Win32_Service -Filter "Name='Conflux'"
 if (!$service -or !$service.ProcessId) { throw 'Conflux service is not running.' }
 $servicePID = [int]$service.ProcessId
 $process = Get-Process -Id $servicePID
 $runtimePath = Join-Path (Split-Path $script:ControlPath) 'sing-box.exe'
 $children = @(Get-CimInstance Win32_Process -Filter "Name='sing-box.exe'" | Where-Object {$_.ExecutablePath -eq $runtimePath})
 $orphans = @($children | Where-Object {$_.ParentProcessId -ne $servicePID})
 return [ordered]@{utc=[DateTime]::UtcNow.ToString('o');servicePID=$servicePID;handles=$process.HandleCount;rssBytes=$process.WorkingSet64;threads=$process.Threads.Count;ownedProxyCount=$children.Count;orphanProxyCount=$orphans.Count;goroutines='NOT AVAILABLE through current service API'}
}
function Save-ConfluxEvidence($Record,[string]$FileName) {
 $path = Join-Path $script:EvidenceDirectory $FileName
 $temporary = $path + '.tmp'
 [IO.File]::WriteAllText($temporary,($Record | ConvertTo-Json -Depth 20),(New-Object Text.UTF8Encoding($false)))
 Move-Item -LiteralPath $temporary -Destination $path -Force
}
function New-ConfluxHttpClient {
 Add-Type -AssemblyName System.Net.Http
 $handler = New-Object System.Net.Http.HttpClientHandler
 $handler.UseProxy = $false
 $handler.AllowAutoRedirect = $false
 $client = [System.Net.Http.HttpClient]::new($handler)
 $client.Timeout = [TimeSpan]::FromSeconds(30)
 return $client
}
function Assert-ConfluxEgress($Client,[uri]$Endpoint,[string]$ExpectedIPv4) {
 if ($Endpoint.Scheme -ne 'https' -or $Endpoint.UserInfo) { throw 'Public IP endpoint must be HTTPS without embedded credentials.' }
 $text = $Client.GetStringAsync($Endpoint).GetAwaiter().GetResult().Trim()
 $address = $null
 if (![Net.IPAddress]::TryParse($text,[ref]$address) -or $address.AddressFamily -ne [Net.Sockets.AddressFamily]::InterNetwork -or $text -ne $ExpectedIPv4) { throw 'Actual external IPv4 does not match expected relay IPv4.' }
 return $text
}
