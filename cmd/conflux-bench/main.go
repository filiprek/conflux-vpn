// conflux-bench measures the same authenticated DATAGRAM data path used by the VPN.
package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/relay"
	"github.com/filiprek/conflux-vpn/internal/transport/socksudp"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var version = "dev"
var commit = "unknown"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	self := flag.Bool("self-test", false, "local real-QUIC test with bounded echo packet devices; no Internet/TUN proof")
	enrollmentFile := flag.String("relay-file", "", "file containing relay enrollment string (secret)")
	duration := flag.Duration("duration", 5*time.Second, "measurement duration")
	count := flag.Int("paths", 2, "number of independent direct paths")
	socks := flag.String("socks", "", "comma-separated local SOCKS UDP listeners (replaces direct paths)")
	target := flag.String("target", "1.1.1.1:9000", "benchmark echo service IPv4:port")
	echoListen := flag.String("echo-listen", "", "run UDP benchmark responder at this address")
	modeName := flag.String("mode", "bidi", "upload, download or bidi")
	rate := flag.Float64("rate-mbps", 20, "offered IP payload rate in Mbps")
	byteLimit := flag.Uint64("bytes", 0, "stop offering packets after this many inner bytes")
	failAfter := flag.Duration("fail-after", 0, "self-test only: take first path offline for one second")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Printf("Conflux bench %s commit=%s go=%s protocol=%d\n", version, commit, runtime.Version(), protocol.Version)
		return nil
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *echoListen != "" {
		return serveEcho(ctx, *echoListen)
	}
	if *duration <= 0 || *duration > 24*time.Hour || *rate <= 0 || *rate > 10000 || *count < 1 || *count > 16 {
		return errors.New("invalid duration, rate or path count")
	}
	mode := bidi
	switch *modeName {
	case "upload":
		mode = upload
	case "download":
		mode = download
	case "bidi":
	default:
		return errors.New("invalid benchmark mode")
	}
	var tc *tls.Config
	var pin [32]byte
	var token, address string
	if *self {
		var err error
		var cleanup func()
		tc, pin, token, address, cleanup, err = startLocal(ctx)
		if err != nil {
			return err
		}
		defer cleanup()
	} else {
		if *enrollmentFile == "" {
			return errors.New("use --relay-file or --self-test")
		}
		b, err := os.ReadFile(*enrollmentFile)
		if err != nil {
			return err
		}
		e, err := protocol.ParseEnrollment(strings.TrimSpace(string(b)))
		if err != nil {
			return err
		}
		tc, err = e.TLSConfig()
		if err != nil {
			return err
		}
		pin = e.Pin
		token = e.Token
		address = e.Address
	}
	paths := make([]client.PathConfig, *count)
	for i := range paths {
		paths[i] = client.PathConfig{ID: fmt.Sprintf("path-%d", i+1), Address: address}
	}
	if *socks != "" {
		addresses := strings.Split(*socks, ",")
		paths = nil
		for i, proxy := range addresses {
			paths = append(paths, client.PathConfig{ID: fmt.Sprintf("socks-%d", i+1), Address: address, Open: func(ctx context.Context) (net.PacketConn, error) { return socksudp.Dial(ctx, proxy) }})
		}
	}
	var socketMu sync.Mutex
	var firstSocket net.PacketConn
	var offline atomic.Bool
	if *failAfter > 0 {
		if !*self || *socks != "" {
			return errors.New("--fail-after requires direct --self-test")
		}
		paths[0].Open = func(context.Context) (net.PacketConn, error) {
			if offline.Load() {
				return nil, errors.New("benchmark path offline")
			}
			pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
			socketMu.Lock()
			firstSocket = pc
			socketMu.Unlock()
			return pc, err
		}
	}
	device := newPacketDevice(false)
	c, err := client.New(client.Config{TLS: tc, Pin: pin, Token: token, Paths: paths, Device: device, MinPaths: len(paths)})
	if err != nil {
		return err
	}
	defer c.Close()
	startup, startupCancel := context.WithTimeout(ctx, 10*time.Second)
	defer startupCancel()
	started := make(chan error, 1)
	go func() { started <- c.Start(ctx) }()
	select {
	case err = <-started:
		if err != nil {
			return err
		}
	case <-startup.Done():
		return errors.New("benchmark path quorum timeout")
	}
	destination, err := net.ResolveUDPAddr("udp4", *target)
	if err != nil {
		return err
	}
	dst, ok := netip.AddrFromSlice(destination.IP)
	if !ok {
		return errors.New("invalid benchmark target")
	}
	dst = dst.Unmap()
	src := netip.MustParseAddr(c.Snapshot().AssignedIP)
	var received atomic.Uint64
	var replies atomic.Uint64
	var workers sync.WaitGroup
	runctx, stop := context.WithCancel(ctx)
	defer func() { stop(); workers.Wait() }()
	workers.Add(1)
	go func() {
		defer workers.Done()
		for {
			select {
			case p := <-device.output:
				received.Add(uint64(len(p)))
				replies.Add(1)
			case <-runctx.Done():
				return
			}
		}
	}()
	if *failAfter > 0 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			timer := time.NewTimer(*failAfter)
			defer timer.Stop()
			select {
			case <-runctx.Done():
				return
			case <-timer.C:
			}
			offline.Store(true)
			socketMu.Lock()
			if firstSocket != nil {
				firstSocket.Close()
			}
			socketMu.Unlock()
			timer.Reset(time.Second)
			select {
			case <-runctx.Done():
				return
			case <-timer.C:
				offline.Store(false)
			}
		}()
	}
	start := time.Now()
	deadline := time.NewTimer(*duration)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	size := 1000
	offerSize := size + 28
	if mode == download {
		offerSize = 44
	}
	budget := 0.0
	var sent, offered, drops uint64
	last := start
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-deadline.C:
			break loop
		case now := <-ticker.C:
			budget += now.Sub(last).Seconds() * (*rate) * 1000000 / 8 / float64(size+28)
			last = now
			budget = min(budget, 64)
			for budget >= 1 {
				budget--
				if *byteLimit > 0 && offered >= *byteLimit {
					continue
				}
				p := makePacket(src, dst, 40000, uint16(destination.Port), mode, size, sent+1)
				select {
				case device.input <- p:
					sent++
					offered += uint64(offerSize)
				default:
					drops++
				}
			}
		}
	}
	elapsed := time.Since(start).Seconds()
	snapshot := c.Snapshot()
	result := struct {
		Environment, Mode                                                          string
		Seconds                                                                    float64
		OfferedPackets, Replies, OfferedBytes, DeliveredReturnBytes, ProducerDrops uint64
		UploadMbps, DownloadMbps                                                   float64
		Goroutines                                                                 int
		Tunnel                                                                     client.Snapshot
	}{"real QUIC loopback packet devices", *modeName, elapsed, sent, replies.Load(), offered, received.Load(), drops, float64(snapshot.BytesSent) * 8 / elapsed / 1000000, float64(received.Load()) * 8 / elapsed / 1000000, runtime.NumGoroutine(), snapshot}
	if !*self {
		result.Environment = "relay TUN to UDP echo target"
	}
	if result.Replies == 0 {
		return errors.New("benchmark received no echo replies; verify target, forwarding and NAT")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}
