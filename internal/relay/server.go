// Package relay authenticates QUIC paths and enforces per-session IPv4 ownership.
package relay

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/multipath"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/transport/quicpath"
	"github.com/quic-go/quic-go"
	"io"
	"net"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"
)

type PacketDevice interface{ io.ReadWriteCloser }
type Config struct {
	TLS         *tls.Config
	Token       string
	Device      PacketDevice
	Subnet      netip.Prefix
	MaxSessions int
}
type session struct {
	id, instance string
	epoch        string
	ip           netip.Addr
	mtu          int
	endpoint     *multipath.Endpoint
	paths        map[string]bool
	known        map[string]bool
	last         time.Time
	cancel       context.CancelFunc
	ctx          context.Context
}
type Server struct {
	mu, deviceMu    sync.Mutex
	config          Config
	sessions        map[string]*session
	byIP            map[netip.Addr]*session
	listener        *quic.Listener
	pc              net.PacketConn
	ctx             context.Context
	cancel          context.CancelFunc
	started, closed bool
	done            chan struct{}
	wg              sync.WaitGroup
	slots           chan struct{}
	failure         error
	spoofRejected   atomic.Uint64
}

func New(config Config) (*Server, error) {
	if config.TLS == nil || len(config.TLS.Certificates) == 0 || config.Device == nil || len(config.Token) < 16 || len(config.Token) > 256 {
		return nil, errors.New("relay certificate, device and high entropy token required")
	}
	if !config.Subnet.IsValid() {
		config.Subnet = netip.MustParsePrefix("10.250.0.0/16")
	}
	if !config.Subnet.Addr().Is4() || config.Subnet.Bits() < 16 || config.Subnet.Bits() > 24 {
		return nil, errors.New("relay requires an IPv4 /16 through /24 subnet")
	}
	config.Subnet = config.Subnet.Masked()
	if config.MaxSessions == 0 {
		config.MaxSessions = 128
	}
	if config.MaxSessions < 1 || config.MaxSessions > 1024 {
		return nil, errors.New("invalid session limit")
	}
	return &Server{config: config, sessions: make(map[string]*session), byIP: make(map[netip.Addr]*session), done: make(chan struct{}), slots: make(chan struct{}, 64)}, nil
}
func (s *Server) Addr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pc == nil {
		return nil
	}
	return s.pc.LocalAddr()
}
func (s *Server) Serve(ctx context.Context, pc net.PacketConn) error {
	s.mu.Lock()
	if s.started || s.closed {
		s.mu.Unlock()
		return errors.New("relay already served or closed")
	}
	s.started = true
	s.pc = pc
	s.ctx, s.cancel = context.WithCancel(ctx)
	listener, err := quicpath.Listen(pc, s.config.TLS)
	if err != nil {
		s.closed = true
		s.mu.Unlock()
		close(s.done)
		return err
	}
	s.listener = listener
	s.mu.Unlock()
	defer close(s.done)
	s.wg.Add(3)
	go s.readDevice()
	go s.reap()
	go func() { defer s.wg.Done(); <-s.ctx.Done(); s.shutdown() }()
	for {
		conn, err := listener.Accept(s.ctx)
		if err != nil {
			break
		}
		// Slots bound unauthenticated handshakes only; handle releases its slot once
		// the path is authenticated so long-lived paths do not exhaust capacity.
		select {
		case s.slots <- struct{}{}:
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				var once sync.Once
				release := func() { once.Do(func() { <-s.slots }) }
				defer release()
				s.handle(conn, release)
			}()
		default:
			conn.CloseWithError(1, "relay busy")
		}
	}
	s.shutdown()
	s.wg.Wait()
	s.mu.Lock()
	err = s.failure
	s.mu.Unlock()
	return err
}
func (s *Server) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	cancel := s.cancel
	listener := s.listener
	pc := s.pc
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if listener != nil {
		listener.Close()
	}
	if pc != nil {
		pc.Close()
	}
	s.config.Device.Close()
}
func (s *Server) Close() error {
	s.shutdown()
	s.mu.Lock()
	started := s.started
	s.mu.Unlock()
	if started {
		<-s.done
	}
	return nil
}
func (s *Server) handle(conn *quic.Conn, authenticated func()) {
	defer conn.CloseWithError(0, "path closed")
	ctx, cancel := context.WithTimeout(s.ctx, 4*time.Second)
	defer cancel()
	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		return
	}
	stream.SetDeadline(time.Now().Add(4 * time.Second))
	var h protocol.Hello
	if err = protocol.ReadControl(stream, &h); err != nil {
		return
	}
	reject := func(reason string) {
		_ = protocol.WriteControl(stream, protocol.Welcome{Version: protocol.Version, Error: reason})
		stream.Close()
	}
	if h.MTU > protocol.DefaultMTU {
		reject("requested MTU exceeds underlay-safe maximum 1100")
		return
	}
	if err = h.Validate(); err != nil {
		reject(err.Error())
		return
	}
	if subtle.ConstantTimeCompare([]byte(h.Token), []byte(s.config.Token)) != 1 {
		reject("relay authentication failed")
		return
	}
	if !conn.ConnectionState().SupportsDatagrams.Remote {
		reject("QUIC DATAGRAM required")
		return
	}
	authenticated()
	s.mu.Lock()
	ss := s.sessions[h.SessionID]
	if ss == nil {
		if len(s.sessions) >= s.config.MaxSessions {
			s.mu.Unlock()
			reject("relay session limit reached")
			return
		}
		ip, err := nextAddress(s.config.Subnet, s.byIP)
		if err != nil {
			s.mu.Unlock()
			reject("relay address pool exhausted")
			return
		}
		var epoch [16]byte
		if _, err := rand.Read(epoch[:]); err != nil {
			s.mu.Unlock()
			reject("relay session identity unavailable")
			return
		}
		ss = &session{epoch: hex.EncodeToString(epoch[:]), id: h.SessionID, instance: h.InstanceID, ip: ip, mtu: h.MTU, paths: make(map[string]bool), known: make(map[string]bool), last: time.Now()}
		runctx, stop := context.WithCancel(s.ctx)
		ss.cancel = stop
		ss.ctx = runctx
		ss.endpoint = multipath.NewEndpoint(h.MTU, func(p []byte) error {
			src, dst, e := protocol.IPv4(p)
			if e != nil || src != ip {
				s.spoofRejected.Add(1)
				return nil
			}
			if s.config.Subnet.Contains(dst) && dst != s.config.Subnet.Addr().Next() {
				s.spoofRejected.Add(1)
				return nil
			}
			s.deviceMu.Lock()
			defer s.deviceMu.Unlock()
			n, err := s.config.Device.Write(p)
			if err == nil && n != len(p) {
				return io.ErrShortWrite
			}
			return err
		})
		s.sessions[h.SessionID] = ss
		s.byIP[ip] = ss
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := ss.endpoint.Run(runctx); err != nil && runctx.Err() == nil {
				s.fail(errors.New("relay TUN write failed"))
			}
		}()
	}
	if ss.instance != h.InstanceID || ss.mtu != h.MTU || ss.paths[h.PathID] || len(ss.paths) >= 16 || (!ss.known[h.PathID] && len(ss.known) >= 16) {
		s.mu.Unlock()
		reject("session attachment rejected")
		return
	}
	ss.paths[h.PathID] = true
	ss.known[h.PathID] = true
	ss.last = time.Now()
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(ss.paths, h.PathID); ss.last = time.Now(); s.mu.Unlock() }()
	if err = protocol.WriteControl(stream, protocol.Welcome{Version: protocol.Version, IP: ss.ip.String(), MTU: ss.mtu, Epoch: ss.epoch}); err != nil {
		return
	}
	stream.SetDeadline(time.Time{})
	controlDone := make(chan struct{})
	go func() {
		defer close(controlDone)
		var cmd protocol.Command
		if protocol.ReadControl(stream, &cmd) == nil && cmd.Type == "close-session" {
			s.mu.Lock()
			if s.sessions[ss.id] == ss {
				s.removeLocked(ss)
			}
			s.mu.Unlock()
			_ = protocol.WriteControl(stream, protocol.Command{Type: "closed"})
			stream.Close()
		}
	}()
	defer func() { stream.CancelRead(0); <-controlDone }()
	_ = ss.endpoint.Attach(ss.ctx, h.PathID, conn)
}
func (s *Server) readDevice() {
	defer s.wg.Done()
	b := make([]byte, protocol.MaxMTU+1)
	for {
		n, err := s.config.Device.Read(b)
		if err != nil {
			if s.ctx.Err() == nil {
				s.fail(errors.New("relay TUN read failed"))
			} else {
				s.shutdown()
			}
			return
		}
		_, dst, err := protocol.IPv4(b[:n])
		if err != nil {
			continue
		}
		s.mu.Lock()
		target := s.byIP[dst]
		s.mu.Unlock()
		if target != nil {
			target.endpoint.Send(b[:n])
		}
	}
}
func (s *Server) reap() {
	defer s.wg.Done()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			for _, ss := range s.sessions {
				if len(ss.paths) == 0 && now.Sub(ss.last) > time.Minute {
					s.removeLocked(ss)
				}
			}
			s.mu.Unlock()
		}
	}
}

// removeLocked forgets a session and stops its endpoint. The caller holds s.mu.
func (s *Server) removeLocked(ss *session) {
	delete(s.sessions, ss.id)
	if s.byIP[ss.ip] == ss {
		delete(s.byIP, ss.ip)
	}
	ss.cancel()
}

func (s *Server) fail(err error) {
	s.mu.Lock()
	if s.failure == nil {
		s.failure = err
	}
	s.mu.Unlock()
	s.shutdown()
}

// SessionCount returns the current bounded count, including unclean recovery leases.
func (s *Server) SessionCount() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.sessions) }
