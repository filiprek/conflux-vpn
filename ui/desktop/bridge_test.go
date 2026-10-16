package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, body, id string
		ok             bool
	}{{"valid", `{"version":1,"id":"42","result":{"state":"connected"}}` + "\n", "42", true}, {"wrong id", `{"version":1,"id":"43","result":{}}` + "\n", "42", false}, {"wrong version", `{"version":2,"id":"42","result":{}}` + "\n", "42", false}, {"service error", `{"version":1,"id":"42","error":"denied"}` + "\n", "42", false}, {"unterminated", `{"version":1,"id":"42","result":{}}`, "42", false}, {"oversized", strings.Repeat("x", 65537), "42", false}} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeResponse(bytes.NewBufferString(tc.body), tc.id)
			if (err == nil) != tc.ok {
				t.Fatalf("success=%v, want %v", err == nil, tc.ok)
			}
		})
	}
}
func TestRejectUnknownMethod(t *testing.T) {
	_, err := (&App{}).Invoke("execute", map[string]any{})
	if err == nil {
		t.Fatal("unknown method accepted")
	}
}
func TestRejectOversizedRequest(t *testing.T) {
	_, err := (&App{}).Invoke("importPath", map[string]any{"uri": strings.Repeat("x", 65536)})
	if err == nil {
		t.Fatal("oversized request accepted")
	}
}
func TestPublicErrorRedactsSensitiveDetails(t *testing.T) {
	for in, want := range map[string]string{
		"MTU must be 576..1100":                     "MTU must be 576..1100",
		"network activation: failed\nrecovery done": "network activation: failed recovery done",
		"bad vless://uuid@example.com:443":          "service operation failed",
		`open C:\ProgramData\Conflux\secrets.dat`:   "service operation failed",
		strings.Repeat("x", 301):                    "service operation failed",
		"":                                          "service operation failed",
	} {
		if got := publicError(in); got != want {
			t.Fatalf("publicError(%q) = %q, want %q", in, got, want)
		}
	}
}
