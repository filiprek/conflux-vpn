package main

import (
	"io"
	"sync"
)

type packetDevice struct {
	input, output chan []byte
	done          chan struct{}
	once          sync.Once
	echo          bool
}

func newPacketDevice(echo bool) *packetDevice {
	return &packetDevice{input: make(chan []byte, 512), output: make(chan []byte, 512), done: make(chan struct{}), echo: echo}
}
func (d *packetDevice) Read(b []byte) (int, error) {
	select {
	case p := <-d.input:
		return copy(b, p), nil
	case <-d.done:
		return 0, io.EOF
	}
}
func (d *packetDevice) Write(p []byte) (int, error) {
	n := len(p)
	out := d.output
	data := append([]byte(nil), p...)
	if d.echo {
		var err error
		data, err = echoPacket(p)
		if err != nil {
			return n, nil
		}
		out = d.input
	}
	select {
	case out <- data:
		return n, nil
	case <-d.done:
		return 0, io.EOF
	}
}
func (d *packetDevice) Close() error { d.once.Do(func() { close(d.done) }); return nil }
