package protocol

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"testing"
)

func TestEnrollment(t *testing.T) {
	cert, _, e := GenerateIdentity("127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	c, e := x509.ParseCertificate(cert)
	if e != nil {
		t.Fatal(e)
	}
	pin := sha256.Sum256(cert)
	q := url.Values{"cert": {base64.RawURLEncoding.EncodeToString(cert)}, "fp": {hex.EncodeToString(pin[:])}, "token": {"secret"}}
	e1, e := ParseEnrollment("conflux://127.0.0.1:4433?" + q.Encode())
	if e != nil {
		t.Fatal(e)
	}
	tc, e := e1.TLSConfig()
	if e != nil || tc.InsecureSkipVerify || tc.RootCAs == nil || tc.ServerName != "127.0.0.1" {
		t.Fatalf("TLS %+v %v", tc, e)
	}
	if _, e = c.Verify(x509.VerifyOptions{Roots: tc.RootCAs, DNSName: tc.ServerName}); e != nil {
		t.Fatal(e)
	}
	q.Set("fp", hex.EncodeToString(make([]byte, 32)))
	if _, e = ParseEnrollment("conflux://127.0.0.1:4433?" + q.Encode()); e == nil {
		t.Fatal("wrong pin accepted")
	}
}
func FuzzEnrollment(f *testing.F) {
	f.Add("conflux://127.0.0.1:4433?token=invalid")
	f.Fuzz(func(t *testing.T, s string) {
		e, err := ParseEnrollment(s)
		if err == nil {
			if _, err := ParseEnrollment(e.String()); err != nil {
				t.Fatal("valid enrollment cannot be reencoded")
			}
		}
	})
}
func TestEnrollmentRejectsInvalidEndpoint(t *testing.T) {
	der, _, err := GenerateIdentity("127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	e := Enrollment{Token: "secret", Certificate: der, Pin: sha256.Sum256(der)}
	for _, address := range []string{"127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:not-a-port", ":4433"} {
		e.Address = address
		if _, err := ParseEnrollment(e.String()); err == nil {
			t.Fatalf("accepted invalid endpoint %s", address)
		}
	}
}
