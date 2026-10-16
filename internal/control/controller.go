package control

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"github.com/filiprek/conflux-vpn/internal/proxy"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Controller struct {
	mu       sync.Mutex
	store    Store
	engine   Engine
	profile  Profile
	dir      string
	ownerSID string
	busy     int
}

func New(s Store, e Engine, dir string) (*Controller, error) {
	p, err := s.Load()
	if err != nil {
		return nil, err
	}
	if p.Version == 0 {
		p.Version = 1
		p.MTU = 1100
	}
	if p.Version != 1 {
		return nil, errors.New("unsupported profile version")
	}
	return &Controller{store: s, engine: e, profile: p, dir: dir}, nil
}
func decode(b []byte, v any) error {
	if len(b) == 0 {
		b = []byte("{}")
	}
	if len(b) > MaxMessage {
		return errors.New("message too large")
	}
	if json.Unmarshal(b, v) != nil {
		return errors.New("invalid request parameters")
	}
	return nil
}
func (c *Controller) status() Status {
	s := c.engine.Status()
	s.Mode = "bond"
	s.MTU = c.profile.MTU
	existing := map[string]PathStatus{}
	for _, p := range s.Paths {
		existing[p.ID] = p
	}
	s.Paths = []PathStatus{}
	for _, p := range c.profile.Paths {
		v := existing[p.ID]
		v.ID = p.ID
		v.Name = p.Name
		v.Enabled = p.Enabled
		if v.State == "" {
			v.State = "idle"
		}
		if n, e := ParsePath(p); e == nil {
			v.Endpoint = fmt.Sprintf("%s:%d", n.Server(), n.Port())
		}
		s.Paths = append(s.Paths, v)
	}
	if c.profile.RelayURI != "" {
		if e, err := protocol.ParseEnrollment(c.profile.RelayURI); err == nil {
			s.Relay = &RelayStatus{Name: "Relay", Address: e.Address, Fingerprint: hex.EncodeToString(e.Pin[:])}
		}
	}
	return s
}
func (c *Controller) Handle(ctx context.Context, q Request) (any, error) {
	if q.Version != 1 || len(q.ID) > 128 || len(q.Params) > MaxMessage {
		return nil, errors.New("unsupported protocol version or message size")
	}
	if q.Method == "connect" || q.Method == "disconnect" || q.Method == "recover" || q.Method == "testPath" || q.Method == "testRelay" {
		c.mu.Lock()
		p := c.profile
		p.Paths = append([]Path(nil), p.Paths...)
		c.busy++
		c.mu.Unlock()
		defer func() { c.mu.Lock(); c.busy--; c.mu.Unlock() }()
		var err error
		switch q.Method {
		case "connect":
			err = c.engine.Connect(ctx, p)
		case "disconnect":
			err = c.engine.Disconnect()
		case "recover":
			err = errors.Join(c.engine.Disconnect(), c.engine.Recover())
		default:
			var v struct {
				ID string `json:"id"`
			}
			if err = decode(q.Params, &v); err != nil {
				return nil, err
			}
			if q.Method == "testRelay" {
				v.ID = "relay"
			}
			err = c.engine.Test(ctx, p, v.ID)
		}
		if err != nil {
			return nil, err
		}
		if q.Method == "testPath" || q.Method == "testRelay" {
			return map[string]bool{"ok": true}, nil
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.status(), nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if q.Version != 1 || len(q.ID) > 128 || len(q.Params) > MaxMessage {
		return nil, errors.New("unsupported protocol version or message size")
	}
	if q.Method == "status" {
		return c.status(), nil
	}
	if q.Method == "diagnostics" {
		return c.diagnostics()
	}
	if c.busy > 0 || c.engine.Status().State != "disconnected" {
		return nil, errors.New("disconnect before changing configuration")
	}
	next := c.profile
	next.Paths = append([]Path(nil), next.Paths...)
	switch q.Method {
	case "importRelay":
		var v struct {
			URI string `json:"uri"`
		}
		if err := decode(q.Params, &v); err != nil {
			return nil, err
		}
		if _, err := protocol.ParseEnrollment(v.URI); err != nil {
			return nil, errors.New("invalid relay enrollment string")
		}
		next.RelayURI = v.URI
	case "importPath":
		var v struct {
			URI, Name string
			Outbound  json.RawMessage `json:"outbound"`
		}
		if err := decode(q.Params, &v); err != nil {
			return nil, err
		}
		if len(next.Paths) >= 8 {
			return nil, errors.New("maximum eight paths")
		}
		n, err := ParsePath(Path{URI: v.URI, Outbound: v.Outbound})
		if err != nil {
			return nil, err
		}
		if len(v.Name) > 80 {
			return nil, errors.New("path name too long")
		}
		if strings.TrimSpace(v.Name) == "" {
			v.Name = n.Name()
		}
		var b [8]byte
		if _, err = rand.Read(b[:]); err != nil {
			return nil, err
		}
		next.Paths = append(next.Paths, Path{ID: hex.EncodeToString(b[:]), Name: v.Name, URI: v.URI, Outbound: v.Outbound, Enabled: true})
	case "updatePath":
		var v struct {
			ID, Name string
			Enabled  bool
			URI      string
		}
		if err := decode(q.Params, &v); err != nil {
			return nil, err
		}
		found := false
		for i := range next.Paths {
			if next.Paths[i].ID == v.ID {
				if len(v.Name) > 80 {
					return nil, errors.New("path name too long")
				}
				next.Paths[i].Name = v.Name
				next.Paths[i].Enabled = v.Enabled
				if v.URI != "" {
					if _, err := proxy.ParseVLESS(v.URI); err != nil {
						return nil, err
					}
					next.Paths[i].URI = v.URI
					next.Paths[i].Outbound = nil
				}
				found = true
			}
		}
		if !found {
			return nil, errors.New("path not found")
		}
	case "deletePath":
		var v struct{ ID string }
		if err := decode(q.Params, &v); err != nil {
			return nil, err
		}
		found := false
		for i, p := range next.Paths {
			if p.ID == v.ID {
				next.Paths = append(next.Paths[:i], next.Paths[i+1:]...)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.New("path not found")
		}
	case "configure":
		var v struct {
			Mode string
			MTU  int
		}
		if err := decode(q.Params, &v); err != nil {
			return nil, err
		}
		if v.Mode != "" && v.Mode != "bond" {
			return nil, errors.New("only Bond mode is supported")
		}
		if v.MTU < 576 || v.MTU > 1100 {
			return nil, errors.New("MTU must be 576..1100")
		}
		next.MTU = v.MTU
	default:
		return nil, errors.New("unknown service method")
	}
	if err := c.store.Save(next); err != nil {
		return nil, err
	}
	c.profile = next
	return c.status(), nil
}
func (c *Controller) diagnostics() (any, error) {
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(c.dir, "conflux-diagnostics-*.zip")
	if err != nil {
		return nil, err
	}
	_ = f.Chmod(0600)
	name := f.Name()
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(name)
		}
	}()
	z := zip.NewWriter(f)
	w, err := z.Create("status.json")
	if err != nil {
		return nil, err
	}
	s := c.status()
	for i := range s.Paths {
		s.Paths[i].Name = "<redacted>"
		s.Paths[i].Endpoint = "<redacted>"
		s.Paths[i].LastError = ""
	}
	if s.Relay != nil {
		s.Relay.Name = "<redacted>"
		s.Relay.Address = "<redacted>"
	}
	s.LastError = ""
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	if _, err = w.Write(b); err != nil {
		return nil, err
	}
	w, err = z.Create("manifest.json")
	if err != nil {
		return nil, err
	}
	if _, err = fmt.Fprintf(w, `{"schema":1,"protocol":1,"created":%q,"credentials":"excluded"}`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return nil, err
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	// The archive is removed by the deferred cleanup unless its ACL is applied.
	if err = permitExport(name, c.ownerSID); err != nil {
		return nil, err
	}
	ok = true
	abs, err := filepath.Abs(name)
	if err != nil {
		return nil, err
	}
	return map[string]string{"path": abs}, nil
}

// ParsePath normalizes either supported import form without exposing its credentials.
func ParsePath(p Path) (proxy.Node, error) {
	if len(p.Outbound) > 0 {
		if p.URI != "" {
			return proxy.Node{}, errors.New("choose one path import format")
		}
		return proxy.ParseAdvancedOutbound(p.Outbound)
	}
	return proxy.ParseVLESS(p.URI)
}
