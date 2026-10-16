// Package protocol defines the bounded Conflux v1 wire protocol.
package protocol

import (
	"encoding/binary"
	"errors"
)

const (
	Version         = 1
	MaxMTU          = 1280
	DefaultMTU      = 1100
	HeaderSize      = 14
	Data       byte = 0
	Ping       byte = 1
	Pong       byte = 2
	ALPN            = "conflux/1"
)

var ErrFrame = errors.New("invalid Conflux datagram")

// Frame is an IP packet or a path-local probe. Sequence starts at one.
type Frame struct {
	Kind     byte
	Sequence uint64
	Payload  []byte
}

func Encode(f Frame) ([]byte, error) {
	if f.Kind > Pong || len(f.Payload) > MaxMTU || f.Sequence == 0 || (f.Kind != Data && len(f.Payload) != 0) {
		return nil, ErrFrame
	}
	b := make([]byte, HeaderSize+len(f.Payload))
	b[0] = Version
	b[1] = f.Kind
	binary.BigEndian.PutUint64(b[4:12], f.Sequence)
	binary.BigEndian.PutUint16(b[12:14], uint16(len(f.Payload)))
	copy(b[14:], f.Payload)
	return b, nil
}

// Decode borrows the payload from b and never allocates.
func Decode(b []byte) (Frame, error) {
	if len(b) < HeaderSize || len(b) > HeaderSize+MaxMTU || b[0] != Version || b[1] > Pong || b[2] != 0 || b[3] != 0 || int(binary.BigEndian.Uint16(b[12:14])) != len(b)-HeaderSize {
		return Frame{}, ErrFrame
	}
	f := Frame{Kind: b[1], Sequence: binary.BigEndian.Uint64(b[4:12]), Payload: b[14:]}
	if f.Sequence == 0 || (f.Kind != Data && len(f.Payload) != 0) {
		return Frame{}, ErrFrame
	}
	return f, nil
}
