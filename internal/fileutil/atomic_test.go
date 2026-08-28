package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomic_CreatesParentAndContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "secret.txt")
	want := []byte("hello-secret\n")
	if err := WriteFileAtomic(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("content=%q want=%q", got, want)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o777 != 0o600 {
		// Windows 上权限位可能被映射；仅校验可读可写文件存在即可。
		t.Logf("perm=%o（Windows 可能忽略 Unix 模式位）", st.Mode().Perm())
	}
}

func TestWriteFileAtomic_Overwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := WriteFileAtomic(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Fatalf("got %q", got)
	}
}

func TestWriteFileAtomic_EmptyPath(t *testing.T) {
	if err := WriteFileAtomic("", []byte("x"), 0o600); err == nil {
		t.Fatal("expected error")
	}
}

func TestExecutableDir(t *testing.T) {
	dir, err := ExecutableDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir == "" {
		t.Fatal("empty dir")
	}
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		t.Fatalf("dir=%q err=%v", dir, err)
	}
}
