//go:build windows

package tunnel

import (
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wintun"
	"io"
	"sync"
)

type Device struct {
	mu      sync.RWMutex
	adapter *wintun.Adapter
	session wintun.Session
	stop    windows.Handle
	closed  bool
}

func Open(name string, mtu int) (io.ReadWriteCloser, error) {
	if name == "" || mtu < 576 || mtu > 9000 {
		return nil, errors.New("invalid tunnel configuration")
	}
	a, err := wintun.CreateAdapter(name, "Conflux", nil)
	if err != nil {
		return nil, err
	}
	s, err := a.StartSession(4 << 20)
	if err != nil {
		a.Close()
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		s.End()
		a.Close()
		return nil, err
	}
	return &Device{adapter: a, session: s, stop: stop}, nil
}
func (d *Device) Read(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for {
		if d.closed {
			return 0, io.EOF
		}
		p, err := d.session.ReceivePacket()
		if err == nil {
			n := copy(b, p)
			d.session.ReleaseReceivePacket(p)
			if n < len(p) {
				return n, io.ErrShortBuffer
			}
			return n, nil
		}
		if !errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
			return 0, err
		}
		v, err := windows.WaitForMultipleObjects([]windows.Handle{d.stop, d.session.ReadWaitEvent()}, false, windows.INFINITE)
		if err != nil {
			return 0, err
		}
		if v == windows.WAIT_OBJECT_0 {
			return 0, io.EOF
		}
	}
}
func (d *Device) Write(b []byte) (int, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return 0, io.ErrClosedPipe
	}
	p, err := d.session.AllocateSendPacket(len(b))
	if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
		return 0, protocol.ErrPacketDropped
	}
	if err != nil {
		return 0, err
	}
	copy(p, b)
	d.session.SendPacket(p)
	return len(b), nil
}
func (d *Device) Close() error {
	_ = windows.SetEvent(d.stop)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	d.session.End()
	err := d.adapter.Close()
	_ = windows.CloseHandle(d.stop)
	return err
}
