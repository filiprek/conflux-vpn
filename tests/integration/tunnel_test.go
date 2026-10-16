package integration

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/relay"
	"io"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type device struct {
	in, out chan []byte
	done    chan struct{}
	once    sync.Once
}

func newDevice() *device {
	return &device{in: make(chan []byte, 1024), out: make(chan []byte, 1024), done: make(chan struct{})}
}
func (d *device) Read(b []byte) (int, error) {
	select {
	case p := <-d.in:
		return copy(b, p), nil
	case <-d.done:
		return 0, io.EOF
	}
}
func (d *device) Write(b []byte) (int, error) {
	p := append([]byte(nil), b...)
	select {
	case d.out <- p:
		return len(p), nil
	case <-d.done:
		return 0, io.EOF
	}
}
func (d *device) Close() error { d.once.Do(func() { close(d.done) }); return nil }
func packet(src, dst string, marker byte) []byte {
	p := make([]byte, 100)
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	copy(p[12:16], net.ParseIP(src).To4())
	copy(p[16:20], net.ParseIP(dst).To4())
	p[20] = marker
	return p
}

type harness struct {
	server    *relay.Server
	pc        net.PacketConn
	tls       *tls.Config
	pin       [32]byte
	dev       *device
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan error
	serverTLS *tls.Config
}

