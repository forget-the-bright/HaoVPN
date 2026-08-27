//go:build windows

package wintundll

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileMatches(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wintun.dll")
	data := []byte("fake-wintun-bytes")

	if fileMatches(p, data) {
		t.Fatal("missing file should not match")
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if !fileMatches(p, data) {
		t.Fatal("identical bytes should match")
	}
	if !fileMatches(p, append([]byte(nil), data...)) {
		t.Fatal("equal copy should match")
	}
}
