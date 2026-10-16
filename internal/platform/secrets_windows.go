//go:build windows

package platform

import (
	"errors"
	"golang.org/x/sys/windows"
	"unsafe"
)

// Protect encrypts for the service account. Its directory must additionally be ACL restricted.
func Protect(b []byte) ([]byte, error)   { return crypt(b, true) }
func Unprotect(b []byte) ([]byte, error) { return crypt(b, false) }
func crypt(b []byte, encrypt bool) ([]byte, error) {
	if len(b) == 0 || len(b) > 1<<20 {
		return nil, errors.New("invalid secret storage size")
	}
	in := windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
	var out windows.DataBlob
	entropy := []byte("Conflux/secrets/v1")
	e := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	var err error
	if encrypt {
		err = windows.CryptProtectData(&in, nil, &e, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	} else {
		err = windows.CryptUnprotectData(&in, nil, &e, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out)
	}
	if err != nil {
		return nil, errors.New("windows credential protection failed")
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}
