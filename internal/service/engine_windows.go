//go:build windows

package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/control"
	"github.com/filiprek/conflux-vpn/internal/platform"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/proxy"
	"github.com/filiprek/conflux-vpn/internal/transport/socksudp"
	"github.com/filiprek/conflux-vpn/internal/tunnel"
	"github.com/filiprek/conflux-vpn/internal/version"
)

type Settings struct {
	Version        int    `json:"version"`
	SingBoxPath    string `json:"singBoxPath"`
	SingBoxSHA256  string `json:"singBoxSHA256"`
	SingBoxVersion string `json:"singBoxVersion"`
}

// Engine serializes network operations with mu. Fields read by Status (state,
// lastError, client, tested) are additionally written under view so status polling
// never waits behind a connection attempt or path test.
type Engine struct {
	mu               sync.Mutex
	view             sync.Mutex
	cancelMu         sync.Mutex
	intent           atomic.Uint64
	ctx              context.Context
	settings         Settings
	tx               platform.Transaction
	client           *client.Client
	cancel           context.CancelFunc
	proxyDone        chan error
	state, lastError string
	activeContext    context.Context
	tested           map[string]bool
}

func NewEngine(ctx context.Context, s Settings, dir string) *Engine {
	return &Engine{ctx: ctx, settings: s, tx: platform.Transaction{Path: filepath.Join(dir, "state.json"), Runner: platform.WindowsRunner{}}, state: "disconnected", tested: map[string]bool{}}
}
func (e *Engine) Recover() error { e.mu.Lock(); defer e.mu.Unlock(); return e.tx.Recover() }
func (e *Engine) setState(state, lastError string) {
	e.view.Lock()
	e.state, e.lastError = state, lastError
	e.view.Unlock()
}
func (e *Engine) Status() control.Status {
	e.view.Lock()
	defer e.view.Unlock()
	s := control.Status{State: e.state, Paths: []control.PathStatus{}, Mode: "bond", MTU: protocol.DefaultMTU, LastError: e.lastError, ServiceVersion: version.String(), ProtocolVersion: "1"}
	for id, ok := range e.tested {
		if ok {
			s.Paths = append(s.Paths, control.PathStatus{ID: id, State: "ready"})
		}
	}
	if e.client == nil {
		return s
	}
	v := e.client.Snapshot()
	// Until routes are applied the tunnel carries no traffic; report the engine state.
	if e.state == "connected" {
		s.State = strings.ToLower(v.State)
	}
	s.MTU = v.MTU
	s.TxBytes = v.BytesSent
	s.RxBytes = v.BytesReceived
	if !v.StartedAt.IsZero() {
		s.UptimeSeconds = time.Since(v.StartedAt).Seconds()
	}
	aggregate := 0.0
	for _, p := range v.Paths {
		var loss *float64
		if p.ProbeSent > 0 {
			x := 100 * float64(p.ProbeLost) / float64(p.ProbeSent)
			loss = &x
		}
		throughput := p.BytesPerSecond
		aggregate += throughput
		s.Paths = append(s.Paths, control.PathStatus{ID: p.ID, State: strings.ToLower(p.State), LossPercent: loss, BytesPerSecond: &throughput, RTTMs: float64(p.RTT) / float64(time.Millisecond), TxBytes: p.SentBytes, RxBytes: p.ReceivedBytes, LastError: p.LastError})
	}
	s.BytesPerSecond = &aggregate
	return s
}
func (e *Engine) Disconnect() error {
	e.intent.Add(1)
	e.cancelMu.Lock()
	cancel := e.cancel
	e.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.stop(); err != nil {
		return err
	}
	e.setState("disconnected", "")
	return nil
}
func (e *Engine) stop() error {
	err := e.tx.Recover()
	e.cancelMu.Lock()
	cancel := e.cancel
	e.cancel = nil
	e.cancelMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if e.client != nil {
		err = errors.Join(err, e.client.Close())
		e.view.Lock()
		e.client = nil
		e.view.Unlock()
	}
	if e.proxyDone != nil {
		<-e.proxyDone
		e.proxyDone = nil
	}
	if err != nil {
		e.setState("error", "network recovery incomplete; use Recover before reconnecting")
	} else {
		e.view.Lock()
		e.state = "disconnected"
		e.view.Unlock()
	}
	return err
}
func resolve4(ctx context.Context, host string) ([]netip.Addr, error) {
	lookup, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(lookup, "ip4", host)
	if err != nil || len(ips) == 0 {
		return nil, errors.New("ipv4 endpoint resolution failed")
	}
	return ips, nil
}
func (e *Engine) Connect(ctx context.Context, p control.Profile) error {
	e.mu.Lock()
	if e.client != nil && e.client.Snapshot().State != "Disconnected" {
		e.mu.Unlock()
		return nil
	}
	e.mu.Unlock()
	return e.connect(ctx, p, e.intent.Add(1))
}
func (e *Engine) connect(ctx context.Context, p control.Profile, intent uint64) (err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.intent.Load() != intent {
		return context.Canceled
	}
	if e.client != nil {
		if err = e.stop(); err != nil {
			return err
		}
	}
	if err = e.tx.Recover(); err != nil {
		return err
	}
	e.setState("connecting", "")
	defer func() {
		if err != nil && e.stop() == nil {
			e.setState("disconnected", "connection failed; traffic is not tunneled")
		}
	}()
	var nonce [6]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return err
	}
	alias := "Conflux-" + hex.EncodeToString(nonce[:])
	dev, err := tunnel.Open(alias, p.MTU)
	if err != nil {
		return errors.New("wintun could not start; verify installed driver and service permissions")
	}
	owned := false
	defer func() {
		if !owned {
			dev.Close()
		}
	}()
	cfg, endpoints, done, cancel, err := e.prepare(p, "", dev)
	if err != nil {
		return err
	}
	e.cancelMu.Lock()
	e.cancel = cancel
	e.cancelMu.Unlock()
	if e.intent.Load() != intent {
		cancel()
		return context.Canceled
	}
	e.proxyDone = done
	c, err := client.New(cfg)
	if err != nil {
		return err
	}
	e.view.Lock()
	e.client = c
	e.view.Unlock()
	owned = true
	start := make(chan error, 1)
	go func() { start <- c.Start(e.activeContext) }()
	select {
	case err = <-start:
	case proxyErr := <-done:
		cancel()
		<-start
		if proxyErr != nil {
			return errors.New("managed sing-box could not start; verify configured executable and hash")
		}
		return errors.New("managed sing-box stopped before tunnel activation")
	case <-ctx.Done():
		cancel()
		err = ctx.Err()
		<-start
	}
	if err != nil {
		return errors.New("relay quorum not established; verify both VLESS paths support UDP")
	}
	index, gateway, err := platform.PhysicalRoute()
	if err != nil {
		return err
	}
	steps, err := platform.PlanNetwork(platform.NetworkInput{Alias: alias, PhysicalIndex: index, Gateway: gateway, Address: c.Snapshot().AssignedIP, MTU: p.MTU, Endpoints: endpoints})
	if err != nil {
		return err
	}
	if err = platform.Preflight(steps); err != nil {
		return err
	}
	if err = e.tx.Apply(steps); err != nil {
		return err
	}
	e.setState("connected", "")
	go e.monitor(p, intent, e.activeContext, c, done)

	return nil
}

