//go:build windows

package control

import (
	"errors"
	"golang.org/x/sys/windows"
)

func permitExport(path, ownerSID string) error {
	if _, err := windows.StringToSid(ownerSID); err != nil {
		return errors.New("diagnostics owner missing")
	}
	sd, err := windows.SecurityDescriptorFromString("O:SYG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)(A;;FR;;;" + ownerSID + ")")
	if err != nil {
		return err
	}
	acl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil)
}
