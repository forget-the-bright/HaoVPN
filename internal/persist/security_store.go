package persist

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"haovpn/internal/paginate"
	"haovpn/internal/timeutil"
)

// InsertSecurityEvent 追加一条探针/拒绝安全事件。
//
// 副作用：写 security_events；失败返回 err（调用方通常只打日志，不阻断连接关闭）。
func (s *Store) InsertSecurityEvent(e SecurityEvent) error {
	_, err := s.db.Exec(
		`INSERT INTO security_events(client_ip, client_port, phase, signature, action, detail_json) VALUES(?,?,?,?,?,?)`,
		e.ClientIP, e.ClientPort, e.Phase, e.Signature, e.Action, e.DetailJSON,
	)
	return err
}

// SecurityEventFilter 安全事件分页筛选。
type SecurityEventFilter struct {
	ClientIP  string
	Signature string
	Limit     int
	Offset    int
}

// ListSecurityEvents 分页列出安全事件（id 降序）。
func (s *Store) ListSecurityEvents(f SecurityEventFilter) ([]SecurityEvent, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	where := []string{"1=1"}
	args := []any{}
	if ip := strings.TrimSpace(f.ClientIP); ip != "" {
		where = append(where, "client_ip=?")
		args = append(args, ip)
	}
	if sig := strings.TrimSpace(f.Signature); sig != "" {
		where = append(where, "signature=?")
		args = append(args, sig)
	}
	wsql := strings.Join(where, " AND ")
	var out []SecurityEvent
	total, err := s.queryPageTotal(
		`SELECT COUNT(*) FROM security_events WHERE `+wsql,
		`SELECT id, client_ip, client_port, phase, signature, action, detail_json, created_at
		FROM security_events WHERE `+wsql+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		args, f.Limit, f.Offset,
		func(rows *sql.Rows) error {
			var e SecurityEvent
			var created string
			var port, detail sql.NullString
			if err := rows.Scan(&e.ID, &e.ClientIP, &port, &e.Phase, &e.Signature, &e.Action, &detail, &created); err != nil {
				return err
			}
			e.ClientPort = port.String
			e.DetailJSON = detail.String
			e.CreatedAt = timeutil.ParseUTC(created)
			out = append(out, e)
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// CountSecurityEventsSince 统计某 IP 自 since 以来 action=rejected 的事件数（可排除部分 signature）。
//
// 仅计 rejected，避免解封后残留 auto_banned/banned_hit 等立刻再触发自动封。
func (s *Store) CountSecurityEventsSince(ip string, since time.Time, excludeSignatures []string) (int, error) {
	where := []string{"client_ip=?", "created_at>=?", "action=?"}
	args := []any{ip, timeutil.FormatUTC(since), "rejected"}
	for _, sig := range excludeSignatures {
		sig = strings.TrimSpace(sig)
		if sig == "" {
			continue
		}
		where = append(where, "signature<>?")
		args = append(args, sig)
	}
	q := `SELECT COUNT(*) FROM security_events WHERE ` + strings.Join(where, " AND ")
	var n int
	err := s.db.QueryRow(q, args...).Scan(&n)
	return n, err
}

// PruneSecurityEvents 删除早于 cutoff 的安全事件。
func (s *Store) PruneSecurityEvents(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM security_events WHERE created_at < ?`, timeutil.FormatUTC(cutoff))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetActiveIPBlock 返回仍生效的封禁（enabled=1 且未过期）；无则 (nil, nil)。
func (s *Store) GetActiveIPBlock(ip string) (*IPBlock, error) {
	row := s.db.QueryRow(
		`SELECT id, ip, reason, source, signature, hits, expires_at, enabled, created_at, updated_at, last_hit_at
		FROM ip_blocks WHERE ip=? AND enabled=1`, ip)
	b, err := scanIPBlock(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if b.ExpiresAt != nil && time.Now().After(*b.ExpiresAt) {
		return nil, nil
	}
	return b, nil
}

// UpsertIPBlock 插入或更新封禁（同 IP 合并为一行并重新启用）。
func (s *Store) UpsertIPBlock(b IPBlock) error {
	var exp any
	if b.ExpiresAt != nil {
		exp = timeutil.FormatUTC(*b.ExpiresAt)
	}
	enabled := 0
	if b.Enabled {
		enabled = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO ip_blocks(ip, reason, source, signature, hits, expires_at, enabled, created_at, updated_at, last_hit_at)
		VALUES(?,?,?,?,?,?,?,datetime('now'),datetime('now'),NULL)
		ON CONFLICT(ip) DO UPDATE SET
			reason=excluded.reason,
			source=excluded.source,
			signature=excluded.signature,
			expires_at=excluded.expires_at,
			enabled=excluded.enabled,
			updated_at=datetime('now')`,
		b.IP, b.Reason, b.Source, b.Signature, b.Hits, exp, enabled,
	)
	return err
}

// IncrementIPBlockHit 封禁命中：hits+1，更新 last_hit_at。
func (s *Store) IncrementIPBlockHit(ip string) error {
	_, err := s.db.Exec(
		`UPDATE ip_blocks SET hits=hits+1, last_hit_at=datetime('now'), updated_at=datetime('now') WHERE ip=? AND enabled=1`,
		ip,
	)
	return err
}

// DisableIPBlock 解封（enabled=0，保留行）。
func (s *Store) DisableIPBlock(ip string) error {
	_, err := s.db.Exec(
		`UPDATE ip_blocks SET enabled=0, updated_at=datetime('now') WHERE ip=?`,
		ip,
	)
	return err
}

// IPBlockFilter 封禁列表筛选。
type IPBlockFilter struct {
	OnlyEnabled bool
	Limit       int
	Offset      int
}

// ListIPBlocks 分页列出封禁记录。
func (s *Store) ListIPBlocks(f IPBlockFilter) ([]IPBlock, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	where := []string{"1=1"}
	args := []any{}
	if f.OnlyEnabled {
		where = append(where, "enabled=1")
	}
	wsql := strings.Join(where, " AND ")
	var out []IPBlock
	total, err := s.queryPageTotal(
		`SELECT COUNT(*) FROM ip_blocks WHERE `+wsql,
		`SELECT id, ip, reason, source, signature, hits, expires_at, enabled, created_at, updated_at, last_hit_at
		FROM ip_blocks WHERE `+wsql+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		args, f.Limit, f.Offset,
		func(rows *sql.Rows) error {
			b, err := scanIPBlockRows(rows)
			if err != nil {
				return err
			}
			out = append(out, *b)
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// PruneExpiredIPBlocks 将已过期的 enabled 封禁置为 enabled=0。
func (s *Store) PruneExpiredIPBlocks(now time.Time) (int64, error) {
	res, err := s.db.Exec(
		`UPDATE ip_blocks SET enabled=0, updated_at=datetime('now')
		 WHERE enabled=1 AND expires_at IS NOT NULL AND expires_at < ?`,
		timeutil.FormatUTC(now),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BanExemptFilter 封禁豁免列表筛选。
type BanExemptFilter struct {
	OnlyEnabled bool
	Limit       int
	Offset      int
}

// UpsertBanExempt 插入或更新封禁豁免（同 IP 合并为一行并重新启用）。
func (s *Store) UpsertBanExempt(ip, note, source string) error {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return fmt.Errorf("ip 不能为空")
	}
	if strings.TrimSpace(source) == "" {
		source = "manual"
	}
	_, err := s.db.Exec(`
		INSERT INTO ip_ban_exempt(ip, note, source, enabled, created_at)
		VALUES(?,?,?,1,datetime('now'))
		ON CONFLICT(ip) DO UPDATE SET
			note=CASE WHEN excluded.note<>'' THEN excluded.note ELSE ip_ban_exempt.note END,
			source=excluded.source,
			enabled=1`,
		ip, strings.TrimSpace(note), source,
	)
	return err
}

// DisableBanExempt 移除封禁豁免（enabled=0，保留行）。
func (s *Store) DisableBanExempt(ip string) error {
	_, err := s.db.Exec(
		`UPDATE ip_ban_exempt SET enabled=0 WHERE ip=?`,
		strings.TrimSpace(ip),
	)
	return err
}

// ListBanExempt 分页列出封禁豁免。
func (s *Store) ListBanExempt(f BanExemptFilter) ([]IPBanExempt, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	where := []string{"1=1"}
	args := []any{}
	if f.OnlyEnabled {
		where = append(where, "enabled=1")
	}
	wsql := strings.Join(where, " AND ")
	var out []IPBanExempt
	total, err := s.queryPageTotal(
		`SELECT COUNT(*) FROM ip_ban_exempt WHERE `+wsql,
		`SELECT id, ip, note, source, enabled, created_at
		FROM ip_ban_exempt WHERE `+wsql+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		args, f.Limit, f.Offset,
		func(rows *sql.Rows) error {
			var e IPBanExempt
			var created string
			var enabled int
			if err := rows.Scan(&e.ID, &e.IP, &e.Note, &e.Source, &enabled, &created); err != nil {
				return err
			}
			e.Enabled = enabled != 0
			e.CreatedAt = timeutil.ParseUTC(created)
			out = append(out, e)
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// ListEnabledBanExemptIPs 返回所有生效豁免 IP/CIDR 字符串（Guard 热加载用）。
func (s *Store) ListEnabledBanExemptIPs() ([]string, error) {
	rows, err := s.db.Query(`SELECT ip FROM ip_ban_exempt WHERE enabled=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var ip string
		if err := rows.Scan(&ip); err != nil {
			return nil, err
		}
		out = append(out, ip)
	}
	return out, rows.Err()
}

// ipBlockScanner 统一 *sql.Row / *sql.Rows 的 Scan 接口，避免两套填充逻辑漂移。
type ipBlockScanner interface {
	Scan(dest ...any) error
}

// scanIPBlock 从单行查询填充 IPBlock。
func scanIPBlock(row *sql.Row) (*IPBlock, error) {
	return fillIPBlock(row)
}

// scanIPBlockRows 从分页 rows 填充一条 IPBlock。
func scanIPBlockRows(rows *sql.Rows) (*IPBlock, error) {
	return fillIPBlock(rows)
}

// fillIPBlock 将 ip_blocks 表一行扫入结构体（列顺序须与 SELECT 一致）。
//
// 列顺序：id, ip, reason, source, signature, hits, expires_at, enabled, created_at, updated_at, last_hit_at。
// 为何合一：原先 scanIPBlock / scanIPBlockRows 复制粘贴，字段增减易漏改一侧。
func fillIPBlock(sc ipBlockScanner) (*IPBlock, error) {
	var b IPBlock
	var exp, created, updated, lastHit sql.NullString
	var sig sql.NullString
	var enabled int
	err := sc.Scan(&b.ID, &b.IP, &b.Reason, &b.Source, &sig, &b.Hits, &exp, &enabled, &created, &updated, &lastHit)
	if err != nil {
		return nil, err
	}
	b.Signature = sig.String
	b.Enabled = enabled != 0
	b.CreatedAt = timeutil.ParseUTC(created.String)
	b.UpdatedAt = timeutil.ParseUTC(updated.String)
	if exp.Valid && exp.String != "" {
		t := timeutil.ParseUTC(exp.String)
		b.ExpiresAt = &t
	}
	if lastHit.Valid && lastHit.String != "" {
		t := timeutil.ParseUTC(lastHit.String)
		b.LastHitAt = &t
	}
	return &b, nil
}
