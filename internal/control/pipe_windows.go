//go:build windows

package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"net"
	"strings"
	"sync"
	"time"
)

func PipePath(name string) (string, error) {
	if len(name) > 180 || !strings.HasPrefix(name, "Conflux-") || strings.ContainsAny(name, `\/`) {
		return "", errors.New("invalid Conflux pipe name")
	}
	return `\\.\pipe\` + name, nil
}

// ServePipe accepts only the configured owner and administrators. Remote clients are rejected by go-winio.
func ServePipe(ctx context.Context, name, ownerSID string, c *Controller) error {
	return ServePipeReady(ctx, name, ownerSID, c, nil)
}
func ServePipeReady(ctx context.Context, name, ownerSID string, c *Controller, ready func()) error {
	sid, err := windows.StringToSid(ownerSID)
	if err != nil {
		return errors.New("invalid owner SID")
	}
	_ = sid
	c.ownerSID = ownerSID
	path, err := PipePath(name)
	if err != nil {
		return err
	}
	listener, err := winio.ListenPipe(path, &winio.PipeConfig{SecurityDescriptor: "O:SYG:SYD:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x120003;;;" + ownerSID + ")", InputBufferSize: MaxMessage, OutputBufferSize: MaxMessage})
	if err != nil {
		return err
	}
	defer listener.Close()
	if ready != nil {
		ready()
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			listener.Close()
		case <-done:
		}
	}()
	var wg sync.WaitGroup
	defer wg.Wait()
	slots := make(chan struct{}, 8)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case slots <- struct{}{}:
			wg.Add(1)
			go func() { defer wg.Done(); defer func() { <-slots }(); handleConnection(ctx, conn, c) }()
		default:
			conn.Close()
		}
	}
}
func handleConnection(ctx context.Context, conn net.Conn, c *Controller) {
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	for scanner.Scan() {
		var q Request
		r := Response{Version: 1}
		if json.Unmarshal(scanner.Bytes(), &q) != nil {
			r.Error = "invalid service request"
		} else {
			r.ID = q.ID
			callCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
			v, err := c.Handle(callCtx, q)
			cancel()
			if err != nil {
				r.Error = err.Error()
			} else {
				r.Result = v
			}
		}
		b, err := json.Marshal(r)
		if err != nil || len(b) > MaxMessage {
			return
		}
		b = append(b, '\n')
		if _, err = conn.Write(b); err != nil {
			return
		}
		_ = conn.SetDeadline(time.Now().Add(90 * time.Second))
	}
}
func CallPipe(ctx context.Context, name string, q Request) (Response, error) {
	var r Response
	path, err := PipePath(name)
	if err != nil {
		return r, err
	}
	conn, err := winio.DialPipeAccess(ctx, path, windows.SYNCHRONIZE|windows.READ_CONTROL|windows.FILE_READ_DATA|windows.FILE_WRITE_DATA)
	if err != nil {
		return r, errors.New("conflux service unavailable")
	}
	defer conn.Close()
	if err = verifyPipeOwner(conn); err != nil {
		return r, err
	}
	deadline := time.Now().Add(65 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	b, err := json.Marshal(q)
	if err != nil || len(b) > MaxMessage {
		return r, errors.New("request too large")
	}
	if _, err = conn.Write(append(b, '\n')); err != nil {
		return r, err
	}
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), MaxMessage)
	if !scanner.Scan() {
		return r, errors.New("service response missing")
	}
	if err = json.Unmarshal(scanner.Bytes(), &r); err != nil {
		return r, errors.New("invalid service response")
	}
	if r.Version != 1 || r.ID != q.ID {
		return r, errors.New("service response mismatch")
	}
	if r.Error != "" {
		return r, errors.New(r.Error)
	}
	return r, nil
}

func verifyPipeOwner(conn interface{}) error {
	p, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return errors.New("pipe identity unavailable")
	}
	sd, err := windows.GetSecurityInfo(windows.Handle(p.Fd()), windows.SE_KERNEL_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return errors.New("pipe identity verification failed")
	}
	owner, _, err := sd.Owner()
	if err != nil || !owner.IsWellKnown(windows.WinLocalSystemSid) {
		return errors.New("pipe server is not owned by LocalSystem")
	}
	return nil
}
