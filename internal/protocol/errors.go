package protocol

import "errors"

// ErrPacketDropped indicates bounded device backpressure, not a fatal TUN failure.
var ErrPacketDropped = errors.New("packet dropped due to device backpressure")
