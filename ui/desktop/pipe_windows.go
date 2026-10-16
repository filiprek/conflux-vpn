//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"time"
)

func callService(body []byte, id string) (json.RawMessage, error) {
	token := windows.GetCurrentProcessToken()
	owner, err := token.GetTokenUser()
	if err != nil {
		return nil, errors.New("service unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
	defer cancel()
	conn, err := winio.DialPipeAccess(ctx, `\\.\pipe\Conflux-`+owner.User.Sid.String(), windows.SYNCHRONIZE|windows.READ_CONTROL|windows.FILE_READ_DATA|windows.FILE_WRITE_DATA)
	if err != nil {
		return nil, errors.New("service unavailable")
	}
	defer conn.Close()
	if err = verifyService(conn); err != nil {
		return nil, errors.New("service identity could not be verified")
	}
	if err = conn.SetDeadline(time.Now().Add(65 * time.Second)); err != nil {
		return nil, errors.New("service unavailable")
	}
	for len(body) > 0 {
		n, writeErr := conn.Write(body)
		if writeErr != nil || n == 0 {
			return nil, errors.New("service request failed")
		}
		body = body[n:]
	}
	return decodeResponse(conn, id)
}

// Authenticate the server before transmitting imported credentials. A local user
// may otherwise squat the predictable pipe name before the service starts.
func verifyService(conn interface{}) error {
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("missing pipe handle")
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(pipe.Fd()), windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return err
	}
	if owner == nil || !owner.IsWellKnown(windows.WinLocalSystemSid) {
		return errors.New("pipe owner is not LocalSystem")
	}
	return nil
}
