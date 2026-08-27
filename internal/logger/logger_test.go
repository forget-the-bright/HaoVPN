package logger_test

import (
	"os"
	"path/filepath"
	"testing"

	"haovpn/internal/logger"
)

func TestParseLevel(t *testing.T) {
	tests := map[string]logger.Level{
		"trace": logger.LevelTrace,
		"DEBUG": logger.LevelDebug,
		"info":  logger.LevelInfo,
		"warn":  logger.LevelWarn,
		"error": logger.LevelError,
		"fatal": logger.LevelFatal,
		"":      logger.LevelInfo,
	}
	for in, want := range tests {
		if got := logger.ParseLevel(in); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestLoggerFileOutput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	l, err := logger.NewWithError(logger.Config{
		Level: "debug",
		File:  path,
	})
	if err != nil {
		t.Fatal(err)
	}
	l.Debug("hello %s", "world")
	live := l.LivePath()
	if live == "" {
		t.Fatal("应生成 live 日志路径")
	}
	// Sync 后不 Close 也应可读（观测场景）
	liveData, err := os.ReadFile(live)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(liveData), "hello world") {
		t.Fatalf("live 日志未同步写入: %s", liveData)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected log file content")
	}
}

// TestLiveLogPath 验证 server.log → server.live.log 命名。
func TestLiveLogPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "server.log")
	l, err := logger.NewWithError(logger.Config{Level: "info", File: path})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	want := filepath.Join(dir, "server.live.log")
	if l.LivePath() != want {
		t.Fatalf("LivePath=%q want %q", l.LivePath(), want)
	}
}

func TestErrorIncludesStack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "err.log")
	l, err := logger.NewWithError(logger.Config{Level: "error", File: path})
	if err != nil {
		t.Fatal(err)
	}
	l.Error("boom")
	_ = l.Close()
	data, _ := os.ReadFile(path)
	if !contains(string(data), "stack trace") {
		t.Fatalf("expected stack trace in log: %s", data)
	}
}

func contains(s, sub string) bool {
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
