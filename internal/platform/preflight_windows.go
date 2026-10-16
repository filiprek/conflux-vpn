//go:build windows

package platform

// Preflight checks ownership conflicts before the journal is created. It never changes host state.
func Preflight(steps []Step) error {
	for _, s := range steps {
		switch s.Do.Name {
		case "bypass":
			if _, err := powershell(`if(Get-NetRoute -DestinationPrefix $a[0] -ErrorAction SilentlyContinue){throw 'existing host route requires explicit resolution before connecting'}`, s.Do.Args); err != nil {
				return err
			}
		case "protection":
			if _, err := powershell(`if(Get-NetFirewallRule -Name ($a[0]+'-*') -ErrorAction SilentlyContinue){throw 'firewall ownership conflict'}`, s.Do.Args); err != nil {
				return err
			}
		}
	}
	return nil
}
