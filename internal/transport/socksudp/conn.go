// Package socksudp provides an RFC 1928 UDP association over a loopback-only SOCKS server.
package socksudp

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const maxFrame = 65507

var errFrame = errors.New("invalid SOCKS UDP frame")

type DomainAddr struct {
	Host string
	Port int
}

func (a DomainAddr) Network() string { return "udp" }
func (a DomainAddr) String() string  { return net.JoinHostPort(a.Host, strconv.Itoa(a.Port)) }

type Conn struct {
	udp     *net.UDPConn
	control net.Conn
	relay   *net.UDPAddr
	once    sync.Once
	closed  chan struct{}
	readMu  sync.Mutex
	frame   []byte
}

var _ net.PacketConn = (*Conn)(nil)

// Dial associates without authentication.
func Dial(ctx context.Context, proxyAddress string) (*Conn, error) {
	return DialAuth(ctx, proxyAddress, "", "")
}

// DialAuth associates using RFC 1929 username/password authentication when
// username is non-empty, so other local processes cannot use the loopback proxy.
func DialAuth(ctx context.Context, proxyAddress, username, password string) (*Conn, error) {
	if len(username) > 255 || len(password) > 255 || (username == "") != (password == "") {
		return nil, errors.New("invalid SOCKS credentials")
	}
	host, _, e := net.SplitHostPort(proxyAddress)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return nil, errors.New("SOCKS server must be a literal loopback address")
	}
	d := net.Dialer{Timeout: 5 * time.Second}
	control, e := d.DialContext(ctx, "tcp", proxyAddress)
	if e != nil {
		return nil, e
	}
	ok := false
	defer func() {
		if !ok {
			control.Close()
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	if v, yes := ctx.Deadline(); yes && v.Before(deadline) {
		deadline = v
	}
	control.SetDeadline(deadline)
	// Cancellation must interrupt the handshake even when the caller has no deadline.
	stop := context.AfterFunc(ctx, func() { control.Close() })
	defer stop()
	method := byte(0)
	if username != "" {
		method = 2
	}
	if e = writeAll(control, []byte{5, 1, method}); e != nil {
		return nil, e
	}
	r := make([]byte, 2)
	if _, e = io.ReadFull(control, r); e != nil {
		return nil, e
	}
	if r[0] != 5 || r[1] != method {
		return nil, errors.New("SOCKS authentication negotiation rejected")
	}
	if method == 2 {
		auth := append([]byte{1, byte(len(username))}, username...)
		auth = append(append(auth, byte(len(password))), password...)
		if e = writeAll(control, auth); e != nil {
			return nil, e
		}
		if _, e = io.ReadFull(control, r); e != nil {
			return nil, e
		}
		if r[0] != 1 || r[1] != 0 {
			return nil, errors.New("SOCKS authentication failed")
		}
	}
	local := net.IPv4(127, 0, 0, 1)
	network := "udp4"
	if net.ParseIP(host).To4() == nil {
		local = net.IPv6loopback
		network = "udp6"
	}
	udp, e := net.ListenUDP(network, &net.UDPAddr{IP: local})
	if e != nil {
		return nil, e
	}
	defer func() {
		if !ok {
			udp.Close()
		}
	}()
	a := udp.LocalAddr().(*net.UDPAddr)
	addr, e := encodeAddr(a)
	if e != nil {
		return nil, e
	}
	if e = writeAll(control, append([]byte{5, 3, 0}, addr...)); e != nil {
		return nil, e
	}
	h := make([]byte, 4)
	if _, e = io.ReadFull(control, h); e != nil {
		return nil, e
	}
	if h[0] != 5 || h[1] != 0 || h[2] != 0 {
		return nil, errors.New("SOCKS UDP association rejected")
	}
	relayAddr, e := readAddr(control, h[3])
	if e != nil {
		return nil, e
	}
	relay, yes := relayAddr.(*net.UDPAddr)
	if !yes || !relay.IP.IsLoopback() || relay.Port == 0 {
		return nil, errors.New("SOCKS relay must be loopback IP with nonzero port")
	}
	if (relay.IP.To4() != nil) != (a.IP.To4() != nil) {
		return nil, errors.New("SOCKS relay address family mismatch")
	}
	control.SetDeadline(time.Time{})
	c := &Conn{udp: udp, control: control, relay: relay, closed: make(chan struct{}), frame: make([]byte, maxFrame+1)}
	ok = true
	go func() { io.Copy(io.Discard, io.LimitReader(control, 1)); c.Close() }()
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-c.closed:
		}
	}()
	return c, nil
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, e := w.Write(b)
		if e != nil {
			return e
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return nil
}
func readAddr(r io.Reader, typ byte) (net.Addr, error) {
	n := 0
	switch typ {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		b := []byte{0}
		if _, e := io.ReadFull(r, b); e != nil {
			return nil, e
		}
		n = int(b[0])
		if n == 0 {
			return nil, errFrame
		}
	default:
		return nil, errFrame
	}
	b := make([]byte, n+2)
	if _, e := io.ReadFull(r, b); e != nil {
		return nil, e
	}
	p := int(binary.BigEndian.Uint16(b[n:]))
	if p == 0 {
		return nil, errFrame
	}
	if typ == 3 {
		for _, c := range b[:n] {
			if c <= 32 || c >= 127 {
				return nil, errFrame
			}
		}
		return DomainAddr{string(b[:n]), p}, nil
	}
	return &net.UDPAddr{IP: net.IP(b[:n]), Port: p}, nil
}
func encodeAddr(a net.Addr) ([]byte, error) {
	var b []byte
	var p int
	switch a := a.(type) {
	case *net.UDPAddr:
		if a == nil {
			return nil, errFrame
		}
		p = a.Port
		if a.Zone != "" {
			return nil, errFrame
		}
		if ip := a.IP.To4(); ip != nil {
			b = append([]byte{1}, ip...)
		} else if ip := a.IP.To16(); ip != nil {
			b = append([]byte{4}, ip...)
		} else {
			return nil, errFrame
		}
	case DomainAddr:
		p = a.Port
		if len(a.Host) == 0 || len(a.Host) > 255 {
			return nil, errFrame
		}
		for _, c := range []byte(a.Host) {
			if c <= 32 || c >= 127 {
				return nil, errFrame
			}
		}
		b = append([]byte{3, byte(len(a.Host))}, a.Host...)
	default:
		return nil, fmt.Errorf("unsupported UDP target address type")
	}
	if p <= 0 || p > 65535 {
		return nil, errFrame
	}
	return append(b, byte(p>>8), byte(p)), nil
}
func decode(b []byte) ([]byte, net.Addr, error) {
	if len(b) < 4 || len(b) > maxFrame || b[0] != 0 || b[1] != 0 || b[2] != 0 {
		return nil, nil, errFrame
	}
	r := &byteReader{b: b[4:]}
	a, e := readAddr(r, b[3])
	if e != nil {
		return nil, nil, e
	}
	return r.b, a, nil
}

