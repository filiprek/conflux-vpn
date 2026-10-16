package protocol

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"net"
	"net/url"
	"strconv"
	"time"
)

// Enrollment is the relay trust material imported once through a protected channel.
type Enrollment struct {
	Address, Token string
	Pin            [32]byte
	Certificate    []byte
}

func ParseEnrollment(raw string) (Enrollment, error) {
	var e Enrollment
	if len(raw) > 16384 {
		return e, errors.New("relay enrollment too large")
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "conflux" && u.Scheme != "bondrelay") || u.User != nil || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return e, errors.New("invalid relay enrollment")
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" {
		return e, errors.New("relay requires host and port")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return e, errors.New("invalid relay port")
	}
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return e, errors.New("invalid relay parameters")
	}
	for key, values := range q {
		if (key != "cert" && key != "fp" && key != "token") || len(values) != 1 {
			return e, errors.New("unknown or duplicate relay parameter")
		}
	}
	e.Address = u.Host
	e.Token = q.Get("token")
	if len(e.Token) < 1 || len(e.Token) > 256 {
		return e, errors.New("invalid relay token")
	}
	pin, err := hex.DecodeString(q.Get("fp"))
	if err != nil || len(pin) != 32 {
		return e, errors.New("invalid relay fingerprint")
	}
	copy(e.Pin[:], pin)
	e.Certificate, err = base64.RawURLEncoding.DecodeString(q.Get("cert"))
	if err != nil || len(e.Certificate) > 8192 {
		return e, errors.New("invalid relay certificate")
	}
	if sha256.Sum256(e.Certificate) != e.Pin {
		return e, errors.New("relay certificate fingerprint mismatch")
	}
	if _, err = e.TLSConfig(); err != nil {
		return e, err
	}
	return e, nil
}
func (e Enrollment) TLSConfig() (*tls.Config, error) {
	cert, err := x509.ParseCertificate(e.Certificate)
	if err != nil {
		return nil, errors.New("invalid relay certificate")
	}
	host, _, err := net.SplitHostPort(e.Address)
	if err != nil {
		return nil, errors.New("invalid relay address")
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots, ServerName: host, NextProtos: []string{ALPN}}, nil
}
func (e Enrollment) String() string {
	q := url.Values{"fp": {hex.EncodeToString(e.Pin[:])}, "cert": {base64.RawURLEncoding.EncodeToString(e.Certificate)}, "token": {e.Token}}
	return "conflux://" + e.Address + "?" + q.Encode()
}

// GenerateIdentity creates a self-signed TLS server identity with a verified SAN.
func GenerateIdentity(host string) ([]byte, crypto.PrivateKey, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	// Clients pin this exact certificate. A short validity would silently break every
	// enrolled client on expiry, so identity rotation is an explicit re-enrollment instead.
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Conflux relay"}, NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(10, 0, 0), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else if host != "" {
		template.DNSNames = []string{host}
	} else {
		return nil, nil, errors.New("relay certificate host required")
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, pub, key)
	return der, key, err
}
