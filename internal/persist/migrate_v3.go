package persist

import (
	"database/sql"
	"fmt"

	"haovpn/internal/logger"
)

const schemaVersionV3 = "3"

// migrateV3 增加 users.is_admin 列并迁移现有管理员标记。
func (s *Store) migrateV3() error {
	if err := s.ensureSchemaMeta(); err != nil {
		return fmt.Errorf("ensure schema_meta: %w", err)
	}

	var ver string
	err := s.db.QueryRow(`SELECT value FROM schema_meta WHERE key='version'`).Scan(&ver)
	if err == nil && ver == schemaVersionV3 {
		return nil
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// 若已是 v3 结构但版本号未写入，也跳过重复 ALTER
	if s.tableHasColumn("users", "is_admin") {
		_, err = s.db.Exec(`INSERT INTO schema_meta(key, value) VALUES('version', ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, schemaVersionV3)
		return err
	}

	logger.Info("开始迁移至 v3（users.is_admin）…")
	if _, err := s.db.Exec(`ALTER TABLE users ADD COLUMN is_admin INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add is_admin: %w", err)
	}

	// 将首个用户及 username=admin 的账号标记为管理员（兼容旧库）
	if _, err := s.db.Exec(`UPDATE users SET is_admin=1 WHERE username='admin'`); err != nil {
		return fmt.Errorf("mark admin user: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE users SET is_admin=1 WHERE id=(SELECT MIN(id) FROM users) AND NOT EXISTS(SELECT 1 FROM users WHERE is_admin=1)`); err != nil {
		return fmt.Errorf("mark first user admin: %w", err)
	}

	_, err = s.db.Exec(`INSERT INTO schema_meta(key, value) VALUES('version', ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, schemaVersionV3)
	if err != nil {
		return err
	}
	logger.Info("v3 迁移完成（is_admin）")
	return nil
}

// SetUserIsAdmin 设置账号是否为 Web 管理员。
func (s *Store) SetUserIsAdmin(id int64, isAdmin bool) error {
	_, err := s.db.Exec(`UPDATE users SET is_admin=?, updated_at=datetime('now') WHERE id=?`, boolToInt(isAdmin), id)
	return err
}
