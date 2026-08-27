package logstore

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestQueryAndPrune 验证写入、分页查询与保留清理。
func TestQueryAndPrune(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(filepath.Join(dir, "logs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	s.Enqueue("INFO", "[INFO] hello")
	s.Enqueue("ERROR", "[ERROR] boom")
	time.Sleep(80 * time.Millisecond)

	items, total, err := s.Query(Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if total < 2 || len(items) < 2 {
		t.Fatalf("total=%d len=%d", total, len(items))
	}

	old := time.Now().Add(-48 * time.Hour)
	_, _ = s.db.Exec(`UPDATE log_entries SET ts=? WHERE id=1`, old.Format("2006-01-02 15:04:05"))
	n, err := s.Prune(time.Now().Add(-24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("prune=%d", n)
	}
}

// TestOpenCreatesFile 打开后应创建 logs.db 文件。
func TestOpenCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logs.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
