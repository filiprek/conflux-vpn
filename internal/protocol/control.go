package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
)

const MaxControl = 4096

// Hello authenticates an independent path and attaches it to a logical tunnel.
type Hello struct {
	Version    int    `json:"version"`
	SessionID  string `json:"session"`
	InstanceID string `json:"instance"`
	PathID     string `json:"path"`
	Token      string `json:"token"`
	MTU        int    `json:"mtu"`
	Datagrams  bool   `json:"datagrams"`
}

func (h Hello) Validate() error {
	if h.Version != Version {
		return errors.New("unsupported Conflux protocol version")
	}
	if !h.Datagrams {
		return errors.New("QUIC DATAGRAM capability required")
	}
	for _, id := range []string{h.SessionID, h.InstanceID} {
		v, e := hex.DecodeString(id)
		if e != nil || len(v) != 16 {
			return errors.New("invalid session identifier")
		}
	}
	if len(h.PathID) == 0 || len(h.PathID) > 64 || len(h.Token) < 1 || len(h.Token) > 256 || h.MTU < 576 || h.MTU > MaxMTU {
		return errors.New("invalid path parameters")
	}
	return nil
}

// Welcome returns negotiated IPv4 ownership, MTU or a sanitized error.
type Welcome struct {
	Epoch   string `json:"epoch"`
	Version int    `json:"version"`
	IP      string `json:"ip"`
	MTU     int    `json:"mtu"`
	Error   string `json:"error,omitempty"`
}

func (w Welcome) Validate() error {
	if w.Error != "" {
		return errors.New(w.Error)
	}
	epoch, err := hex.DecodeString(w.Epoch)
	if err != nil || len(epoch) != 16 {
		return errors.New("invalid relay session epoch")
	}
	ip, e := netip.ParseAddr(w.IP)
	if e != nil || !ip.Is4() || w.Version != Version || w.MTU < 576 || w.MTU > MaxMTU {
		return errors.New("invalid relay welcome")
	}
	return nil
}
func WriteControl(w io.Writer, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	if len(b) > MaxControl {
		return errors.New("control frame too large")
	}
	var h [4]byte
	binary.BigEndian.PutUint32(h[:], uint32(len(b)))
	if e = writeAll(w, h[:]); e != nil {
		return e
	}
	return writeAll(w, b)
}
func ReadControl(r io.Reader, v any) error {
	var h [4]byte
	if _, e := io.ReadFull(r, h[:]); e != nil {
		return e
	}
	n := binary.BigEndian.Uint32(h[:])
	if n == 0 || n > MaxControl {
		return errors.New("control frame too large")
	}
	b := make([]byte, n)
	if _, e := io.ReadFull(r, b); e != nil {
		return e
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		return errors.New("malformed control frame")
	}
	if d.Decode(new(any)) != io.EOF {
		return errors.New("trailing control data")
	}
	return nil
}

// IPv4 validates lengths and rejects unsupported packets before TUN injection.
func IPv4(b []byte) (source, destination netip.Addr, err error) {
	if len(b) < 20 || len(b) > MaxMTU || b[0]>>4 != 4 || int(b[0]&15)*4 < 20 || int(b[0]&15)*4 > len(b) || int(binary.BigEndian.Uint16(b[2:4])) != len(b) {
		return source, destination, errors.New("invalid IPv4 packet")
	}
	return netip.AddrFrom4([4]byte(b[12:16])), netip.AddrFrom4([4]byte(b[16:20])), nil
}

func writeAll(w io.Writer, b []byte) error {
	n, e := w.Write(b)
	if e == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return e
}

// Command is a reliable session lifecycle operation following Welcome.
type Command struct {
	Type string `json:"type"`
}
