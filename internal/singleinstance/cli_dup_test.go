package singleinstance_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"haovpn/internal/singleinstance"
)

// TestCLIAlreadyRunningExit 持有单实例锁时，CLI 第二次启动应 stderr 提示并以 1 退出。
func TestCLIAlreadyRunningExit(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("CI") == "true" {
		t.Skip("Windows CI 无 GUI/交互环境，跳过 subprocess CLI 测试")
	}

	lock, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer lock.Release()

	bin := buildClientBinary(t)
	cmd := exec.Command(bin, "-c", filepath.Join(t.TempDir(), "nonexistent-client.yaml"))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err == nil {
		t.Fatal("expected non-zero exit when lock held")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.ExitCode() != 1 {
		t.Fatalf("exit code want 1, got %d stderr=%q", exitErr.ExitCode(), stderr.String())
	}
	out := stderr.String()
	if !strings.Contains(out, "已在运行") {
		t.Fatalf("stderr should mention 已在运行, got %q", out)
	}
}

func buildClientBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	name := "haovpn-client-test"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	out := filepath.Join(dir, name)
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = clientMainDir(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build client: %v\n%s", err, b)
	}
	return out
}

func clientMainDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/singleinstance -> repo root -> cmd/client
	return filepath.Join(filepath.Dir(file), "..", "..", "cmd", "client")
}
