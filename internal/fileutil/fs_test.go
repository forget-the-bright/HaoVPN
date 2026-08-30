package fileutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"haovpn/internal/fileutil"
)

// TestExists 存在与空路径。
func TestExists(t *testing.T) {
	if fileutil.Exists("") {
		t.Fatal("空路径应 false")
	}
	p := filepath.Join(t.TempDir(), "f.txt")
	if fileutil.Exists(p) {
		t.Fatal("未创建应 false")
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileutil.Exists(p) {
		t.Fatal("已创建应 true")
	}
}

// TestAbsPair 必填 exe；可选 cfg。
func TestAbsPair(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "app.exe")
	cfg := filepath.Join(dir, "c.yaml")
	_ = os.WriteFile(exe, []byte("x"), 0o755)
	_ = os.WriteFile(cfg, []byte("y"), 0o600)
	ea, ca, err := fileutil.AbsPair(exe, cfg)
	if err != nil || ea == "" || ca == "" {
		t.Fatalf("got ea=%q ca=%q err=%v", ea, ca, err)
	}
	ea2, ca2, err := fileutil.AbsPair(exe, "")
	if err != nil || ea2 == "" || ca2 != "" {
		t.Fatalf("空 cfg 应只返回 exe abs=%q cfg=%q err=%v", ea2, ca2, err)
	}
}
