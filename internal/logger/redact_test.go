package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRedactSensitiveInLogFile 验证含 password/private_key 的日志写盘后无明文。
func TestRedactSensitiveInLogFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	if err := Init(Config{Level: "info", File: logPath}); err != nil {
		t.Fatal(err)
	}
	defer Close()

	Info("login password=SecretPass123 private_key=abc123def456")
	Close()

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if strings.Contains(content, "SecretPass123") {
		t.Fatalf("日志文件仍含明文 password: %s", content)
	}
	if strings.Contains(content, "private_key=abc123def456") {
		t.Fatalf("日志文件仍含明文 private_key: %s", content)
	}
	if !strings.Contains(content, "[REDACTED]") {
		t.Fatalf("期望脱敏标记: %s", content)
	}
}

// TestRedactAuthorizationAndSessionToken 覆盖 Authorization 与 session= 会话值。
func TestRedactAuthorizationAndSessionToken(t *testing.T) {
	hexTok := strings.Repeat("ab", 32) // 64 hex
	in := "req Authorization: Bearer " + hexTok + " session=" + hexTok
	out := RedactSensitive(in)
	if strings.Contains(out, hexTok) {
		t.Fatalf("仍含明文 token: %s", out)
	}
	if !strings.Contains(out, "[REDACTED]") {
		t.Fatalf("期望脱敏标记: %s", out)
	}
	// host_id 长 hex 不得被误伤
	hostLine := "lan_registry host_id=" + hexTok + " count=1"
	got := RedactSensitive(hostLine)
	if !strings.Contains(got, hexTok) {
		t.Fatalf("host_id 不应被脱敏: %s", got)
	}
}
