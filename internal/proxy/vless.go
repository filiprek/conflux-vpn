// Package proxy manages a separately installed sing-box executable; it does not link sing-box.
package proxy

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Node keeps imported credentials private. String deliberately never reveals them.
type Node struct {
	uuid, server, security, sni, fingerprint, key, sid, flow, network, host, path, service, packet, name string
	port                                                                                                 uint16
	alpn                                                                                                 []string
}

func (n Node) Server() string   { return n.server }
func (n Node) Port() uint16     { return n.port }
func (n Node) Name() string     { return n.name }
func (n Node) String() string   { return "VLESS node [redacted]" }
func (n Node) GoString() string { return n.String() }

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func invalid() error {
	return errors.New("invalid or unsupported VLESS configuration (credentials redacted)")
}
func ParseVLESS(raw string) (Node, error) {
	var n Node
	if len(raw) > 16384 || strings.ContainsAny(raw, "\r\n\x00") {
		return n, invalid()
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "vless" || u.Opaque != "" || u.User == nil || u.Path != "" {
		return n, invalid()
	}
	if _, ok := u.User.Password(); ok {
		return n, invalid()
	}
	n.uuid = u.User.Username()
	if !uuidPattern.MatchString(n.uuid) {
		return Node{}, invalid()
	}
	n.server = u.Hostname()
	if !validHost(n.server) {
		return Node{}, invalid()
	}
	p, e := strconv.ParseUint(u.Port(), 10, 16)
	if e != nil || p == 0 {
		return Node{}, invalid()
	}
	n.port = uint16(p)
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return Node{}, invalid()
	}
	allowed := map[string]bool{}
	for _, k := range []string{"encryption", "security", "sni", "alpn", "fp", "pbk", "sid", "flow", "type", "host", "path", "serviceName", "headerType", "packetEncoding"} {
		allowed[k] = true
	}
	for k, v := range q {
		if !allowed[k] || len(v) != 1 || len(v[0]) > 2048 || strings.ContainsAny(v[0], "\r\n\x00") {
			return Node{}, invalid()
		}
	}
	if v := q.Get("encryption"); v != "" && v != "none" {
		return Node{}, invalid()
	}
	n.security = q.Get("security")
	if n.security == "" {
		n.security = "none"
	}
	if n.security != "none" && n.security != "tls" && n.security != "reality" {
		return Node{}, invalid()
	}
	n.network = q.Get("type")
	if n.network == "" {
		n.network = "tcp"
	}
	if n.network != "tcp" && n.network != "ws" && n.network != "grpc" {
		return Node{}, invalid()
	}
	n.sni = q.Get("sni")
	if n.sni != "" && !validHost(n.sni) {
		return Node{}, invalid()
	}
	n.fingerprint = q.Get("fp")
	switch n.fingerprint {
	case "", "chrome", "firefox", "edge", "safari", "ios", "android", "random", "randomized":
	default:
		return Node{}, invalid()
	}
	if q.Get("alpn") != "" {
		n.alpn = strings.Split(q.Get("alpn"), ",")
		for _, v := range n.alpn {
			if len(v) == 0 || len(v) > 255 {
				return Node{}, invalid()
			}
		}
	}
	n.key = q.Get("pbk")
	n.sid = q.Get("sid")
	if n.security == "reality" {
		k, e := base64.RawURLEncoding.DecodeString(n.key)
		if e != nil || len(k) != 32 {
			return Node{}, invalid()
		}
		s, e := hex.DecodeString(n.sid)
		if e != nil || len(s) > 8 || n.fingerprint == "" {
			return Node{}, invalid()
		}
	} else if n.key != "" || n.sid != "" {
		return Node{}, invalid()
	}
	if n.security == "none" && (n.sni != "" || n.fingerprint != "" || len(n.alpn) > 0) {
		return Node{}, invalid()
	}
	n.flow = q.Get("flow")
	if n.flow != "" && (n.flow != "xtls-rprx-vision" || n.security == "none" || n.network != "tcp") {
		return Node{}, invalid()
	}
	n.host = q.Get("host")
	n.path = q.Get("path")
	n.service = q.Get("serviceName")
	if q.Get("headerType") != "" && q.Get("headerType") != "none" {
		return Node{}, invalid()
	}
	if n.network == "tcp" && (n.host != "" || n.path != "" || n.service != "") {
		return Node{}, invalid()
	}
	if n.network == "ws" && (n.service != "" || (n.path != "" && !strings.HasPrefix(n.path, "/"))) {
		return Node{}, invalid()
	}
	if n.network == "grpc" && (n.host != "" || n.path != "") {
		return Node{}, invalid()
	}
	n.packet = q.Get("packetEncoding")
	if n.packet != "" && n.packet != "xudp" && n.packet != "packetaddr" {
		return Node{}, invalid()
	}
	n.name = u.Fragment
	if len(n.name) > 256 || strings.ContainsAny(n.name, "\r\n\x00") {
		return Node{}, invalid()
	}
	return n, nil
}
func validHost(s string) bool {
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	if net.ParseIP(s) != nil {
		return true
	}
	for _, p := range strings.Split(s, ".") {
		if len(p) == 0 || len(p) > 63 || p[0] == '-' || p[len(p)-1] == '-' {
			return false
		}
		for _, c := range p {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

type Path struct {
	ID   string
	Port uint16
	Node Node
}

func BuildConfig(paths []Path) ([]byte, error) {
	if len(paths) == 0 || len(paths) > 64 {
		return nil, invalid()
	}
	ids := map[string]bool{}
	ports := map[uint16]bool{}
	in := []any{}
	out := []any{}
	rules := []any{}
	for _, p := range paths {
		n := p.Node
		if p.ID == "" || len(p.ID) > 64 || ids[p.ID] || p.Port == 0 || ports[p.Port] || n.uuid == "" {
			return nil, invalid()
		}
		ids[p.ID] = true
		ports[p.Port] = true
		it, ot := "in-"+p.ID, "out-"+p.ID
		in = append(in, map[string]any{"type": "socks", "tag": it, "listen": "127.0.0.1", "listen_port": p.Port})
		o := map[string]any{"type": "vless", "tag": ot, "server": n.server, "server_port": n.port, "uuid": n.uuid}
		if n.flow != "" {
			o["flow"] = n.flow
		}
		if n.packet != "" {
			o["packet_encoding"] = n.packet
		}
		if n.security != "none" {
			tls := map[string]any{"enabled": true}
			if n.sni != "" {
				tls["server_name"] = n.sni
			}
			if len(n.alpn) > 0 {
				tls["alpn"] = n.alpn
			}
			if n.fingerprint != "" {
				tls["utls"] = map[string]any{"enabled": true, "fingerprint": n.fingerprint}
			}
			if n.security == "reality" {
				tls["reality"] = map[string]any{"enabled": true, "public_key": n.key, "short_id": n.sid}
			}
			o["tls"] = tls
		}
		if n.network == "ws" {
			tr := map[string]any{"type": "ws", "path": n.path}
			if n.host != "" {
				tr["headers"] = map[string]string{"Host": n.host}
			}
			o["transport"] = tr
		}
		if n.network == "grpc" {
			o["transport"] = map[string]any{"type": "grpc", "service_name": n.service}
		}
		out = append(out, o)
		rules = append(rules, map[string]any{"inbound": []string{it}, "action": "route", "outbound": ot})
	}
	// The default reject outbound prevents accidental fallback across path mappings.
	rules = append(rules, map[string]any{"action": "reject"})
	return json.Marshal(map[string]any{"log": map[string]any{"level": "error", "timestamp": true}, "dns": map[string]any{"servers": []any{map[string]any{"type": "local", "tag": "system"}}, "final": "system", "strategy": "ipv4_only"}, "inbounds": in, "outbounds": out, "route": map[string]any{"rules": rules, "default_domain_resolver": "system"}})
}
