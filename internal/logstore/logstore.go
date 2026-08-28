package logstore

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"haovpn/internal/logger"
	"haovpn/internal/paginate"
	"haovpn/internal/timeutil"
)

// Entry 单条结构化历史日志记录，对应 log_entries 表一行。
//
// 字段：
//   ID — SQLite 自增主键；Query 按 id DESC 排序。
//   Ts — UTC 写入时间；从库中 ts 文本列解析。
//   Level — 大写级别名（INFO/WARN 等）。
//   Line — 完整 logger 行，含 `[LEVEL]` 前缀。
type Entry struct {
	ID    int64     `json:"id"`
	Ts    time.Time `json:"ts"`
	Level string    `json:"level"`
	Line  string    `json:"line"`
}

// Query 历史日志分页查询条件。
//
// 字段：
//   Level — 过滤级别；空或 ALL 表示不限。
//   Keyword — 对 line 列 LIKE 模糊匹配；空则忽略。
//   Since — 仅 ts ≥ Since（UTC）的条目；零值忽略。
//   Limit — 每页条数；≤0 默认 200，最大 2000。
//   Offset — 分页偏移；<0 视为 0。
type Query struct {
	Level   string
	Keyword string
	Since   time.Time
	Limit   int
	Offset  int
}

// Store 独立 logs.db 的异步写入与查询门面。
//
// 字段：
//   db — WAL 模式 SQLite；MaxOpenConns=1。
//   ch — 缓冲 pending 的 channel（容量 4096）；Enqueue 非阻塞。
//   done — Close 时关闭，writerLoop 刷队列后退出。
//   wg — 等待 writer goroutine 结束。
//
// 线程安全：Enqueue/Query/Close 可从多 goroutine 调用；写入在单 writer 串行化。
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

// Open 打开或创建 logs.db，迁移表结构并启动后台写入协程。
//
// 参数：path 为 logs.db 文件路径（可不存在）。
// 返回：*Store 与 err；Ping/Exec schema 失败时 Store 为 nil且 db 已关闭。
// 副作用：创建 WAL DB、启动 writerLoop goroutine、打 Info 日志。
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

// Close 停止接受新写入、刷尽队列并关闭数据库。
//
// 参数：无；重复 Close 会 panic（close done）。
// 返回：db.Close 的错误。
// 副作用：关闭 done、等待 writer、释放 SQLite 连接。
func (s *Store) Close() error {
	close(s.done)
	s.wg.Wait()
	return s.db.Close()
}

// Enqueue 非阻塞地将一条日志放入写入队列。
//
// 参数：level 会 ToUpper；line 为完整格式化行；s 为 nil 时 no-op。
// 返回：无；队列满时丢弃并 Warn，不阻塞业务 goroutine。
// 副作用：可能触发 writerLoop insertOne。
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
		timeutil.FormatUTC(p.ts), p.level, p.line)
	return err
}

// Query 按条件分页查询历史日志。
//
// 参数：q 见 Query；Limit/Offset 在方法内规范化。
// 返回：条目切片、匹配总数 total、SQL 错误；无匹配时 out 可为 nil 且 total=0。
// 副作用：只读查询 logs.db；ORDER BY id DESC。
func (s *Store) Query(q Query) ([]Entry, int, error) {
	q.Limit = paginate.ClampLimit(q.Limit, 200, 2000)
	q.Offset = paginate.ClampOffset(q.Offset)

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
		args = append(args, timeutil.FormatUTC(q.Since))
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
		e.Ts = timeutil.ParseUTC(ts)
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// Prune 删除 ts 早于 cutoff 的历史条目，用于 retention 策略。
//
// 参数：cutoff 按 UTC 格式化为 ts 列比较。
// 返回：实际删除行数与 err。
// 副作用：DELETE FROM log_entries；不可恢复。
func (s *Store) Prune(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM log_entries WHERE ts < ?`, timeutil.FormatUTC(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ParseLevelFromLine 从 logger 格式化行首提取级别名。
//
// 参数：line 期望形如 `[INFO] message`；格式不符时保守返回 INFO。
// 返回：大写级别字符串。
// 副作用：无。
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
