package socksudp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestLoopbackAssociate(t *testing.T) {
	tcp, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer tcp.Close()
	udp, e := net.ListenPacket("udp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer udp.Close()
	done := make(chan error, 1)
	go func() {
		c, e := tcp.Accept()
		if e != nil {
			done <- e
			return
		}
		defer c.Close()
		h := make([]byte, 3)
		_, e = io.ReadFull(c, h)
		if e == nil && !bytes.Equal(h, []byte{5, 1, 0}) {
			e = io.ErrUnexpectedEOF
		}
		if e == nil {
			_, e = c.Write([]byte{5, 0})
		}
		r := make([]byte, 10)
		if e == nil {
			_, e = io.ReadFull(c, r)
		}
		if e == nil {
			a := udp.LocalAddr().(*net.UDPAddr)
			_, e = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(a.Port >> 8), byte(a.Port)})
		}
		if e != nil {
			done <- e
			return
		}
		b := make([]byte, 65535)
		for i := 0; i < 2; i++ {
			n, a, e := udp.ReadFrom(b)
			if e != nil {
				done <- e
				return
			}
			_, e = udp.WriteTo(b[:n], a)
			if e != nil {
				done <- e
				return
			}
		}
		done <- nil
		io.Copy(io.Discard, c)
	}()
	c, e := Dial(context.Background(), tcp.Addr().String())
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	for _, dst := range []net.Addr{&net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 443}, DomainAddr{Host: "relay.example", Port: 443}} {
		if n, e := c.WriteTo([]byte("hello"), dst); e != nil || n != 5 {
			t.Fatalf("write %d %v", n, e)
		}
		b := make([]byte, 20)
		n, a, e := c.ReadFrom(b)
		if e != nil || string(b[:n]) != "hello" || a.String() != dst.String() {
			t.Fatalf("read %d %v %v", n, a, e)
		}
	}
	if e := <-done; e != nil {
		t.Fatal(e)
	}
}
func TestRemoteRejected(t *testing.T) {
	if _, e := Dial(context.Background(), "192.0.2.1:1080"); e == nil {
		t.Fatal("remote accepted")
	}
}
func TestMalformedFrames(t *testing.T) {
	for _, b := range [][]byte{{}, {0, 0, 1, 1, 1, 2, 3, 4, 0, 1}, {0, 0, 0, 3, 255}, {1, 0, 0, 1, 1, 2, 3, 4, 0, 1}, {0, 0, 0, 9}} {
		if _, _, e := decode(b); e == nil {
			t.Fatal("malformed accepted")
		}
	}
}
func FuzzFrame(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 127, 0, 0, 1, 1, 187, 1})
	f.Fuzz(func(t *testing.T, b []byte) { decode(b) })
}
func TestRejectMalformedAssociateReplies(t *testing.T) {
	for _, reply := range [][]byte{{4, 0, 0, 1, 127, 0, 0, 1, 1, 1}, {5, 1, 0, 1, 127, 0, 0, 1, 1, 1}, {5, 0, 1, 1, 127, 0, 0, 1, 1, 1}, {5, 0, 0, 1, 192, 0, 2, 1, 1, 1}, {5, 0, 0, 1, 127, 0, 0, 1, 0, 0}, {5, 0, 0, 3, 1, 'x', 1, 1}, {5, 0, 0, 9}, {5, 0, 0, 1, 127}} {
		t.Run(fmt.Sprint(reply), func(t *testing.T) {
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			defer l.Close()
			go func() {
				c, e := l.Accept()
				if e != nil {
					return
				}
				defer c.Close()
				b := make([]byte, 3)
				io.ReadFull(c, b)
				c.Write([]byte{5, 0})
				b = make([]byte, 10)
				io.ReadFull(c, b)
				c.Write(reply)
			}()
			if c, e := Dial(context.Background(), l.Addr().String()); e == nil {
				c.Close()
				t.Fatal("malformed reply accepted")
			}
		})
	}
}
func TestHandshakeCancellation(t *testing.T) {
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer l.Close()
	done := make(chan struct{})
	go func() {
		c, e := l.Accept()
		if e != nil {
			return
		}
		defer c.Close()
		io.Copy(io.Discard, c)
		close(done)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, e := Dial(ctx, l.Addr().String()); e == nil {
		t.Fatal("timeout ignored")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TCP control connection leaked")
	}
}
func TestAddressRoundTripBounds(t *testing.T) {
	for _, a := range []net.Addr{&net.UDPAddr{IP: net.ParseIP("::1"), Port: 443}, DomainAddr{Host: "relay.example", Port: 65535}} {
		b, e := encodeAddr(a)
		if e != nil {
			t.Fatal(e)
		}
		payload, target, e := decode(append(append([]byte{0, 0, 0}, b...), []byte("x")...))
		if e != nil || target.String() != a.String() || string(payload) != "x" {
			t.Fatalf("%v %v", target, e)
		}
	}
	for _, a := range []net.Addr{DomainAddr{Host: "a", Port: 0}, DomainAddr{Host: strings.Repeat("x", 256), Port: 1}, &net.UDPAddr{IP: net.IPv4(1, 2, 3, 4), Port: 65536}} {
		if _, e := encodeAddr(a); e == nil {
			t.Fatal("invalid target accepted")
		}
	}
}
func TestUsernamePasswordAuthentication(t *testing.T) {
	for _, accept := range []bool{true, false} {
		l, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		udp, e := net.ListenPacket("udp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		go func() {
			c, e := l.Accept()
			if e != nil {
				return
			}
			defer c.Close()
			greeting := make([]byte, 3)
			if _, e = io.ReadFull(c, greeting); e != nil || !bytes.Equal(greeting, []byte{5, 1, 2}) {
				return
			}
			c.Write([]byte{5, 2})
			auth := make([]byte, 2+4+1+6)
			if _, e = io.ReadFull(c, auth); e != nil {
				return
			}
			if !accept || !bytes.Equal(auth, []byte("\x01\x04user\x06secret")) {
				c.Write([]byte{1, 1})
				return
			}
			c.Write([]byte{1, 0})
			io.ReadFull(c, make([]byte, 10))
			a := udp.LocalAddr().(*net.UDPAddr)
			c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, byte(a.Port >> 8), byte(a.Port)})
			io.Copy(io.Discard, c)
		}()
		c, e := DialAuth(context.Background(), l.Addr().String(), "user", "secret")
		if accept != (e == nil) {
			t.Fatalf("accept=%v err=%v", accept, e)
		}
		if c != nil {
			c.Close()
		}
		l.Close()
		udp.Close()
	}
	if _, e := DialAuth(context.Background(), "127.0.0.1:1", "user", ""); e == nil {
		t.Fatal("partial credentials accepted")
	}
}