func setup(t *testing.T) *harness {
	t.Helper()
	der, key, e := protocol.GenerateIdentity("127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	roots := x509.NewCertPool()
	parsed, _ := x509.ParseCertificate(der)
	roots.AddCert(parsed)
	pc, e := net.ListenPacket("udp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	dev := newDevice()
	s, e := relay.New(relay.Config{TLS: &tls.Config{Certificates: []tls.Certificate{cert}}, Token: "integration-secret-token", Device: dev, Subnet: netip.MustParsePrefix("10.250.0.0/16"), MaxSessions: 4})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &harness{s, pc, &tls.Config{RootCAs: roots, ServerName: "127.0.0.1"}, sha256.Sum256(der), dev, ctx, cancel, make(chan error, 1), &tls.Config{Certificates: []tls.Certificate{cert}}}
	go func() { h.done <- s.Serve(ctx, pc) }()
	t.Cleanup(func() {
		cancel()
		s.Close()
		select {
		case <-h.done:
		case <-time.After(3 * time.Second):
			t.Error("relay leaked")
		}
	})
	return h
}
func (h *harness) newClient(t *testing.T, token string, pin [32]byte, paths []client.PathConfig) (*client.Client, *device) {
	t.Helper()
	d := newDevice()
	c, e := client.New(client.Config{TLS: h.tls, Pin: pin, Token: token, Paths: paths, Device: d, MinPaths: len(paths)})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return c, d
}
func wait(t *testing.T, f func() bool) {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if f() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("condition timeout")
		case <-ticker.C:
		}
	}
}
func TestRealTwoPathTunnelFailoverRecovery(t *testing.T) {
	h := setup(t)
	var mu sync.Mutex
	var current net.PacketConn
	blocked := false
	open := func(context.Context) (net.PacketConn, error) {
		mu.Lock()
		deny := blocked
		mu.Unlock()
		if deny {
			return nil, errors.New("path offline")
		}
		p, e := net.ListenPacket("udp4", "127.0.0.1:0")
		mu.Lock()
		current = p
		mu.Unlock()
		return p, e
	}
	c, d := h.newClient(t, "integration-secret-token", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String(), Open: open}, {ID: "b", Address: h.pc.LocalAddr().String()}})
	ctx, cancel := context.WithTimeout(h.ctx, 5*time.Second)
	defer cancel()
	if e := c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	ip := c.Snapshot().AssignedIP
	if ip == "" {
		t.Fatal("no allocation")
	}
	for i := range 20 {
		d.in <- packet(ip, "1.1.1.1", byte(i))
	}
	got := 0
	for got < 20 {
		select {
		case p := <-h.dev.out:
			if p[20] != byte(got) {
				t.Fatalf("reorder %d %d", p[20], got)
			}
			got++
		case <-time.After(3 * time.Second):
			t.Fatal("tunnel upload timeout")
		}
	}
	wait(t, func() bool {
		s := c.Snapshot()
		return len(s.Paths) == 2 && s.Paths[0].SentPackets > 0 && s.Paths[1].SentPackets > 0
	})
	h.dev.in <- packet("1.1.1.1", ip, 88)
	select {
	case p := <-d.out:
		if p[20] != 88 {
			t.Fatal("wrong return")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("download timeout")
	}
	mu.Lock()
	blocked = true
	current.Close()
	mu.Unlock()
	wait(t, func() bool {
		for _, p := range c.Snapshot().Paths {
			if p.ID == "a" && p.State != "Healthy" {
				return true
			}
		}
		return false
	})
	d.in <- packet(ip, "1.1.1.1", 99)
	select {
	case <-h.dev.out:
	case <-time.After(3 * time.Second):
		t.Fatal("failover lost tunnel")
	}
	mu.Lock()
	blocked = false
	mu.Unlock()
	wait(t, func() bool {
		for _, p := range c.Snapshot().Paths {
			if p.ID == "a" && p.Reconnects > 0 && p.State == "Healthy" {
				return true
			}
		}
		return false
	})
	d.in <- packet("10.250.0.222", "1.1.1.1", 100)
	select {
	case <-h.dev.out:
		t.Fatal("source spoof accepted")
	case <-time.After(150 * time.Millisecond):
	}
}
func TestRelayMeasuredMetrics(t *testing.T) {
	h := setup(t)
	c, d := h.newClient(t, "integration-secret-token", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
	ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()
	if e := c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	d.in <- packet(c.Snapshot().AssignedIP, "1.1.1.1", 1)
	select {
	case <-h.dev.out:
	case <-ctx.Done():
		t.Fatal("missing packet")
	}
	s := h.server.Snapshot()
	if s.Sessions != 1 || s.Paths != 1 || s.BytesReceived != 100 {
		t.Fatalf("unmeasured relay metrics %+v", s)
	}
	d.in <- packet("10.250.0.99", "1.1.1.1", 1)
	wait(t, func() bool { return h.server.Snapshot().SpoofRejected == 1 })
}

func TestAuthenticationAndPinRejection(t *testing.T) {
	for _, test := range []string{"token", "pin", "untrusted"} {
		t.Run(test, func(t *testing.T) {
			h := setup(t)
			token := "integration-secret-token"
			pin := h.pin
			if test == "token" {
				token = "incorrect"
			}
			if test == "pin" {
				pin[0] ^= 1
			}
			if test == "untrusted" {
				h.tls = &tls.Config{ServerName: "127.0.0.1"}
			}
			c, _ := h.newClient(t, token, pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
			ctx, cancel := context.WithTimeout(h.ctx, 400*time.Millisecond)
			defer cancel()
			if e := c.Start(ctx); e == nil {
				t.Fatal("invalid authentication accepted")
			}
		})
	}
}

func TestCloseDuringStartAndRepeatedClose(t *testing.T) {
	h := setup(t)
	c, _ := h.newClient(t, "incorrect", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
	done := make(chan error, 1)
	go func() { done <- c.Start(h.ctx) }()
	c.Close()
	c.Close()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("start succeeded after close")
		}
	case <-time.After(time.Second):
		t.Fatal("close during start leaked")
	}
}
func TestPathProbeAuthenticatedDatagramHealth(t *testing.T) {
	h := setup(t)
	ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()
	s, e := client.Probe(ctx, client.Config{TLS: h.tls, Pin: h.pin, Token: "integration-secret-token", Paths: []client.PathConfig{{ID: "probe", Address: h.pc.LocalAddr().String()}}})
	if e != nil || len(s.Paths) != 1 || s.Paths[0].ProbeSent < 2 || s.Paths[0].State != "Healthy" {
		t.Fatalf("probe %+v %v", s, e)
	}
}
func TestRelayDeviceFailureReported(t *testing.T) {
	h := setup(t)
	wait(t, func() bool { return h.server.Addr() != nil })
	h.dev.Close()
	select {
	case err := <-h.done:
		if err == nil {
			t.Error("fatal TUN failure reported as clean shutdown")
		}
		h.done <- err
	case <-time.After(2 * time.Second):
		t.Fatal("relay did not stop after TUN failure")
	}
}
func TestCleanDisconnectReleasesSession(t *testing.T) {
	h := setup(t)
	c, _ := h.newClient(t, "integration-secret-token", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
	ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
	defer cancel()
	if e := c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	c.Close()
	if h.server.SessionCount() != 0 {
		t.Fatal("clean disconnect retained session goroutines")
	}
}
func TestConnectDisconnectCycles(t *testing.T) {
	h := setup(t)
	for i := range 100 {
		c, _ := h.newClient(t, "integration-secret-token", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
		ctx, cancel := context.WithTimeout(h.ctx, 3*time.Second)
		if err := c.Start(ctx); err != nil {
			cancel()
			t.Fatalf("cycle %d: %v", i, err)
		}
		c.Close()
		cancel()
		if h.server.SessionCount() != 0 {
			t.Fatalf("cycle %d retained session", i)
		}
	}
}

func TestRelayRecreationTerminatesOldGeneration(t *testing.T) {
	h := setup(t)
	c, _ := h.newClient(t, "integration-secret-token", h.pin, []client.PathConfig{{ID: "a", Address: h.pc.LocalAddr().String()}})
	ctx, cancel := context.WithTimeout(h.ctx, 12*time.Second)
	defer cancel()
	if e := c.Start(ctx); e != nil {
		t.Fatal(e)
	}
	address := h.pc.LocalAddr().String()
	h.server.Close()
	<-h.done
	pc, e := net.ListenPacket("udp4", address)
	if e != nil {
		t.Fatal(e)
	}
	dev := newDevice()
	server, e := relay.New(relay.Config{TLS: h.serverTLS, Token: "integration-secret-token", Device: dev, MaxSessions: 4})
	if e != nil {
		t.Fatal(e)
	}
	h.pc = pc
	h.dev = dev
	h.server = server
	h.done = make(chan error, 1)
	go func() { h.done <- server.Serve(h.ctx, pc) }()
	wait(t, func() bool { return c.Snapshot().State == "Disconnected" })
}