func (e *Engine) prepare(p control.Profile, only string, dev io.ReadWriteCloser) (client.Config, []netip.Addr, chan error, context.CancelFunc, error) {
	var cfg client.Config
	enrollment, err := protocol.ParseEnrollment(p.RelayURI)
	if err != nil {
		return cfg, nil, nil, nil, errors.New("import a valid relay first")
	}
	tlsConfig, err := enrollment.TLSConfig()
	if err != nil {
		return cfg, nil, nil, nil, err
	}
	life, cancel := context.WithCancel(e.ctx)
	e.activeContext = life
	fail := func(err error) (client.Config, []netip.Addr, chan error, context.CancelFunc, error) {
		cancel()
		return cfg, nil, nil, nil, err
	}
	host, port, err := net.SplitHostPort(enrollment.Address)
	if err != nil {
		return fail(err)
	}
	ips, err := resolve4(life, host)
	if err != nil {
		return fail(err)
	}
	address := net.JoinHostPort(ips[0].String(), port)
	endpoints := []netip.Addr{ips[0]}
	cfg = client.Config{TLS: tlsConfig, Pin: enrollment.Pin, Token: enrollment.Token, Device: dev, MTU: p.MTU, MinPaths: 2}
	if only == "relay" {
		cfg.MinPaths = 1
		cfg.Paths = []client.PathConfig{{ID: "relay", Address: address}}
		done := make(chan error)
		close(done)
		return cfg, endpoints, done, cancel, nil
	}
	var proxyPaths []proxy.Path
	resolved := map[string]string{}
	for i, path := range p.Paths {
		if (only == "" && !path.Enabled) || (only != "" && path.ID != only) {
			continue
		}
		node, err := control.ParsePath(path)
		if err != nil {
			return fail(err)
		}
		ips, err := resolve4(life, node.Server())
		if err != nil {
			return fail(err)
		}
		endpoints = append(endpoints, ips...)
		resolved[path.ID] = ips[0].String()
		localPort := uint16(13001 + i)
		// Fresh per-session credentials keep other local processes off the loopback proxy.
		var secret [24]byte
		if _, err = rand.Read(secret[:]); err != nil {
			return fail(err)
		}
		user, pass := "conflux-"+path.ID, hex.EncodeToString(secret[:])
		proxyPaths = append(proxyPaths, proxy.Path{ID: path.ID, Port: localPort, Node: node, Username: user, Password: pass})
		local := net.JoinHostPort("127.0.0.1", strconv.Itoa(int(localPort)))
		cfg.Paths = append(cfg.Paths, client.PathConfig{ID: path.ID, Address: address, Open: func(ctx context.Context) (net.PacketConn, error) { return socksudp.DialAuth(ctx, local, user, pass) }})
	}
	if only != "" {
		cfg.MinPaths = 1
	}
	if len(cfg.Paths) < cfg.MinPaths {
		return fail(errors.New("enable at least two VLESS paths for Bond"))
	}
	config, err := proxy.BuildConfig(proxyPaths)
	if err != nil {
		return fail(err)
	}
	// Pin resolved proxy addresses for the lifetime of this routing transaction.
	// Keep the original TLS name when replacing a hostname by its physical IPv4 endpoint.
	var doc map[string]any
	if err = json.Unmarshal(config, &doc); err != nil {
		return fail(err)
	}
	outbounds, _ := doc["outbounds"].([]any)
	for _, v := range outbounds {
		o, ok := v.(map[string]any)
		if !ok {
			continue
		}
		id, _ := o["tag"].(string)
		ip, ok := resolved[strings.TrimPrefix(id, "out-")]
		if !ok {
			continue
		}
		original, _ := o["server"].(string)
		o["server"] = ip
		if t, ok := o["tls"].(map[string]any); ok {
			if _, exists := t["server_name"]; !exists {
				t["server_name"] = original
			}
		}
	}
	config, err = json.Marshal(doc)
	if err != nil {
		return fail(err)
	}
	supervisor, err := proxy.NewSupervisor(proxy.Executable{Path: e.settings.SingBoxPath, SHA256: e.settings.SingBoxSHA256, Version: e.settings.SingBoxVersion}, proxy.Options{MaxRestarts: 4})
	if err != nil {
		return fail(err)
	}
	done := make(chan error, 1)
	go func() { defer close(done); done <- supervisor.Run(life, config) }()
	return cfg, endpoints, done, cancel, nil
}

