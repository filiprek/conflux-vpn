//go:build windows

package platform

import (
	"bytes"
	"testing"
)

func TestDPAPIRoundTripAndTamper(t *testing.T) {
	secret := []byte("test-only-credential")
	b, e := Protect(secret)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(b, secret) {
		t.Fatal("plaintext persisted")
	}
	p, e := Unprotect(b)
	if e != nil || !bytes.Equal(p, secret) {
		t.Fatal("DPAPI roundtrip", e)
	}
	b[len(b)/2] ^= 1
	if _, e = Unprotect(b); e == nil {
		t.Fatal("tamper accepted")
	}
}
