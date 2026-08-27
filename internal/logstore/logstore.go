// Package logstore 提供结构化运行日志库（独立 SQLite），供 WebUI 分页检索与保留策略清理。
package logstore

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"haovpn/internal/logger"
)

// Entry 单条结构化日志。
type Entry struct {
	ID    int64     `json:"id"`
	Ts    time.Time `json:"ts"`
	Level string    `json:"level"`
	Line  string    `json:"line"`
}

// Query 历史日志查询条件。
type Query struct {
	Level   string
	Keyword string
	Since   time.Time
	Limit   int
	Offset  int
}

// Store 独立 logs.db 存储。
type Store struct {
	db   *sql.DB
	ch   chan pending
	done chan struct{}
	wg   sync.WaitGroup
}

type pending struct {
	level string
	line  string
	ts    time.Time
}

const schema = `
CREATE TABLE IF NOT EXISTS log_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts TEXT NOT NULL,
    level TEXT NOT NULL,
    line TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_log_entries_ts ON log_entries(ts);
CREATE INDEX IF NOT EXISTS idx_log_entries_level_ts ON log_entries(level, ts);
`

// Open 打开或创建 logs.db。
func Open(path string) (*Store, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open logs.db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate logs.db: %w", err)
	}
	s := &Store{
		db:   db,
		ch:   make(chan pending, 4096),
		done: make(chan struct{}),
	}
	s.wg.Add(1)
	go s.writerLoop()
	logger.Info("logstore opened: %s", path)
	return s, nil
}

// Close 停止写入协程并关闭数据库。
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}

// Enqueue 异步写入（队列满则丢弃并打 WARN，避免阻塞业务路径）。
func (s *Store) Enqueue(level, line string) {
	if s == nil {
		return
	}
	p := pending{level: strings.ToUpper(level), line: line, ts: time.Now().UTC()}
	select {
	case s.ch <- p:
	default:
		logger.Warn("logstore 写入队列已满，丢弃一条日志")
	}
}

func (s *Store) writerLoop() {
	defer s.wg.Done()
	for {
		select {
		case <-s.done:
			for {
				select {
				case p := <-s.ch:
					_ = s.insertOne(p)
				default:
					return
				}
			}
		case p := <-s.ch:
			_ = s.insertOne(p)
		}
	}
}

func (s *Store) insertOne(p pending) error {
	_, err := s.db.Exec(`INSERT INTO log_entries(ts, level, line) VALUES(?,?,?)`,
		p.ts.Format("2006-01-02 15:04:05"), p.level, p.line)
	return err
}

// Query 分页查询历史日志；返回条目与匹配总数。
func (s *Store) Query(q Query) ([]Entry, int, error) {
	if q.Limit <= 0 {
		q.Limit = 200
	}
	if q.Limit > 2000 {
		q.Limit = 2000
	}
	if q.Offset < 0 {
		q.Offset = 0
	}

	where := []string{"1=1"}
	args := []any{}
	if lv := strings.ToUpper(strings.TrimSpace(q.Level)); lv != "" && lv != "ALL" {
		where = append(where, "level=?")
		args = append(args, lv)
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		where = append(where, "line LIKE ?")
		args = append(args, "%"+kw+"%")
	}
	if !q.Since.IsZero() {
		where = append(where, "ts>=?")
		args = append(args, q.Since.UTC().Format("2006-01-02 15:04:05"))
	}
	wsql := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM log_entries WHERE `+wsql, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	qargs := append(append([]any{}, args...), q.Limit, q.Offset)
	rows, err := s.db.Query(`SELECT id, ts, level, line FROM log_entries WHERE `+wsql+
		` ORDER BY id DESC LIMIT ? OFFSET ?`, qargs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Level, &e.Line); err != nil {
			return nil, 0, err
		}
		e.Ts, _ = time.Parse("2006-01-02 15:04:05", ts)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Prune 删除早于 cutoff 的条目，返回删除行数。
func (s *Store) Prune(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM log_entries WHERE ts < ?`, cutoff.UTC().Format("2006-01-02 15:04:05"))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ParseLevelFromLine 从 logger 行 `[INFO] ...` 提取级别。
func ParseLevelFromLine(line string) string {
	if len(line) < 3 || line[0] != '[' {
		return "INFO"
	}
	end := strings.IndexByte(line, ']')
	if end <= 1 {
		return "INFO"
	}
	return strings.ToUpper(line[1:end])
}
