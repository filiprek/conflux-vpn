package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func executableFixture(t *testing.T, body string) Executable {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture requires Unix")
	}
	p := filepath.Join(t.TempDir(), "sing-box")
	s := "#!/bin/sh\ncase \"$1\" in\nversion) echo 'sing-box version 1.14.3';;\ncheck) exit 0;;\nrun) " + body + ";;\nesac\n"
	if e := os.WriteFile(p, []byte(s), 0700); e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256([]byte(s))
	return Executable{Path: p, SHA256: hex.EncodeToString(sum[:]), Version: "1.14.3"}
}
func TestSupervisorBoundsRestartsAndRedacts(t *testing.T) {
	exe := executableFixture(t, "echo 'vless://123e4567-e89b-12d3-a456-426614174000@host:443 secret-key'; exit 7")
	s, e := NewSupervisor(exe, Options{MaxRestarts: 2, Backoff: time.Millisecond, MaxLogBytes: 128})
	if e != nil {
		t.Fatal(e)
	}
	n, _ := ParseVLESS(baseURI + "?security=tls")
	config, _ := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n}})
	e = s.Run(context.Background(), config)
	if e == nil {
		t.Fatal("crash ignored")
	}
	status := s.Status()
	if status.Attempts != 3 || status.State != "failed" {
		t.Fatalf("%+v", status)
	}
	if strings.Contains(status.Log, "123e4567") || strings.Contains(status.Log, "secret-key") {
		t.Fatal("secret leaked")
	}
	if len(status.Log) > 128 {
		t.Fatal("unbounded logs")
	}
}
func TestSupervisorVerifiesHashAndVersion(t *testing.T) {
	exe := executableFixture(t, "exit 0")
	exe.SHA256 = strings.Repeat("0", 64)
	s, e := NewSupervisor(exe, Options{})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Run(context.Background(), []byte(`{}`)); e == nil {
		t.Fatal("hash not checked")
	}
	exe = executableFixture(t, "exit 0")
	exe.Version = "1.14.2"
	if _, e = NewSupervisor(exe, Options{}); e == nil {
		t.Fatal("version not pinned")
	}
}
func TestSupervisorCancellation(t *testing.T) {
	exe := executableFixture(t, "exec sleep 30")
	s, e := NewSupervisor(exe, Options{})
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if e = s.Run(ctx, []byte(`{}`)); e == nil {
		t.Fatal("cancellation ignored")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("child not reaped promptly")
	}
}
func TestSupervisorUsesStdin(t *testing.T) {
	exe := executableFixture(t, "test \"$3\" = stdin || exit 20; grep -q 'inbounds' || exit 21; exit 7")
	s, e := NewSupervisor(exe, Options{})
	if e != nil {
		t.Fatal(e)
	}
	n, _ := ParseVLESS(baseURI + "?security=tls")
	b, _ := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n}})
	if e = s.Run(context.Background(), b); e == nil {
		t.Fatal("exit expected")
	}
	if s.Status().ExitCode != 7 {
		t.Fatalf("config not supplied on stdin: %+v", s.Status())
	}
}
