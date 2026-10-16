//go:build !windows

package main

import (
	"encoding/json"
	"errors"
)

func callService(_ []byte, _ string) (json.RawMessage, error) {
	return nil, errors.New("Conflux service requires Windows")
}
