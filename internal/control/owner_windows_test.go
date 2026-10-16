//go:build windows

package control

import (
	"testing"

	"golang.org/x/sys/windows"
)

func testOwnerSID(t *testing.T) string {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid.String()
}
