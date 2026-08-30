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

// TestSecurityHeadersScriptSelfStyleInline 验证 CSP：脚本仅 'self'，样式暂仍允许 unsafe-inline。
func TestSecurityHeadersScriptSelfStyleInline(t *testing.T) {
	h := security.SecurityHeaders()
	csp := h["Content-Security-Policy"]
	if csp == "" {
		t.Fatal("缺少 CSP")
	}
	if !containsAll(csp, "script-src", "'self'") {
		t.Fatalf("CSP 须含 script-src 'self': %s", csp)
	}
	// 仅检查 script-src 指令段，不得再含 unsafe-inline
	scriptPart := csp
	if idx := strings.Index(csp, "script-src"); idx >= 0 {
		scriptPart = csp[idx:]
		if end := strings.Index(scriptPart, ";"); end >= 0 {
			scriptPart = scriptPart[:end]
		}
	}
	if strings.Contains(scriptPart, "'unsafe-inline'") {
		t.Fatalf("CSP script-src 不得含 unsafe-inline（脚本已外置）: %s", csp)
	}
	if !containsAll(csp, "style-src", "'unsafe-inline'") {
		t.Fatalf("CSP 须允许 style-src 'unsafe-inline'（内联 style 尚未迁完）: %s", csp)
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
