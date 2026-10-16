package proxy

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const baseURI = "vless://123e4567-e89b-12d3-a456-426614174000@example.com:443"

func TestVLESS(t *testing.T) {
	for _, q := range []string{"?security=tls&sni=example.com&alpn=h2%2Chttp%2F1.1&type=tcp#Hello%20World", "?security=reality&pbk=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA&sid=ab12&fp=chrome&type=tcp", "?security=tls&type=ws&host=cdn.example&path=%2Fa", "?security=tls&type=grpc&serviceName=svc"} {
		n, err := ParseVLESS(baseURI + q)
		if err != nil {
			t.Fatal(err)
		}
		if n.Server() != "example.com" {
			t.Fatal("server")
		}
		b, err := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n}})
		if err != nil {
			t.Fatal(err)
		}
		var config map[string]any
		if json.Unmarshal(b, &config) != nil {
			t.Fatal("json")
		}
		if strings.Contains(string(b), "experimental") {
			t.Fatal("API enabled")
		}
	}
}
func TestRejectVLESS(t *testing.T) {
	for _, s := range []string{"http://example.com", baseURI + "?insecure=1", baseURI + "?foo=x", baseURI + "?security=tls&security=none", baseURI + "?path=%zz", strings.Replace(baseURI, "123e4567", "bad", 1), strings.Replace(baseURI, ":443", ":0", 1), baseURI + "?security=reality", baseURI + "?type=kcp", baseURI + "?encryption=aes", baseURI + "?type=tcp&path=/ignored", baseURI + "?security=none&sni=ignored"} {
		if _, err := ParseVLESS(s); err == nil {
			t.Fatalf("accepted invalid case %q", s)
		}
	}
}
func TestConfigRejectsPortsAndIDs(t *testing.T) {
	n, _ := ParseVLESS(baseURI + "?security=tls")
	for _, paths := range [][]Path{{{ID: "a", Port: 0, Node: n}}, {{ID: "a", Port: 13001, Node: n}, {ID: "a", Port: 13002, Node: n}}, {{ID: "a", Port: 13001, Node: n}, {ID: "b", Port: 13001, Node: n}}} {
		if _, err := BuildConfig(paths); err == nil {
			t.Fatal("accepted invalid mapping")
		}
	}
}
func FuzzVLESS(f *testing.F) {
	f.Add(baseURI + "?security=tls")
	f.Add("vless://%zz")
	f.Fuzz(func(t *testing.T, s string) {
		n, e := ParseVLESS(s)
		if e == nil {
			if _, e := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n}}); e != nil {
				t.Fatal(e)
			}
		}
	})
}
func TestNodeFormattingRedacts(t *testing.T) {
	n, _ := ParseVLESS(baseURI + "?security=tls")
	for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
		if strings.Contains(fmt.Sprintf(format, n), "123e4567") {
			t.Fatal("credential formatting leak")
		}
	}
}
func TestAdvancedOutbound(t *testing.T) {
	good := []byte(`{"type":"vless","server":"example.com","server_port":443,"uuid":"123e4567-e89b-12d3-a456-426614174000","tls":{"enabled":true,"server_name":"example.com"},"transport":{"type":"ws","path":"/vpn","headers":{"Host":"cdn.example"}}}`)
	if _, e := ParseAdvancedOutbound(good); e != nil {
		t.Fatal(e)
	}
	for _, s := range []string{strings.Replace(string(good), `"enabled":true`, `"enabled":true,"insecure":true`, 1), strings.Replace(string(good), `"type":"vless"`, `"type":"vless","detour":"evil"`, 1), strings.Replace(string(good), `"enabled":true`, `"enabled":true,"certificate_path":"/tmp/file"`, 1), strings.Replace(string(good), `"server":"example.com"`, `"server":"example.com","server":"evil.example"`, 1)} {
		if _, e := ParseAdvancedOutbound([]byte(s)); e == nil {
			t.Fatal("unsafe advanced config accepted")
		}
	}
}
func TestAdvancedRejectsAmbiguousFields(t *testing.T) {
	for _, s := range []string{`{"TYPE":"vless","server":"example.com","server_port":443,"uuid":"123e4567-e89b-12d3-a456-426614174000"}`, `{"type":"vless","server":"example.com","server_port":443,"uuid":"123e4567-e89b-12d3-a456-426614174000","tls":{"enabled":true,"utls":{"enabled":true,"fingerprint":""}}}`, `{"type":"vless","server":"example.com","server_port":443,"uuid":"123e4567-e89b-12d3-a456-426614174000","tls":{"enabled":true,"alpn":["a,b"]}}`} {
		if _, e := ParseAdvancedOutbound([]byte(s)); e == nil {
			t.Fatal("ambiguous advanced field accepted")
		}
	}
}
func FuzzAdvancedOutbound(f *testing.F) {
	f.Add([]byte(`{"type":"vless"}`))
	f.Fuzz(func(t *testing.T, b []byte) { ParseAdvancedOutbound(b) })
}
func TestConfigRequiresCompleteInboundCredentials(t *testing.T) {
	n, _ := ParseVLESS(baseURI + "?security=tls")
	if _, err := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n, Username: "user"}}); err == nil {
		t.Fatal("accepted username without password")
	}
	b, err := BuildConfig([]Path{{ID: "a", Port: 13001, Node: n, Username: "user", Password: "secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"users":[{"password":"secret","username":"user"}]`) {
		t.Fatalf("inbound credentials missing: %s", b)
	}
}
