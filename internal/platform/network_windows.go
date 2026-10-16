//go:build windows

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

type WindowsRunner struct{}

// All PowerShell programs below are fixed; values are transported as JSON, never executable text.
func powershell(script string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	b, _ := json.Marshal(args)
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `$ErrorActionPreference='Stop'; $a=ConvertFrom-Json $env:CONFLUX_NETWORK_ARGS; `+script)
	cmd.Env = append(os.Environ(), "CONFLUX_NETWORK_ARGS="+string(b))
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 25 {
			return nil, ErrNotApplied
		}
		return nil, errors.New("windows network operation failed; journal retained for recovery")
	}
	return out.Bytes(), nil
}
func (WindowsRunner) Run(name string, args ...string) error {
	scripts := map[string]string{
		"bypass":               `if(Get-NetRoute -AddressFamily IPv4 -DestinationPrefix $a[0] -ErrorAction SilentlyContinue){exit 25}; New-NetRoute -DestinationPrefix $a[0] -InterfaceIndex ([int]$a[1]) -NextHop $a[2] -RouteMetric ([int]$a[3]) -PolicyStore ActiveStore | Out-Null`,
		"remove-bypass":        `Get-NetRoute -DestinationPrefix $a[0] -InterfaceIndex ([int]$a[1]) -NextHop $a[2] -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Where-Object {$_.RouteMetric -eq ([int]$a[3])} | Remove-NetRoute -Confirm:$false`,
		"address":              `New-NetIPAddress -InterfaceAlias $a[0] -IPAddress $a[1] -PrefixLength 16 -AddressFamily IPv4 -PolicyStore ActiveStore | Out-Null; Set-NetIPInterface -InterfaceAlias $a[0] -AddressFamily IPv4 -NlMtuBytes ([int]$a[2]) -InterfaceMetric 1`,
		"remove-address":       `Get-NetIPAddress -InterfaceAlias $a[0] -IPAddress $a[1] -ErrorAction SilentlyContinue | Remove-NetIPAddress -Confirm:$false`,
		"protection":           `if(Get-NetFirewallRule -Name ($a[0]+'-*') -ErrorAction SilentlyContinue){exit 25}; New-NetFirewallRule -Name ($a[0]+'-IPv6') -DisplayName 'Conflux IPv6 leak protection' -Direction Outbound -Action Block -RemoteAddress '::/0' -Profile Any | Out-Null; foreach($proto in @('TCP','UDP')) {New-NetFirewallRule -Name ($a[0]+'-DNS-'+$proto) -DisplayName 'Conflux DNS leak protection' -Direction Outbound -Action Block -Protocol $proto -RemotePort 53 -RemoteAddress @('0.0.0.0-1.1.1.0','1.1.1.2-255.255.255.255') -Profile Any | Out-Null}`,
		"remove-protection":    `foreach($suffix in @('-IPv6','-DNS-TCP','-DNS-UDP')){Get-NetFirewallRule -Name ($a[0]+$suffix) -ErrorAction SilentlyContinue | Remove-NetFirewallRule}`,
		"dns":                  `Set-DnsClientServerAddress -InterfaceAlias $a[0] -ServerAddresses @('1.1.1.1'); Clear-DnsClientCache`,
		"remove-dns":           `if(Get-NetAdapter -Name $a[0] -ErrorAction SilentlyContinue){Set-DnsClientServerAddress -InterfaceAlias $a[0] -ResetServerAddresses}; Clear-DnsClientCache`,
		"tunnel-routes":        `foreach($prefix in @('1.1.1.1/32','0.0.0.0/1','128.0.0.0/1')){New-NetRoute -DestinationPrefix $prefix -InterfaceAlias $a[0] -NextHop '0.0.0.0' -RouteMetric 1 -PolicyStore ActiveStore | Out-Null}`,
		"remove-tunnel-routes": `foreach($prefix in @('1.1.1.1/32','0.0.0.0/1','128.0.0.0/1')){Get-NetRoute -DestinationPrefix $prefix -InterfaceAlias $a[0] -NextHop '0.0.0.0' -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false}`,
	}
	script, ok := scripts[name]
	if !ok {
		return errors.New("unknown journal operation")
	}
	_, err := powershell(script, args)
	return err
}
func PhysicalRoute() (int, string, error) {
	b, err := powershell(`$r=Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' | Where-Object {$_.NextHop -ne '0.0.0.0'} | Sort-Object @{Expression={$_.RouteMetric+(Get-NetIPInterface -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4).InterfaceMetric}} | Select-Object -First 1; if(!$r){throw 'no physical gateway'}; @{index=$r.InterfaceIndex;gateway=$r.NextHop} | ConvertTo-Json -Compress`, nil)
	if err != nil {
		return 0, "", err
	}
	var r struct {
		Index   int    `json:"index"`
		Gateway string `json:"gateway"`
	}
	if err = json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &r); err != nil {
		return 0, "", err
	}
	return r.Index, r.Gateway, nil
}