type byteReader struct{ b []byte }

func (r *byteReader) Read(p []byte) (int, error) {
	if len(r.b) == 0 {
		return 0, io.EOF
	}
	n := copy(p, r.b)
	r.b = r.b[n:]
	return n, nil
}
func (c *Conn) WriteTo(b []byte, a net.Addr) (int, error) {
	h, e := encodeAddr(a)
	if e != nil {
		return 0, e
	}
	if len(b)+len(h)+3 > maxFrame {
		return 0, errors.New("SOCKS UDP payload exceeds datagram bound")
	}
	frame := append([]byte{0, 0, 0}, h...)
	frame = append(frame, b...)
	_, e = c.udp.WriteToUDP(frame, c.relay)
	if e != nil {
		return 0, e
	}
	return len(b), nil
}

// ReadFrom reuses one receive buffer per association; it runs once per packet.
// Malformed or foreign datagrams are skipped rather than failing the transport.
func (c *Conn) ReadFrom(b []byte) (int, net.Addr, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	for {
		n, a, e := c.udp.ReadFromUDP(c.frame)
		if e != nil {
			return 0, nil, e
		}
		if !a.IP.Equal(c.relay.IP) || a.Port != c.relay.Port {
			continue
		}
		payload, target, e := decode(c.frame[:n])
		if e != nil {
			continue
		}
		if len(payload) > len(b) {
			return 0, nil, io.ErrShortBuffer
		}
		return copy(b, payload), target, nil
	}
}
func (c *Conn) Close() error {
	var e error
	c.once.Do(func() { close(c.closed); e = c.udp.Close(); c.control.Close() })
	return e
}
func (c *Conn) LocalAddr() net.Addr                { return c.udp.LocalAddr() }
func (c *Conn) SetDeadline(t time.Time) error      { return c.udp.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.udp.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.udp.SetWriteDeadline(t) }
