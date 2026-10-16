package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// ParseAdvancedOutbound accepts only a bounded VLESS outbound schema. File references,
// plugins, detours, TLS bypasses and unknown fields are rejected, including nested ones.
func ParseAdvancedOutbound(data []byte) (Node, error) {
	if len(data) > 16384 || !uniqueJSON(data) {
		return Node{}, invalid()
	}
	var o struct {
		Type   string `json:"type"`
		Server string `json:"server"`
		Port   uint16 `json:"server_port"`
		UUID   string `json:"uuid"`
		Flow   string `json:"flow"`
		Packet string `json:"packet_encoding"`
		TLS    *struct {
			Enabled    bool     `json:"enabled"`
			ServerName string   `json:"server_name"`
			ALPN       []string `json:"alpn"`
			UTLS       *struct {
				Enabled     bool   `json:"enabled"`
				Fingerprint string `json:"fingerprint"`
			} `json:"utls"`
			Reality *struct {
				Enabled   bool   `json:"enabled"`
				PublicKey string `json:"public_key"`
				ShortID   string `json:"short_id"`
			} `json:"reality"`
		} `json:"tls"`
		Transport *struct {
			Type    string            `json:"type"`
			Path    string            `json:"path"`
			Service string            `json:"service_name"`
			Headers map[string]string `json:"headers"`
		} `json:"transport"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&o) != nil || o.Type != "vless" {
		return Node{}, invalid()
	}
	q := url.Values{}
	if o.Flow != "" {
		q.Set("flow", o.Flow)
	}
	if o.Packet != "" {
		q.Set("packetEncoding", o.Packet)
	}
	if t := o.TLS; t != nil {
		if !t.Enabled {
			return Node{}, invalid()
		}
		q.Set("security", "tls")
		if t.ServerName != "" {
			q.Set("sni", t.ServerName)
		}
		if len(t.ALPN) > 0 {
			a := ""
			for i, v := range t.ALPN {
				if strings.Contains(v, ",") {
					return Node{}, invalid()
				}
				if i > 0 {
					a += ","
				}
				a += v
			}
			q.Set("alpn", a)
		}
		if t.UTLS != nil {
			if !t.UTLS.Enabled || t.UTLS.Fingerprint == "" {
				return Node{}, invalid()
			}
			q.Set("fp", t.UTLS.Fingerprint)
		}
		if t.Reality != nil {
			if !t.Reality.Enabled {
				return Node{}, invalid()
			}
			q.Set("security", "reality")
			q.Set("pbk", t.Reality.PublicKey)
			q.Set("sid", t.Reality.ShortID)
		}
	}
	if t := o.Transport; t != nil {
		if t.Type != "ws" && t.Type != "grpc" {
			return Node{}, invalid()
		}
		q.Set("type", t.Type)
		if t.Path != "" {
			q.Set("path", t.Path)
		}
		if t.Service != "" {
			q.Set("serviceName", t.Service)
		}
		for k, v := range t.Headers {
			if k != "Host" {
				return Node{}, invalid()
			}
			q.Set("host", v)
		}
	}
	u := url.URL{Scheme: "vless", User: url.User(o.UUID), Host: net.JoinHostPort(o.Server, strconv.Itoa(int(o.Port))), RawQuery: q.Encode()}
	return ParseVLESS(u.String())
}
func uniqueJSON(b []byte) bool {
	d := json.NewDecoder(bytes.NewReader(b))
	if scanValue(d, 0) != nil {
		return false
	}
	_, e := d.Token()
	return e == io.EOF
}
func scanValue(d *json.Decoder, depth int) error {
	if depth > 16 {
		return invalid()
	}
	t, e := d.Token()
	if e != nil {
		return e
	}
	delim, yes := t.(json.Delim)
	if !yes {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return e
			}
			s, yes := k.(string)
			if !yes || seen[s] || (s != "Host" && s != strings.ToLower(s)) {
				return invalid()
			}
			seen[s] = true
			if e = scanValue(d, depth+1); e != nil {
				return e
			}
		}
	case '[':
		for d.More() {
			if e = scanValue(d, depth+1); e != nil {
				return e
			}
		}
	default:
		return invalid()
	}
	_, e = d.Token()
	return e
}
