package security_test

import (
	"strings"
	"testing"

	"haovpn/internal/security"
)

func TestBindCheckRejectsWildcard(t *testing.T) {
	err := security.BindCheck([]string{"0.0.0.0"}, false)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBindCheckAllowsWithFlag(t *testing.T) {
	if err := security.BindCheck([]string{"0.0.0.0"}, true); err != nil {
		t.Fatal(err)
	}
}

func TestRedact(t *testing.T) {
	in := "password=secret123"
	out := security.Redact(in)
	if out == in {
		t.Fatalf("expected redaction")
	}
}

// TestSecurityHeadersScriptAndStyleSelf 验证 CSP：脚本与样式均仅 'self'，无 unsafe-inline。
func TestSecurityHeadersScriptAndStyleSelf(t *testing.T) {
	h := security.SecurityHeaders()
	csp := h["Content-Security-Policy"]
	if csp == "" {
		t.Fatal("缺少 CSP")
	}
	if !containsAll(csp, "script-src", "'self'") {
		t.Fatalf("CSP 须含 script-src 'self': %s", csp)
	}
	if !containsAll(csp, "style-src", "'self'") {
		t.Fatalf("CSP 须含 style-src 'self': %s", csp)
	}
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Fatalf("CSP 不得含 unsafe-inline（脚本/样式已外置）: %s", csp)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !containsSub(s, p) {
			return false
		}
	}
	return true
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
