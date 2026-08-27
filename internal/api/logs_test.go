package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLogTailFromEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	var b strings.Builder
	for i := 1; i <= 5000; i++ {
		fmt.Fprintf(&b, "line-%d\n", i)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	lines, truncated, err := readLogTail(path, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Fatal("expected truncated")
	}
	if len(lines) != 50 {
		t.Fatalf("len=%d", len(lines))
	}
	if lines[0] != "line-4951" || lines[49] != "line-5000" {
		t.Fatalf("range wrong: first=%q last=%q", lines[0], lines[49])
	}
}

func TestReadLogTailSmallFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	_ = os.WriteFile(path, []byte("a\nb\nc\n"), 0o644)
	lines, truncated, err := readLogTail(path, 200)
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(lines) != 3 {
		t.Fatalf("trunc=%v len=%d", truncated, len(lines))
	}
}
