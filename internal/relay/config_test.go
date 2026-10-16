package relay

import (
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"os"
	"path/filepath"
	"testing"
)

func TestInitializeConfiguration(t *testing.T) {
	p := filepath.Join(t.TempDir(), "relay.json")
	enrollment, e := Initialize(p, "127.0.0.1", ":4433")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := protocol.ParseEnrollment(enrollment); e != nil {
		t.Fatal(e)
	}
	c, e := LoadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.TLSConfig(); e != nil {
		t.Fatal(e)
	}
	if len(c.Token) < 32 || c.MTU != 1100 {
		t.Fatal("unsafe defaults")
	}
	st, _ := os.Stat(p)
	if st.Mode().Perm() != 0600 {
		t.Fatal("config permissions")
	}
	if _, e = Initialize(p, "127.0.0.1", ":4433"); e == nil {
		t.Fatal("overwrote identity")
	}
	old := c.Token
	if _, e = RotateToken(p); e != nil {
		t.Fatal(e)
	}
	c, e = LoadFile(p)
	if e != nil || c.Token == old {
		t.Fatal("rotation failed")
	}
}
