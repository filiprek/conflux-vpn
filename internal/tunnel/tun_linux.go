//go:build linux

// Package tunnel exposes packet-oriented OS tunnel devices.
package tunnel

import (
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
)

// Open creates a nonpersistent TUN interface. Addressing and routing remain the caller's responsibility.
func Open(name string, mtu int) (io.ReadWriteCloser, error) {
	if name == "" || len(name) >= unix.IFNAMSIZ || mtu < 576 || mtu > 9000 {
		return nil, errors.New("invalid TUN name or MTU")
	}
	fd, err := unix.Open("/dev/net/tun", unix.O_RDWR|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open TUN: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = unix.Close(fd)
		}
	}()
	req, err := unix.NewIfreq(name)
	if err != nil {
		return nil, err
	}
	req.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)
	if err = unix.IoctlIfreq(fd, unix.TUNSETIFF, req); err != nil {
		return nil, err
	}
	sock, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(sock)
	req, err = unix.NewIfreq(name)
	if err != nil {
		return nil, err
	}
	req.SetUint32(uint32(mtu))
	if err = unix.IoctlIfreq(sock, unix.SIOCSIFMTU, req); err != nil {
		return nil, err
	}
	req, err = unix.NewIfreq(name)
	if err != nil {
		return nil, err
	}
	req.SetUint16(unix.IFF_UP | unix.IFF_RUNNING)
	if err = unix.IoctlIfreq(sock, unix.SIOCSIFFLAGS, req); err != nil {
		return nil, err
	}
	if err = unix.SetNonblock(fd, true); err != nil {
		return nil, err
	}
	ok = true
	return os.NewFile(uintptr(fd), name), nil
}
