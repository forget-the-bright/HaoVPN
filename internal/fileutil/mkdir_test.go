package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureParentDir(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "file.txt")
	if err := EnsureParentDir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil || !st.IsDir() {
		t.Fatalf("parent dir missing: %v", err)
	}
	if err := EnsureParentDir("file.txt", 0o755); err != nil {
		t.Fatal(err)
	}
}