// probeDevice is used only by explicit path tests. No system routes are activated.
type probeDevice struct {
	once sync.Once
	done chan struct{}
}

func newProbeDevice() *probeDevice                 { return &probeDevice{done: make(chan struct{})} }
func (d *probeDevice) Read(b []byte) (int, error)  { <-d.done; return 0, io.EOF }
func (d *probeDevice) Write(b []byte) (int, error) { return len(b), nil }
func (d *probeDevice) Close() error                { d.once.Do(func() { close(d.done) }); return nil }
func (e *Engine) Test(ctx context.Context, p control.Profile, id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil {
		return errors.New("disconnect before testing paths")
	}
	if id == "" {
		return errors.New("select a path to test")
	}
	dev := newProbeDevice()
	defer dev.Close()
	cfg, _, done, cancel, err := e.prepare(p, id, dev)
	if err != nil {
		return err
	}
	defer func() { cancel(); <-done }()
	c, err := client.New(cfg)
	if err != nil {
		return err
	}
	defer c.Close()
	result := make(chan error, 1)
	go func() { result <- c.Start(e.activeContext) }()
	timeout := time.NewTimer(15 * time.Second)
	defer timeout.Stop()
	select {
	case err = <-result:
		if err == nil {
			e.view.Lock()
			e.tested[id] = true
			e.view.Unlock()
		}
		return err
	case <-ctx.Done():
		cancel()
		<-result
		return ctx.Err()
	case <-timeout.C:
		cancel()
		<-result
		return errors.New("path did not complete relay QUIC authentication and health probes; check UDP support")
	}
}

// monitor rebuilds a terminal session with fresh sequence/address state. Manual
// disconnect invalidates intent before waiting for the serialized network transaction.
func (e *Engine) monitor(p control.Profile, intent uint64, life context.Context, current *client.Client, proxyDone <-chan error) {
	select {
	case <-life.Done():
		return
	case <-proxyDone:
	case <-current.Done():
	}
	e.mu.Lock()
	if e.intent.Load() != intent || e.client != current || e.activeContext != life || life.Err() != nil {
		e.mu.Unlock()
		return
	}
	err := e.stop()
	if err == nil {
		e.setState("recovering", "tunnel stopped; rebuilding the session after restoring owned network changes")
	}
	e.mu.Unlock()
	if err != nil {
		return
	}
	for attempt := 0; attempt < 3; attempt++ {
		if e.intent.Load() != intent || e.ctx.Err() != nil {
			return
		}
		request, cancel := context.WithTimeout(e.ctx, 30*time.Second)
		err = e.connect(request, p, intent)
		cancel()
		if err == nil {
			return
		}
		delay := time.NewTimer(time.Duration(attempt+1) * time.Second)
		select {
		case <-e.ctx.Done():
			delay.Stop()
			return
		case <-delay.C:
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.intent.Load() == intent {
		e.setState("error", "automatic session recovery exhausted; networking restored, reconnect manually after checking relay and paths")
	}
}
