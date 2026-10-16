// Conflux privileged lab client. This helper changes networking only inside its
// disposable Docker network namespace. It is not a production Linux client.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/client"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/proxy"
	"github.com/filiprek/conflux-vpn/internal/transport/socksudp"
	"github.com/filiprek/conflux-vpn/internal/tunnel"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func command(ctx context.Context, args ...string) error {
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
func run() error {
	if v := os.Getenv("CONFLUX_LAB_EGRESS_PROXY"); v != "" {
		os.Setenv("https_proxy", v)
		os.Setenv("http_proxy", v)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	raw, e := os.ReadFile("/shared/enrollment")
	if e != nil {
		return e
	}
	enroll, e := protocol.ParseEnrollment(strings.TrimSpace(string(raw)))
	if e != nil {
		return e
	}
	tc, e := enroll.TLSConfig()
	if e != nil {
		return e
	}
	paths := []proxy.Path{}
	corePaths := []client.PathConfig{}
	for i := range 2 {
		id := fmt.Sprintf("vless-%d", i)
		n, e := proxy.ParseVLESS(fmt.Sprintf("vless://123e4567-e89b-12d3-a456-426614174000@relay:%d?security=none&type=tcp&packetEncoding=xudp", 19841+i))
		if e != nil {
			return e
		}
		port := uint16(13001 + i)
		paths = append(paths, proxy.Path{ID: id, Port: port, Node: n})
		socks := fmt.Sprintf("127.0.0.1:%d", port)
		corePaths = append(corePaths, client.PathConfig{ID: id, Address: enroll.Address, Open: func(ctx context.Context) (net.PacketConn, error) { return socksudp.Dial(ctx, socks) }})
	}
	config, e := proxy.BuildConfig(paths)
	if e != nil {
		return e
	}
	supervisor, e := proxy.NewSupervisor(proxy.Executable{Path: "/usr/local/bin/sing-box", SHA256: os.Getenv("CONFLUX_SINGBOX_SHA256"), Version: proxy.SingBoxVersion}, proxy.Options{})
	if e != nil {
		return e
	}
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- supervisor.Run(ctx, config) }()
	defer func() { cancel(); <-proxyDone }()
	for {
		c, e := net.DialTimeout("tcp", "127.0.0.1:13001", 20*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if supervisor.Status().State == "failed" {
			return fmt.Errorf("lab proxy failed: %+v", supervisor.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	dev, e := tunnel.Open("conflux0", protocol.DefaultMTU)
	if e != nil {
		return e
	}
	defer dev.Close()
	core, e := client.New(client.Config{TLS: tc, Pin: enroll.Pin, Token: enroll.Token, Paths: corePaths, Device: dev, MinPaths: 2})
	if e != nil {
		return e
	}
	defer core.Close()
	if e = core.Start(ctx); e != nil {
		return e
	}
	ip := core.Snapshot().AssignedIP
	if e = command(ctx, "ip", "addr", "add", ip+"/16", "dev", "conflux0"); e != nil {
		return e
	}
	if e = command(ctx, "ip", "route", "replace", "default", "dev", "conflux0"); e != nil {
		return e
	}
	// Docker DNS on loopback remains available; its bridge subnet route also keeps
	// relay/VLESS endpoints outside the tunnel. External HTTPS follows conflux0.
	fmt.Println("assigned client IPv4", ip)
	if e = command(ctx, "curl", "--fail", "--silent", "--show-error", "--max-time", "25", "--output", "/tmp/payload", "https://raw.githubusercontent.com/SagerNet/sing-box/v1.14.3/LICENSE"); e != nil {
		return fmt.Errorf("external HTTPS over real TUN/VLESS/relay NAT: %w", e)
	}
	payload, e := os.ReadFile("/tmp/payload")
	if e != nil {
		return e
	}
	sum := sha256.Sum256(payload)
	if fmt.Sprintf("%x", sum) != "650d5e3b99a446fb38e820fa87a49562e0c79eab868fff58618ac487a58e554c" {
		return fmt.Errorf("external HTTPS response content hash mismatch")
	}
	fmt.Printf("external HTTPS verified %d bytes SHA256 %x\n", len(payload), sum)
	if e = command(ctx, "ip", "-s", "link", "show", "dev", "conflux0"); e != nil {
		return e
	}
	if os.Getenv("CONFLUX_LAB_BENCH") == "1" {
		if e = benchmarks(ctx); e != nil {
			return e
		}
	}
	snapshot := core.Snapshot()
	for _, p := range snapshot.Paths {
		if p.SentPackets == 0 || p.ReceivedPackets == 0 {
			return fmt.Errorf("lab path did not carry bidirectional IPv4: %+v", p)
		}
	}
	b, _ := json.Marshal(snapshot)
	fmt.Println("verified measured core counters:", string(b))
	return nil
}

func benchmarks(ctx context.Context) error {
	baselineG := runtime.NumGoroutine()
	beforeFD, _ := os.ReadDir("/proc/self/fd")
	run := func(socks string, duration int, bytes string, rate string) (float64, uint64, error) {
		args := []string{"--relay-file", "/shared/enrollment", "--socks", socks, "--target", "10.250.0.1:9000", "--mode", "bidi", "--duration", strconv.Itoa(duration) + "s", "--rate-mbps", rate}
		if bytes != "" {
			args = append(args, "--bytes", bytes)
		}
		cmd := exec.CommandContext(ctx, "/usr/local/bin/conflux-bench", args...)
		cmd.Stderr = os.Stderr
		b, e := cmd.Output()
		if e != nil {
			return 0, 0, fmt.Errorf("real VLESS benchmark: %w", e)
		}
		fmt.Println(string(b))
		var result struct {
			DownloadMbps float64
			Replies      uint64
		}
		if e = json.Unmarshal(b, &result); e != nil {
			return 0, 0, e
		}
		return result.DownloadMbps, result.Replies, nil
	}
	fmt.Println("CAPPED SINGLE PATH 2Mbit/s per VLESS direction")
	single, _, e := run("127.0.0.1:13001", 12, "", "8")
	if e != nil {
		return e
	}
	fmt.Println("CAPPED TWO PATHS each2Mbit/s per VLESS direction")
	dual, _, e := run("127.0.0.1:13001,127.0.0.1:13002", 12, "", "8")
	if e != nil {
		return e
	}
	fmt.Printf("MEASURED CAPPED GAIN single=%.6fMbps dual=%.6fMbps ratio=%.4f\n", single, dual, dual/single)
	// Preserve measured evidence even if acceptance threshold is not reached.
	gainOK := single > 0 && dual/single >= 1.5
	// This private lab volume coordinates only the disposable relay namespace.
	if e = os.WriteFile("/coord/unshape", []byte("ready"), 0600); e != nil {
		return e
	}
	unshapeDeadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = os.Stat("/coord/unshaped"); e == nil {
			break
		}
		if time.Now().After(unshapeDeadline) {
			return fmt.Errorf("relay lab shaping cleanup timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fmt.Println("UNSHAPED SOAK70koffered with bounded runtime")
	_, replies, e := run("127.0.0.1:13001,127.0.0.1:13002", 35, "71960000", "20")
	if e != nil {
		return e
	}
	runtime.GC()
	afterFD, _ := os.ReadDir("/proc/self/fd")
	fmt.Printf("MEASURED SOAK replies=%d goroutines_before=%d after=%d fds_before=%d after=%d\n", replies, baselineG, runtime.NumGoroutine(), len(beforeFD), len(afterFD))
	if !gainOK {
		return fmt.Errorf("measured capped aggregation did not meet1.5x threshold")
	}
	if replies < 60000 {
		return fmt.Errorf("measured soak delivered only%d packets;60000not reached", replies)
	}
	return nil
}
