package singleinstance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"haovpn/internal/singleinstance"
)

// TestGUIDuplicateExits 第二次启动 client-gui 在锁失败提示后应退出（Windows 本地）。
func TestGUIDuplicateExits(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("GUI 子进程测试仅 Windows")
	}
	if os.Getenv("CI") == "true" {
		t.Skip("CI 无 GUI 环境")
	}

	lock, err := singleinstance.AcquireClient()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer lock.Release()

	bin := buildGUIBinary(t)
	cfg := filepath.Join(t.TempDir(), "client.yaml")
	cmd := exec.Command(bin, "-c", cfg)
	cmd.Env = append(os.Environ(), "HAOVPN_GUI_SKIP_DIALOG=1")
	if err := cmd.Start(); err != nil {
		t.Skipf("无法启动 GUI 子进程（环境限制）: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			t.Log("gui duplicate exited 0")
		} else if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 0 {
			t.Log("gui duplicate exited 0")
		} else {
			t.Fatalf("duplicate gui exit: %v", err)
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("duplicate gui did not exit within 15s")
	}
}

func buildGUIBinary(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(guiMainDir(t), "..", "..")
	out := filepath.Join(repo, "bin", "haovpn-client-gui-dup-test.exe")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = guiMainDir(t)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build client-gui: %v\n%s", err, b)
	}
	return out
}

func guiMainDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "cmd", "client-gui")
}