func startLocal(ctx context.Context) (*tls.Config, [32]byte, string, string, func(), error) {
	var zero [32]byte
	der, key, err := protocol.GenerateIdentity("127.0.0.1")
	if err != nil {
		return nil, zero, "", "", nil, err
	}
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		return nil, zero, "", "", nil, err
	}
	roots := x509.NewCertPool()
	cert, _ := x509.ParseCertificate(der)
	roots.AddCert(cert)
	token := "local-benchmark-test-credential"
	server, err := relay.New(relay.Config{TLS: &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}, Token: token, Device: newPacketDevice(true), MaxSessions: 4})
	if err != nil {
		pc.Close()
		return nil, zero, "", "", nil, err
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, pc) }()
	cleanup := func() { server.Close(); pc.Close(); <-done }
	return &tls.Config{RootCAs: roots, ServerName: "127.0.0.1"}, sha256.Sum256(der), token, pc.LocalAddr().String(), cleanup, nil
}
func serveEcho(ctx context.Context, address string) error {
	pc, err := net.ListenPacket("udp4", address)
	if err != nil {
		return err
	}
	defer pc.Close()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			pc.Close()
		case <-done:
		}
	}()
	b := make([]byte, 2048)
	for {
		n, remote, err := pc.ReadFrom(b)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		out, err := echoPayload(b[:n])
		if err != nil {
			continue
		}
		if _, err = pc.WriteTo(out, remote); err != nil {
			return err
		}
	}
}
