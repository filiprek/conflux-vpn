package control

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"
)

func containsSecret(b, s []byte) bool { return bytes.Contains(b, s) }
func checkDiagnostic(t *testing.T, path, secret string) {
	t.Helper()
	z, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) == 0 {
		t.Fatal("empty diagnostics")
	}
	for _, f := range z.File {
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		b, e := io.ReadAll(r)
		r.Close()
		if e != nil {
			t.Fatal(e)
		}
		if containsSecret(b, []byte(secret)) {
			t.Fatal("secret in archive")
		}
	}
}
