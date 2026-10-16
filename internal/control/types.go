// Package control implements the local versioned desktop/service API.
package control

import (
	"context"
	"encoding/json"
)

const MaxMessage = 64 << 10

type Request struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type Response struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   string `json:"error,omitempty"`
}
type Path struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	URI      string          `json:"uri,omitempty"`
	Outbound json.RawMessage `json:"outbound,omitempty"`
	Enabled  bool            `json:"enabled"`
}

// Profile is secret material and may only be persisted through Store's encrypted implementation.
type Profile struct {
	Version  int    `json:"version"`
	RelayURI string `json:"relayURI"`
	Paths    []Path `json:"paths"`
	MTU      int    `json:"mtu"`
}
type RelayStatus struct {
	Name        string `json:"name"`
	Address     string `json:"address"`
	Fingerprint string `json:"fingerprint"`
}
type PathStatus struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Enabled        bool     `json:"enabled"`
	State          string   `json:"state"`
	Endpoint       string   `json:"endpoint,omitempty"`
	RTTMs          float64  `json:"rttMs,omitempty"`
	LossPercent    *float64 `json:"lossPercent,omitempty"`
	BytesPerSecond *float64 `json:"bytesPerSecond,omitempty"`
	TxBytes        uint64   `json:"txBytes,omitempty"`
	RxBytes        uint64   `json:"rxBytes,omitempty"`
	LastError      string   `json:"lastError,omitempty"`
}
type Status struct {
	ServiceVersion  string       `json:"serviceVersion,omitempty"`
	ProtocolVersion string       `json:"protocolVersion,omitempty"`
	BytesPerSecond  *float64     `json:"bytesPerSecond,omitempty"`
	State           string       `json:"state"`
	Relay           *RelayStatus `json:"relay,omitempty"`
	Paths           []PathStatus `json:"paths"`
	Mode            string       `json:"mode"`
	MTU             int          `json:"mtu"`
	UptimeSeconds   float64      `json:"uptimeSeconds,omitempty"`
	TxBytes         uint64       `json:"txBytes,omitempty"`
	RxBytes         uint64       `json:"rxBytes,omitempty"`
	LastError       string       `json:"lastError,omitempty"`
}
type Store interface {
	Load() (Profile, error)
	Save(Profile) error
}
type Engine interface {
	Connect(context.Context, Profile) error
	Disconnect() error
	Status() Status
	Test(context.Context, Profile, string) error
	Recover() error
}
