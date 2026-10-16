package client

import (
	"context"
	"io"
	"sync"
)

type probeDevice struct {
	done chan struct{}
	once sync.Once
}

func (d *probeDevice) Read([]byte) (int, error)    { <-d.done; return 0, io.EOF }
func (d *probeDevice) Write(b []byte) (int, error) { return len(b), nil }
func (d *probeDevice) Close() error                { d.once.Do(func() { close(d.done) }); return nil }

// Probe verifies TLS, pin, token and application DATAGRAM round trips without TUN.
func Probe(ctx context.Context, config Config) (Snapshot, error) {
	config.Device = &probeDevice{done: make(chan struct{})}
	config.MinPaths = len(config.Paths)
	c, err := New(config)
	if err != nil {
		return Snapshot{}, err
	}
	defer c.Close()
	if err = c.Start(ctx); err != nil {
		return c.Snapshot(), err
	}
	return c.Snapshot(), nil
}
