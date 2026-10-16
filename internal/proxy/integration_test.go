package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/filiprek/conflux-vpn/internal/transport/socksudp"
	"net"
	"os"
	"testing"
	"time"
)

// Optional release-binary integration: CONFLUX_SING_BOX must name an independently
// installed sing-box 1.14.3. The test computes a local pin for the test run only.
func TestReleaseBinary(t *testing.T) {
	p := os.Getenv("CONFLUX_SING_BOX")
	if p == "" {
		t.Skip("CONFLUX_SING_BOX not set")
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(b)
	for _, q := range []string{"?security=tls", "?security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=ab12&fp=chrome", "?security=tls&type=ws&path=%2Fa&host=example.com", "?security=tls&type=grpc&serviceName=svc"} {
		t.Run(q, func(t *testing.T) {
			n, e := ParseVLESS(baseURI + q)
			if e != nil {
				t.Fatal(e)
			}
			config, e := BuildConfig([]Path{{ID: "a", Port: 23001, Node: n}, {ID: "b", Port: 23002, Node: n}})
			if e != nil {
				t.Fatal(e)
			}
			s, e := NewSupervisor(Executable{Path: p, SHA256: hex.EncodeToString(sum[:]), Version: SingBoxVersion}, Options{})
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- s.Run(ctx, config) }()
			ready := false
			for ctx.Err() == nil {
				conn, err := net.DialTimeout("tcp", "127.0.0.1:23001", 20*time.Millisecond)
				if err == nil {
					conn.Close()
					ready = true
					break
				}
				select {
				case e := <-done:
					t.Fatalf("release failed before ready: %v %+v", e, s.Status())
				case <-time.After(5 * time.Millisecond):
				}
			}
			if ready {
				assoc, err := socksudp.Dial(ctx, "127.0.0.1:23001")
				if err != nil {
					cancel()
					<-done
					t.Fatalf("real sing-box UDP ASSOCIATE failed: %v", err)
				}
				assoc.Close()
			}
			cancel()
			e = <-done
			if !ready || s.Status().Attempts != 1 || s.Status().State != "stopped" {
				t.Fatalf("release config/start failed: %v %+v", e, s.Status())
			}
		})
	}
}
