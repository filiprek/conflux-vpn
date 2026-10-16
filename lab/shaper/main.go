// A lab-only real TCP byte forwarder. Each listener has a shared byte-rate cap
// in each direction, including across connections. It never parses VLESS/QUIC.
package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type limiter struct {
	mu       sync.Mutex
	next     time.Time
	rate     int
	disabled *atomic.Bool
}

func (l *limiter) wait(ctx context.Context, n int) error {
	if l.disabled.Load() {
		return nil
	}
	l.mu.Lock()
	now := time.Now()
	if l.next.Before(now) {
		l.next = now
	}
	l.next = l.next.Add(time.Duration(n) * time.Second / time.Duration(l.rate))
	delay := time.Until(l.next)
	l.mu.Unlock()
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func pacedCopy(ctx context.Context, dst io.Writer, src io.Reader, l *limiter) error {
	b := make([]byte, 4096)
	for {
		n, e := src.Read(b)
		if n > 0 {
			if err := l.wait(ctx, n); err != nil {
				return err
			}
			for off := 0; off < n; {
				m, err := dst.Write(b[off:n])
				if err != nil {
					return err
				}
				if m == 0 {
					return io.ErrShortWrite
				}
				off += m
			}
		}
		if e != nil {
			return e
		}
	}
}
func serve(ctx context.Context, listener net.Listener, target string, rate int, disabled *atomic.Bool) error {
	var wg sync.WaitGroup
	stop := context.AfterFunc(ctx, func() { listener.Close() })
	defer stop()
	defer wg.Wait()
	up, down := &limiter{rate: rate, disabled: disabled}, &limiter{rate: rate, disabled: disabled}
	slots := make(chan struct{}, 64)
	for {
		a, e := listener.Accept()
		if e != nil {
			return e
		}
		select {
		case slots <- struct{}{}:
		default:
			a.Close()
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-slots }()
			defer a.Close()
			b, e := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", target)
			if e != nil {
				return
			}
			defer b.Close()
			stop := context.AfterFunc(ctx, func() { a.Close(); b.Close() })
			defer stop()
			done := make(chan struct{})
			go func() { pacedCopy(ctx, b, a, up); a.Close(); b.Close(); close(done) }()
			pacedCopy(ctx, a, b, down)
			a.Close()
			b.Close()
			<-done
		}()
	}
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var disabled atomic.Bool
	var wg sync.WaitGroup
	for i := range 2 {
		port := 19841 + i
		target := fmt.Sprintf("127.0.0.1:%d", 19851+i)
		l, e := net.Listen("tcp4", fmt.Sprintf(":%d", port))
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		wg.Add(1)
		go func() { defer wg.Done(); serve(ctx, l, target, 250000, &disabled) }()
	}
	fmt.Println("real TCP byte shaper: each VLESS port capped250000bytes/s in each direction")
	for {
		if _, e := os.Stat("/shared/coord/unshape"); e == nil {
			disabled.Store(true)
			os.WriteFile("/shared/coord/unshaped", []byte("ready"), 0600)
			break
		}
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-time.After(50 * time.Millisecond):
		}
	}
	<-ctx.Done()
	wg.Wait()
}
