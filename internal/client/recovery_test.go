package client

import (
	"crypto/tls"
	"testing"
	"time"
)

func TestRecoveryBackoffBudgetAndStableReset(t *testing.T) {
	r := recovery{}
	previous := time.Duration(0)
	for range 30 {
		d, ok := r.next(0)
		if !ok || d < previous || d > 5*time.Second {
			t.Fatal(d, ok)
		}
		previous = d
	}
	if _, ok := r.next(0); ok {
		t.Fatal("unbounded retry")
	}
	d, ok := r.next(time.Minute)
	if !ok || d != 100*time.Millisecond {
		t.Fatal("stable interval did not reset", d, ok)
	}
}

func TestDoneClosesOnClientStop(t *testing.T) {
	c, err := New(Config{TLS: &tls.Config{ServerName: "localhost"}, Pin: [32]byte{1}, Token: "test", Paths: []PathConfig{{ID: "a", Address: "127.0.0.1:4433"}}, MinPaths: 1, Device: &probeDevice{done: make(chan struct{})}})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
		t.Fatal("new client already stopped")
	default:
	}
	c.Close()
	select {
	case <-c.Done():
	default:
		t.Fatal("terminal lifecycle not signaled")
	}
}
