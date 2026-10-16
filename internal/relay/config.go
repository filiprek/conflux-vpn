package relay

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"github.com/filiprek/conflux-vpn/internal/protocol"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
)

// FileConfig is the bounded relay configuration stored with mode 0600.
type FileConfig struct {
	Listen      string `json:"listen"`
	CertFile    string `json:"cert_file"`
	KeyFile     string `json:"key_file"`
	Token       string `json:"token"`
	TUNName     string `json:"tun_name"`
	MTU         int    `json:"mtu"`
	Subnet      string `json:"subnet"`
	MaxSessions int    `json:"max_sessions"`
}

func LoadFile(path string) (FileConfig, error) {
	var c FileConfig
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 65537))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, errors.New("invalid relay configuration")
	}
	if d.Decode(new(any)) != io.EOF {
		return c, errors.New("trailing relay configuration")
	}
	if _, _, err = net.SplitHostPort(c.Listen); err != nil {
		return c, errors.New("invalid relay listen address")
	}
	if len(c.Token) < 32 || len(c.Token) > 256 || c.MTU < 576 || c.MTU > protocol.DefaultMTU || len(c.TUNName) < 1 || len(c.TUNName) > 15 || c.MaxSessions < 1 || c.MaxSessions > 1024 {
		return c, errors.New("invalid relay configuration parameters")
	}
	prefix, err := netip.ParsePrefix(c.Subnet)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 16 || prefix.Bits() > 24 {
		return c, errors.New("invalid relay subnet")
	}
	if !filepath.IsAbs(c.CertFile) {
		c.CertFile = filepath.Join(filepath.Dir(path), c.CertFile)
	}
	if !filepath.IsAbs(c.KeyFile) {
		c.KeyFile = filepath.Join(filepath.Dir(path), c.KeyFile)
	}
	if _, err = c.TLSConfig(); err != nil {
		return c, err
	}
	return c, nil
}
func (c FileConfig) TLSConfig() (*tls.Config, error) {
	identity, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
	if err != nil {
		return nil, errors.New("cannot load relay TLS identity")
	}
	return &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{identity}}, nil
}
func token() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
func writeExclusive(path string, b []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}

// Initialize creates a fresh identity without overwriting existing files.
func Initialize(path, host, listen string) (string, error) {
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return "", err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	path = abs
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		return "", errors.New("relay configuration already exists or is inaccessible")
	}
	der, key, err := protocol.GenerateIdentity(host)
	if err != nil {
		return "", err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", err
	}
	secret, err := token()
	if err != nil {
		return "", err
	}
	if err = os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		return "", err
	}
	c := FileConfig{Listen: listen, CertFile: filepath.Join(filepath.Dir(path), "relay.crt"), KeyFile: filepath.Join(filepath.Dir(path), "relay.key"), Token: secret, TUNName: "conflux0", MTU: protocol.DefaultMTU, Subnet: "10.250.0.0/16", MaxSessions: 128}
	files := []struct {
		path string
		b    []byte
		mode os.FileMode
	}{{c.CertFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0644}, {c.KeyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0600}}
	var created []string
	ok := false
	defer func() {
		if !ok {
			for _, p := range created {
				os.Remove(p)
			}
		}
	}()
	for _, f := range files {
		if err = writeExclusive(f.path, f.b, f.mode); err != nil {
			return "", err
		}
		created = append(created, f.path)
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err = writeExclusive(path, append(b, '\n'), 0600); err != nil {
		return "", err
	}
	created = append(created, path)
	ok = true
	_, port, _ := net.SplitHostPort(listen)
	return protocol.Enrollment{Address: net.JoinHostPort(host, port), Token: secret, Certificate: der, Pin: sha256.Sum256(der)}.String(), nil
}

// RotateToken updates only the token atomically. Restart the service afterwards.
func RotateToken(path string) (string, error) {
	c, err := LoadFile(path)
	if err != nil {
		return "", err
	}
	c.Token, err = token()
	if err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	tmp, err := os.CreateTemp(filepath.Dir(path), ".relay-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(append(b, '\n')); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return c.Enrollment()
}
func (c FileConfig) Enrollment() (string, error) {
	tc, err := c.TLSConfig()
	if err != nil {
		return "", err
	}
	der := tc.Certificates[0].Certificate[0]
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", err
	}
	host := ""
	if len(cert.DNSNames) > 0 {
		host = cert.DNSNames[0]
	} else if len(cert.IPAddresses) > 0 {
		host = cert.IPAddresses[0].String()
	} else {
		return "", errors.New("relay identity has no SAN")
	}
	_, port, _ := net.SplitHostPort(c.Listen)
	return protocol.Enrollment{Address: net.JoinHostPort(host, port), Token: c.Token, Certificate: der, Pin: sha256.Sum256(der)}.String(), nil
}
