//go:build !linux && !windows

package tunnel

import (
	"errors"
	"io"
)

func Open(name string, mtu int) (io.ReadWriteCloser, error) {
	return nil, errors.New("unsupported tunnel platform")
}
