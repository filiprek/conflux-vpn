package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/proxy"
	"github.com/filiprek/conflux-vpn/internal/transport/socksudp"
)

// These fixtures use the official separately installed executable, never a mock
// proxy. A release hash must be supplied explicitly; no test-time implicit trust.
func vlessExecutable(t *testing.T) proxy.Executable {
	t.Helper()
	path, hash := os.Getenv("CONFLUX_SINGBOX_PATH"), os.Getenv("CONFLUX_SINGBOX_SHA256")
	if path == "" || hash == "" {
		t.Skip("real VLESS integration requires CONFLUX_SINGBOX_PATH and CONFLUX_SINGBOX_SHA256 for separately installed sing-box 1.14.3")
	}
	return proxy.Executable{Path: path, SHA256: hash, Version: proxy.SingBoxVersion}
}

type vlessProcess struct {
	cancel context.CancelFunc
	done   chan error
	once   sync.Once
	s      *proxy.Supervisor
}

func startVLESSProcess(t *testing.T, ctx context.Context, exe proxy.Executable, config []byte, port uint16) *vlessProcess {
	t.Helper()
	s, e := proxy.NewSupervisor(exe, proxy.Options{})
	if e != nil {
		t.Fatal(e)
	}
	runctx, cancel := context.WithCancel(ctx)
	p := &vlessProcess{cancel: cancel, done: make(chan error, 1), s: s}
	t.Cleanup(func() { p.stop(t) })
	go func() { p.done <- s.Run(runctx, config) }()
	waitVLESS(t, 10*time.Second, func() bool {
		if s.Status().State == "failed" {
			t.Fatalf("real sing-box failed to start: %+v", s.Status())
		}
		c, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(int(port))), 20*time.Millisecond)
		if e != nil {
			return false
		}
		c.Close()
		return true
	})
	return p
}
func (p *vlessProcess) stop(t *testing.T) {
	t.Helper()
	p.once.Do(func() {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(5 * time.Second):
			t.Error("real sing-box process did not terminate")
		}
	})
}
func waitVLESS(t *testing.T, timeout time.Duration, f func() bool) {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if f() {
			return
		}
		select {
		case <-timer.C:
			t.Fatal("real VLESS condition timed out")
		case <-ticker.C:
		}
	}
}
func vlessPorts(t *testing.T, n int) []uint16 {
	t.Helper()
	listeners := make([]net.Listener, 0, n)
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	ports := make([]uint16, 0, n)
	for range n {
		l, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		listeners = append(listeners, l)
		ports = append(ports, uint16(l.Addr().(*net.TCPAddr).Port))
	}
	return ports
}
func vlessServerConfig(t *testing.T, port uint16, uuid string) []byte {
	t.Helper()
	b, e := json.Marshal(map[string]any{
		"log":       map[string]any{"level": "error"},
		"inbounds":  []any{map[string]any{"type": "vless", "tag": "server", "listen": "127.0.0.1", "listen_port": port, "users": []any{map[string]any{"uuid": uuid}}}},
		"outbounds": []any{map[string]any{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "direct"},
	})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

// TestRealTwoVLESSPathTunnel exercises actual QUIC inside SOCKS UDP inside VLESS
// TCP through two independent official sing-box servers to a real core relay.
// Memory devices replace only OS TUN interfaces; all transport legs use sockets.
func TestRealTwoVLESSPathTunnel(t *testing.T) {
	exe := vlessExecutable(t)
	h := setup(t)
	ctx, cancel := context.WithTimeout(h.ctx, 45*time.Second)
	defer cancel()
	ports := vlessPorts(t, 4)
	uuid := "123e4567-e89b-12d3-a456-426614174000"
	serverConfigs := [][]byte{vlessServerConfig(t, ports[0], uuid), vlessServerConfig(t, ports[1], uuid)}
	first := startVLESSProcess(t, ctx, exe, serverConfigs[0], ports[0])
	startVLESSProcess(t, ctx, exe, serverConfigs[1], ports[1])
	paths := make([]proxy.Path, 0, 2)
	corePaths := make([]client.PathConfig, 0, 2)
	for i, id := range []string{"vless-a", "vless-b"} {
		n, e := proxy.ParseVLESS(fmt.Sprintf("vless://%s@127.0.0.1:%d?security=none&type=tcp&packetEncoding=xudp", uuid, ports[i]))
		if e != nil {
			t.Fatal(e)
		}
		paths = append(paths, proxy.Path{ID: id, Port: ports[i+2], Node: n})
		socksAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(ports[i+2])))
		corePaths = append(corePaths, client.PathConfig{ID: id, Address: h.pc.LocalAddr().String(), Open: func(ctx context.Context) (net.PacketConn, error) { return socksudp.Dial(ctx, socksAddress) }})
	}
	config, e := proxy.BuildConfig(paths)
	if e != nil {
		t.Fatal(e)
	}
	startVLESSProcess(t, ctx, exe, config, ports[2])
	c, d := h.newClient(t, "integration-secret-token", h.pin, corePaths)
	if e = c.Start(ctx); e != nil {
		t.Fatalf("two real VLESS paths failed QUIC enrollment: %v; snapshot=%+v", e, c.Snapshot())
	}
	initial := c.Snapshot()
	ip := initial.AssignedIP
	if ip == "" {
		t.Fatal("relay did not allocate IPv4")
	}
	roundtrip := func(base byte) {
		t.Helper()
		const count = 20
		for i := range count {
			d.in <- packet(ip, "1.1.1.1", base+byte(i))
		}
		for i := range count {
			select {
			case got := <-h.dev.out:
				if got[20] != base+byte(i) {
					t.Fatalf("VLESS upload order got %d want %d", got[20], base+byte(i))
				}
			case <-time.After(5 * time.Second):
				t.Fatal("VLESS upload did not reach real relay")
			}
		}
		// QUIC DATAGRAM is unreliable: during asymmetric failure detection the
		// relay may send one packet into the failed upstream before its own
		// heartbeat expires. Repeat a bounded application datagram, as an upper
		// layer would retransmit, and require the real return payload to arrive.
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		retry := time.NewTicker(100 * time.Millisecond)
		defer retry.Stop()
		h.dev.in <- packet("1.1.1.1", ip, base)
		for {
			select {
			case got := <-d.out:
				if got[20] == base {
					return
				}
			case <-retry.C:
				h.dev.in <- packet("1.1.1.1", ip, base)
			case <-deadline.C:
				t.Fatalf("relay return did not cross VLESS (phase %d): %+v", base, c.Snapshot())
			}
		}
	}
	roundtrip(0)
	waitVLESS(t, 5*time.Second, func() bool {
		m := c.Snapshot().Paths
		return len(m) == 2 && m[0].SentPackets > 0 && m[1].SentPackets > 0
	})
	beforeFailure := c.Snapshot()
	first.stop(t) // Kill one actual upstream process, retaining the shared client.
	waitVLESS(t, 15*time.Second, func() bool {
		for _, p := range c.Snapshot().Paths {
			if p.ID == "vless-a" {
				return p.State != "Healthy"
			}
		}
		return false
	})
	if snap := c.Snapshot(); snap.AssignedIP != ip || snap.StartedAt != initial.StartedAt {
		t.Fatal("tunnel session changed during upstream failure")
	}
	roundtrip(40)
	startVLESSProcess(t, ctx, exe, serverConfigs[0], ports[0])
	waitVLESS(t, 15*time.Second, func() bool {
		for _, p := range c.Snapshot().Paths {
			if p.ID == "vless-a" {
				return p.State == "Healthy" && p.Reconnects > 0
			}
		}
		return false
	})
	roundtrip(80)
	waitVLESS(t, 5*time.Second, func() bool {
		for _, p := range c.Snapshot().Paths {
			if p.ID == "vless-a" {
				for _, old := range beforeFailure.Paths {
					if old.ID == p.ID {
						return p.SentPackets > old.SentPackets
					}
				}
			}
		}
		return false
	})
	if snap := c.Snapshot(); snap.AssignedIP != ip || snap.StartedAt != initial.StartedAt {
		t.Fatal("recovered VLESS path created a different session")
	}
	t.Logf("real VLESS forward/return, two-path traffic, upstream kill and recovery verified; final snapshot: %+v", c.Snapshot())
}
