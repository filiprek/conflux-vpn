// Package client owns a multipath IPv4 tunnel and its reconnecting QUIC paths.
package client

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/multipath"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/transport/quicpath"
	"io"
	"math/big"
	"net"
	"sync"
	"time"
)

// PacketDevice preserves one IP packet per Read and Write. Close unblocks Read.
type PacketDevice interface{ io.ReadWriteCloser }

// PathConfig opens a fresh owned PacketConn on each recovery attempt.
// Open's context lives until the client closes; nil Open selects direct UDP.
type PathConfig struct {
	ID, Address string
	Open        func(context.Context) (net.PacketConn, error)
}
type Config struct {
	TLS           *tls.Config
	Pin           [32]byte
	Token         string
	Paths         []PathConfig
	Device        PacketDevice
	MTU, MinPaths int
}
type Snapshot struct {
	State, AssignedIP                 string
	MTU                               int
	StartedAt                         time.Time
	Paths                             []multipath.PathMetrics
	BytesSent, BytesReceived, Dropped uint64
	Reorder                           multipath.ReorderStats
}
type Client struct {
	mu                    sync.Mutex
	config                Config
	endpoint              *multipath.Endpoint
	session, instance, ip string
	epoch                 string
	done                  chan struct{}
	startedAt             time.Time
	ctx                   context.Context
	cancel                context.CancelFunc
	started, closed       bool
	wg                    sync.WaitGroup
	closeOnce             sync.Once
}

func New(config Config) (*Client, error) {
	if config.Device == nil || config.TLS == nil || config.TLS.InsecureSkipVerify || config.TLS.ServerName == "" || config.Pin == [32]byte{} || len(config.Token) < 1 || len(config.Token) > 256 {
		return nil, errors.New("device and verified relay credentials required")
	}
	if config.MTU == 0 {
		config.MTU = protocol.DefaultMTU
	}
	if config.MTU < 576 || config.MTU > protocol.DefaultMTU {
		return nil, errors.New("MTU outside supported range")
	}
	if config.MinPaths == 0 {
		config.MinPaths = 2
	}
	if len(config.Paths) < config.MinPaths || len(config.Paths) > 16 || config.MinPaths < 1 {
		return nil, errors.New("invalid path quorum")
	}
	seen := map[string]bool{}
	for _, p := range config.Paths {
		if len(p.ID) < 1 || len(p.ID) > 64 || seen[p.ID] {
			return nil, errors.New("invalid or duplicate path ID")
		}
		seen[p.ID] = true
		if _, _, err := net.SplitHostPort(p.Address); err != nil {
			return nil, errors.New("invalid relay path address")
		}
	}
	config.Paths = append([]PathConfig(nil), config.Paths...)
	config.TLS = config.TLS.Clone()
	c := &Client{config: config, done: make(chan struct{})}
	var id [32]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, err
	}
	c.session = hex.EncodeToString(id[:16])
	c.instance = hex.EncodeToString(id[16:])
	c.endpoint = multipath.NewEndpoint(config.MTU, func(p []byte) error {
		if _, _, err := protocol.IPv4(p); err != nil {
			return nil
		}
		n, err := config.Device.Write(p)
		if err == nil && n != len(p) {
			return io.ErrShortWrite
		}
		return err
	})
	for _, p := range config.Paths {
		c.endpoint.SetState(p.ID, "Connecting", "")
	}
	return c, nil
}

