package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/relay"
	"github.com/filiprek/conflux-vpn/internal/tunnel"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"
)

var version = "dev"
var commit = "unknown"

func main() {
	if err := run(); err != nil {
		slog.Error("relay stopped", "error", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "/etc/conflux/relay.json", "relay configuration file")
	initialize := flag.String("init", "", "initialize identity for this public hostname or IP")
	listen := flag.String("listen", ":4433", "listen address used with --init")
	check := flag.Bool("check", false, "validate configuration and TLS files without changing networking")
	rotate := flag.Bool("rotate-token", false, "rotate enrollment token; restart relay afterwards")
	health := flag.Bool("health", false, "verify local relay TLS, authentication and datagram round trips")
	showEnrollment := flag.Bool("enrollment", false, "print the current secret enrollment line for clients")
	showVersion := flag.Bool("version", false, "print build version")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Conflux relay %s commit=%s go=%s protocol=%d\n", version, commit, runtime.Version(), protocol.Version)
		return nil
	}
	if *initialize != "" {
		enrollment, err := relay.Initialize(*path, *initialize, *listen)
		if err != nil {
			return err
		}
		fmt.Println(enrollment)
		return nil
	}
	if *rotate {
		enrollment, err := relay.RotateToken(*path)
		if err != nil {
			return err
		}
		fmt.Println(enrollment)
		return nil
	}
	config, err := relay.LoadFile(*path)
	if err != nil {
		return err
	}
	if *check {
		fmt.Println("relay configuration and TLS identity valid")
		return nil
	}
	if *showEnrollment {
		enrollment, err := config.Enrollment()
		if err != nil {
			return err
		}
		fmt.Println(enrollment)
		return nil
	}
	if *health {
		raw, e := config.Enrollment()
		if e != nil {
			return e
		}
		enrollment, e := protocol.ParseEnrollment(raw)
		if e != nil {
			return e
		}
		tlsConfig, e := enrollment.TLSConfig()
		if e != nil {
			return e
		}
		_, port, e := net.SplitHostPort(config.Listen)
		if e != nil {
			return e
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		_, e = client.Probe(ctx, client.Config{TLS: tlsConfig, Pin: enrollment.Pin, Token: enrollment.Token, MTU: config.MTU, Paths: []client.PathConfig{{ID: "health", Address: net.JoinHostPort("127.0.0.1", port)}}})
		if e != nil {
			return e
		}
		fmt.Println("relay authenticated QUIC DATAGRAM health passed")
		return nil
	}
	tc, err := config.TLSConfig()
	if err != nil {
		return err
	}
	device, err := tunnel.Open(config.TUNName, config.MTU)
	if err != nil {
		return err
	}
	defer device.Close()
	pc, err := net.ListenPacket("udp", config.Listen)
	if err != nil {
		return err
	}
	defer pc.Close()
	server, err := relay.New(relay.Config{TLS: tc, Token: config.Token, Device: device, Subnet: netip.MustParsePrefix(config.Subnet), MaxSessions: config.MaxSessions})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	metricsCtx, metricsCancel := context.WithCancel(ctx)
	metricsDone := make(chan struct{})
	go func() {
		defer close(metricsDone)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-metricsCtx.Done():
				return
			case <-ticker.C:
				m := server.Snapshot()
				slog.Info("relay metrics", "sessions", m.Sessions, "paths", m.Paths, "sent_bytes", m.BytesSent, "received_bytes", m.BytesReceived, "dropped", m.Dropped, "spoof_rejected", m.SpoofRejected)
			}
		}
	}()
	defer func() { metricsCancel(); <-metricsDone }()
	slog.Info("relay listening", "address", pc.LocalAddr().String(), "tun", config.TUNName, "mtu", config.MTU)
	return server.Serve(ctx, pc)
}
