package security_test

import (
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

// TestSecurityHeadersAllowInlineWebUI 验证 CSP 允许内联脚本/样式（否则登录页白屏无反应）。
func TestSecurityHeadersAllowInlineWebUI(t *testing.T) {
	h := security.SecurityHeaders()
	csp := h["Content-Security-Policy"]
	if csp == "" {
		t.Fatal("缺少 CSP")
	}
	if !containsAll(csp, "script-src", "'unsafe-inline'") {
		t.Fatalf("CSP 须允许 script-src 'unsafe-inline'，否则内联登录 JS 被浏览器拦截: %s", csp)
	}
	if !containsAll(csp, "style-src", "'unsafe-inline'") {
		t.Fatalf("CSP 须允许 style-src 'unsafe-inline': %s", csp)
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
