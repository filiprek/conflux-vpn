// Package quicpath creates verified QUIC DATAGRAM connections.
package quicpath

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/quic-go/quic-go"
	"net"
	"time"
)

func Config() *quic.Config {
	return &quic.Config{EnableDatagrams: true, HandshakeIdleTimeout: 4 * time.Second, MaxIdleTimeout: 5 * time.Second, KeepAlivePeriod: time.Second, MaxIncomingStreams: 1, MaxIncomingUniStreams: -1, MaxStreamReceiveWindow: 8192, InitialStreamReceiveWindow: 8192, MaxConnectionReceiveWindow: 16384, InitialConnectionReceiveWindow: 16384}
}

// Dial always performs ordinary certificate validation before the additional pin.
func Dial(ctx context.Context, pc net.PacketConn, addr net.Addr, config *tls.Config, pin [32]byte) (*quic.Conn, error) {
	if config == nil || config.InsecureSkipVerify || config.ServerName == "" || pin == [32]byte{} {
		return nil, errors.New("verified TLS identity and pin required")
	}
	tc := config.Clone()
	tc.MinVersion = tls.VersionTLS13
	tc.NextProtos = []string{protocol.ALPN}
	previous := tc.VerifyConnection
	tc.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 || sha256.Sum256(cs.PeerCertificates[0].Raw) != pin {
			return errors.New("relay certificate pin mismatch")
		}
		if previous != nil {
			return previous(cs)
		}
		return nil
	}
	return quic.Dial(ctx, pc, addr, tc, Config())
}
func Listen(pc net.PacketConn, config *tls.Config) (*quic.Listener, error) {
	if config == nil || len(config.Certificates) == 0 {
		return nil, errors.New("relay TLS certificate required")
	}
	tc := config.Clone()
	tc.MinVersion = tls.VersionTLS13
	tc.NextProtos = []string{protocol.ALPN}
	return quic.Listen(pc, tc, Config())
}
