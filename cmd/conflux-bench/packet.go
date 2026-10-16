package main

import (
	"encoding/binary"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"net/netip"
)

const (
	upload   byte = 1
	download byte = 2
	bidi     byte = 3
)

func checksum(b []byte) uint16 {
	sum := uint32(0)
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b))
		b = b[2:]
	}
	if len(b) > 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 65535) + (sum >> 16)
	}
	return ^uint16(sum)
}
func makePacket(src, dst netip.Addr, srcPort, dstPort uint16, mode byte, size int, seq uint64) []byte {
	payload := size
	if mode == download {
		payload = 16
	}
	p := make([]byte, 28+payload)
	p[0] = 0x45
	p[8] = 64
	p[9] = 17
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	copy(p[12:16], src.AsSlice())
	copy(p[16:20], dst.AsSlice())
	binary.BigEndian.PutUint16(p[10:12], checksum(p[:20]))
	binary.BigEndian.PutUint16(p[20:22], srcPort)
	binary.BigEndian.PutUint16(p[22:24], dstPort)
	binary.BigEndian.PutUint16(p[24:26], uint16(payload+8))
	copy(p[28:32], "CFXB")
	p[32] = mode
	binary.BigEndian.PutUint16(p[33:35], uint16(size))
	binary.BigEndian.PutUint64(p[36:44], seq)
	return p
}
func echoPayload(b []byte) ([]byte, error) {
	if len(b) < 16 || string(b[:4]) != "CFXB" || b[4] < upload || b[4] > bidi {
		return nil, errors.New("invalid benchmark request")
	}
	size := int(binary.BigEndian.Uint16(b[5:7]))
	if size < 16 || size > protocol.DefaultMTU-28 {
		return nil, errors.New("benchmark payload outside safe MTU")
	}
	if b[4] == upload {
		size = 16
	}
	out := make([]byte, size)
	copy(out, b[:16])
	return out, nil
}
func echoPacket(p []byte) ([]byte, error) {
	src, dst, err := protocol.IPv4(p)
	if err != nil || p[0] != 0x45 || p[9] != 17 || len(p) < 44 {
		return nil, errors.New("invalid benchmark IPv4 UDP packet")
	}
	payload, err := echoPayload(p[28:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 28+len(payload))
	copy(out, p[:28])
	copy(out[12:16], dst.AsSlice())
	copy(out[16:20], src.AsSlice())
	copy(out[20:22], p[22:24])
	copy(out[22:24], p[20:22])
	binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
	binary.BigEndian.PutUint16(out[24:26], uint16(8+len(payload)))
	out[10] = 0
	out[11] = 0
	out[26] = 0
	out[27] = 0
	binary.BigEndian.PutUint16(out[10:12], checksum(out[:20]))
	copy(out[28:], payload)
	return out, nil
}