// Start waits for the configured quorum. ctx owns the entire connection lifetime.
// A failed Start closes all owned resources; create a new Client to retry manually.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New("client closed")
	}
	if !c.started {
		c.started = true
		c.startedAt = time.Now()
		c.ctx, c.cancel = context.WithCancel(ctx)
		c.wg.Add(2 + len(c.config.Paths))
		go func() { defer c.wg.Done(); _ = c.endpoint.Run(c.ctx); c.stop() }()
		go c.readDevice()
		for _, p := range c.config.Paths {
			go c.supervise(p)
		}
	}
	runctx := c.ctx
	c.mu.Unlock()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := c.Snapshot()
		healthy := 0
		for _, p := range snapshot.Paths {
			if p.State == "Healthy" {
				healthy++
			}
		}
		if healthy >= c.config.MinPaths {
			return nil
		}
		select {
		case <-ctx.Done():
			c.Close()
			return ctx.Err()
		case <-runctx.Done():
			c.Close()
			return errors.New("tunnel stopped before path quorum")
		case <-ticker.C:
		}
	}
}
func (c *Client) stop() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		cancel := c.cancel
		c.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		c.config.Device.Close()
		close(c.done)
	})
}
func (c *Client) Close() error { c.stop(); c.wg.Wait(); return nil }
func (c *Client) readDevice() {
	defer c.wg.Done()
	b := make([]byte, protocol.MaxMTU+1)
	for {
		n, err := c.config.Device.Read(b)
		if err != nil {
			c.stop()
			return
		}
		if _, _, err = protocol.IPv4(b[:n]); err != nil {
			continue
		}
		c.endpoint.Send(b[:n])
	}
}
func (c *Client) supervise(p PathConfig) {
	defer c.wg.Done()
	retry := recovery{}
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}
		c.endpoint.SetState(p.ID, "Connecting", "")
		attemptStarted := time.Now()
		err := c.connect(p)
		if c.ctx.Err() != nil {
			return
		}
		if errors.Is(err, errSessionEpoch) {
			c.endpoint.SetState(p.ID, "Failed", "relay session changed; rebuilding tunnel required")
			c.stop()
			return
		}
		c.endpoint.SetState(p.ID, "Backoff", category(err))
		backoff, ok := retry.next(time.Since(attemptStarted))
		if !ok {
			c.endpoint.SetState(p.ID, "Failed", "retry budget exhausted; reconnect tunnel")
			return
		}
		jitter, _ := rand.Int(rand.Reader, big.NewInt(int64(backoff/3)+1))
		delay := backoff
		if jitter != nil {
			delay += time.Duration(jitter.Int64())
		}
		timer := time.NewTimer(delay)
		select {
		case <-c.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func category(err error) string {
	if err == nil {
		return "path closed"
	}
	return "transport or authentication failed"
}
func (c *Client) connect(p PathConfig) error {
	var pc net.PacketConn
	var err error
	if p.Open != nil {
		pc, err = p.Open(c.ctx)
	} else {
		pc, err = net.ListenPacket("udp4", "0.0.0.0:0")
	}
	if err != nil {
		return err
	}
	defer pc.Close()
	addr, err := net.ResolveUDPAddr("udp", p.Address)
	if err != nil {
		return err
	}
	handshake, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	defer cancel()
	conn, err := quicpath.Dial(handshake, pc, addr, c.config.TLS, c.config.Pin)
	if err != nil {
		return err
	}
	defer conn.CloseWithError(0, "path closed")
	stream, err := conn.OpenStreamSync(handshake)
	if err != nil {
		return err
	}
	stream.SetDeadline(time.Now().Add(4 * time.Second))
	hello := protocol.Hello{Version: protocol.Version, SessionID: c.session, InstanceID: c.instance, PathID: p.ID, Token: c.config.Token, MTU: c.config.MTU, Datagrams: true}
	if err = protocol.WriteControl(stream, hello); err != nil {
		return err
	}
	var welcome protocol.Welcome
	if err = protocol.ReadControl(stream, &welcome); err != nil {
		return err
	}
	if err = welcome.Validate(); err != nil {
		return err
	}
	if welcome.MTU != c.config.MTU {
		return errors.New("relay MTU differs from requested MTU")
	}
	c.mu.Lock()
	if c.epoch != "" && c.epoch != welcome.Epoch {
		c.mu.Unlock()
		return errSessionEpoch
	}
	c.epoch = welcome.Epoch
	if c.ip != "" && c.ip != welcome.IP {
		c.mu.Unlock()
		return errors.New("relay session address changed")
	}
	c.ip = welcome.IP
	c.mu.Unlock()
	stream.SetDeadline(time.Time{})
	err = c.endpoint.Attach(c.ctx, p.ID, conn)
	if c.ctx.Err() != nil {
		stream.SetDeadline(time.Now().Add(300 * time.Millisecond))
		if protocol.WriteControl(stream, protocol.Command{Type: "close-session"}) == nil {
			stream.Close()
			var ack protocol.Command
			_ = protocol.ReadControl(stream, &ack)
		}
	}
	return err
}
func (c *Client) Snapshot() Snapshot {
	c.mu.Lock()
	s := Snapshot{AssignedIP: c.ip, MTU: c.config.MTU, StartedAt: c.startedAt, State: "Connecting"}
	closed := c.closed
	started := c.started
	c.mu.Unlock()
	s.Paths, s.Reorder, s.Dropped = c.endpoint.Snapshot()
	healthy := 0
	for _, p := range s.Paths {
		s.BytesSent += p.SentBytes
		s.BytesReceived += p.ReceivedBytes
		if p.State == "Healthy" {
			healthy++
		}
	}
	switch {
	case closed || !started:
		s.State = "Disconnected"
	case healthy >= c.config.MinPaths:
		s.State = "Connected"
	case healthy > 0:
		s.State = "Degraded"
	}
	return s
}

var errSessionEpoch = errors.New("relay session epoch changed")

// Done closes when terminal shutdown begins. Close joins all owned workers.
func (c *Client) Done() <-chan struct{} { return c.done }
