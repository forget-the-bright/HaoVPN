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
