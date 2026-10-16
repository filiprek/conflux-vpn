package control

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

type memoryStore struct{ p Profile }

func (s *memoryStore) Load() (Profile, error) { return s.p, nil }
func (s *memoryStore) Save(p Profile) error   { s.p = p; return nil }

type testEngine struct{ connected bool }

func (e *testEngine) Connect(context.Context, Profile) error { e.connected = true; return nil }
func (e *testEngine) Disconnect() error                      { e.connected = false; return nil }
func (e *testEngine) Status() Status {
	s := "disconnected"
	if e.connected {
		s = "connected"
	}
	return Status{State: s, Paths: []PathStatus{}, Mode: "bond", MTU: 1280}
}
func (e *testEngine) Test(context.Context, Profile, string) error { return nil }
func (e *testEngine) Recover() error                              { return nil }
func TestRejectsUnsafeConfigurationAndUnknownMethods(t *testing.T) {
	c, err := New(&memoryStore{}, &testEngine{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []Request{{Version: 2, Method: "status"}, {Version: 1, Method: "shell"}, {Version: 1, Method: "configure", Params: json.RawMessage(`{"mtu":9000}`)}} {
		if _, err = c.Handle(context.Background(), q); err == nil {
			t.Fatal(q)
		}
	}
}
func TestSecretDoesNotAppearInStatusOrDiagnostics(t *testing.T) {
	s := &memoryStore{}
	c, err := New(s, &testEngine{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	uri := "vless://12345678-1234-4234-8234-123456789abc@example.com:443?security=tls&sni=example.com#My%20node"
	p, _ := json.Marshal(map[string]string{"uri": uri})
	if _, err = c.Handle(context.Background(), Request{Version: 1, Method: "importPath", Params: p}); err != nil {
		t.Fatal(err)
	}
	result, err := c.Handle(context.Background(), Request{Version: 1, Method: "status"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(result)
	if containsSecret(b, []byte("12345678")) {
		t.Fatal("credential disclosed")
	}
	result, err = c.Handle(context.Background(), Request{Version: 1, Method: "diagnostics"})
	if err != nil {
		t.Fatal(err)
	}
	path := result.(map[string]string)["path"]
	checkDiagnostic(t, path, "12345678")
}

func TestAdvancedImportEncryptedAndRejectsDangerousFields(t *testing.T) {
	c, err := New(&memoryStore{}, &testEngine{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, outbound := range []string{`{"type":"vless","server":"example.com","server_port":443,"uuid":"12345678-1234-4234-8234-123456789abc","tls":{"enabled":true}}`, `{"type":"vless","server":"example.com","server_port":443,"uuid":"12345678-1234-4234-8234-123456789abc","detour":"untrusted"}`} {
		p := json.RawMessage(`{"name":"advanced","outbound":` + outbound + `}`)
		_, err = c.Handle(context.Background(), Request{Version: 1, Method: "importPath", Params: p})
		if containsSecret([]byte(outbound), []byte("detour")) {
			if err == nil {
				t.Fatal("unsafe outbound accepted")
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	s, _ := c.Handle(context.Background(), Request{Version: 1, Method: "status"})
	b, _ := json.Marshal(s)
	if containsSecret(b, []byte("12345678")) {
		t.Fatal("advanced credentials disclosed")
	}
}

type blockingEngine struct {
	testEngine
	started, release chan struct{}
	once             sync.Once
}

func (e *blockingEngine) Connect(ctx context.Context, p Profile) error {
	close(e.started)
	select {
	case <-e.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (e *blockingEngine) Disconnect() error { e.once.Do(func() { close(e.release) }); return nil }
func (e *blockingEngine) Status() Status {
	return Status{State: "disconnected", Paths: []PathStatus{}, Mode: "bond", MTU: 1100}
}
func TestDisconnectCanCancelInFlightConnect(t *testing.T) {
	e := &blockingEngine{started: make(chan struct{}), release: make(chan struct{})}
	c, err := New(&memoryStore{}, e, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connectDone := make(chan struct{})
	go func() { defer close(connectDone); _, _ = c.Handle(ctx, Request{Version: 1, Method: "connect"}) }()
	<-e.started
	done := make(chan error, 1)
	go func() { _, err := c.Handle(ctx, Request{Version: 1, Method: "disconnect"}); done <- err }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		cancel()
		t.Fatal("disconnect blocked behind connection attempt")
	}
	<-connectDone
}
