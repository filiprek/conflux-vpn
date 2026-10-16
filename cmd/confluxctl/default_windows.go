//go:build windows

package main

import (
	"golang.org/x/sys/windows"
)

func defaultPipe() string {
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return "Conflux-unknown"
	}
	return "Conflux-" + user.User.Sid.String()
}
