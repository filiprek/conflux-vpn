package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func TestRealTCPForwardingIsPaced(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer backend.Close()
	go func() {
		c, e := backend.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.Copy(c, c)
	}()
	front, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer front.Close()
	var disabled atomic.Bool
	done := make(chan error, 1)
	go func() { done <- serve(ctx, front, backend.Addr().String(), 32768, &disabled) }()
	c, e := net.Dial("tcp4", front.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	payload := bytes.Repeat([]byte("x"), 8192)
	started := time.Now()
	if _, e = c.Write(payload); e != nil {
		t.Fatal(e)
	}
	got := make([]byte, len(payload))
	if _, e = io.ReadFull(c, got); e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("forwarder corrupted bytes")
	}
	if time.Since(started) < 200*time.Millisecond {
		t.Fatal("link cap not enforced")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shaper leaked")
	}
}
