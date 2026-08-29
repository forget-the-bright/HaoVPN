package persist

import (
	"database/sql"
	"fmt"
	"strings"

	"haovpn/internal/paginate"
	"haovpn/internal/readmodel"
)

// userListCols Web 列表所需列（不含 password_hash / private_key_enc）。
const userListCols = `id, username, enabled, public_key, vpn_ip, allowed_ips, ip_mode, ip_lease_sec, policy_ver`

// ListUsersPage 分页列出账号（轻量列 + 可选筛选）。
//
// 参数：f — Limit/Offset 经 paginate 裁剪；Q 模糊匹配 username；UseEnabled+Enabled 筛选启用状态。
// 返回：items 不含 password_hash/private_key_enc；total 为筛选后总行数；err 为 COUNT 或 SELECT 失败。
// 副作用：只读 users 表。
// 并发：可与其他 Store 读操作并行；SQLite 层串行执行。
func (s *Store) ListUsersPage(f readmodel.UserListFilter) ([]readmodel.UserListItem, int, error) {
	f.Limit = paginate.ClampLimit(f.Limit, 50, 500)
	f.Offset = paginate.ClampOffset(f.Offset)

	where := []string{"1=1"}
	args := []any{}
	if q := strings.TrimSpace(f.Q); q != "" {
		where = append(where, "INSTR(LOWER(username), LOWER(?)) > 0")
		args = append(args, q)
	}
	if f.UseEnabled {
		if f.Enabled == 1 {
			where = append(where, "enabled=1")
		} else {
			where = append(where, "enabled=0")
		}
	}
	wsql := strings.Join(where, " AND ")
	var out []readmodel.UserListItem
	total, err := s.queryPageTotal(
		`SELECT COUNT(*) FROM users WHERE `+wsql,
		`SELECT `+userListCols+` FROM users WHERE `+wsql+` ORDER BY id LIMIT ? OFFSET ?`,
		args, f.Limit, f.Offset,
		func(rows *sql.Rows) error {
			item, err := scanUserListItem(rows)
			if err != nil {
				return err
			}
			out = append(out, item)
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func scanUserListItem(row scannable) (readmodel.UserListItem, error) {
	var item readmodel.UserListItem
	var enabled int
	var pubKey, vpnIP, ipsJSON, ipMode sql.NullString
	var ipLease, policyVer int
	if err := row.Scan(&item.ID, &item.Username, &enabled, &pubKey, &vpnIP, &ipsJSON, &ipMode, &ipLease, &policyVer); err != nil {
		return item, err
	}
	item.Enabled = enabled == 1
	item.HasVPN = pubKey.Valid && pubKey.String != ""
	item.VPNIP = vpnIP.String
	item.IPMode = ipMode.String
	item.PolicyVer = policyVer
	unmarshalAllowedIPs(ipsJSON, &item.AllowedIPs)
	return item, nil
}

// UserDirectoryEntry 控制台下拉/路由展示用轻量账号（无私钥/密码）。
type UserDirectoryEntry struct {
	ID       int64
	Username string
	VPNIP    string
	HasVPN   bool
	IsAdmin  bool
}

// ListUserDirectory 列出全部账号的 id/用户名/vpn_ip（托管路由页用，不读私钥）。
func (s *Store) ListUserDirectory() ([]UserDirectoryEntry, error) {
	rows, err := s.db.Query(
		`SELECT id, username, COALESCE(vpn_ip,''), CASE WHEN public_key IS NOT NULL AND public_key != '' THEN 1 ELSE 0 END, is_admin FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UserDirectoryEntry
	for rows.Next() {
		var e UserDirectoryEntry
		var hasVPN, isAdmin int
		if err := rows.Scan(&e.ID, &e.Username, &e.VPNIP, &hasVPN, &isAdmin); err != nil {
			return nil, err
		}
		e.HasVPN = hasVPN == 1
		e.IsAdmin = isAdmin == 1
		out = append(out, e)
	}
	return out, rows.Err()
}

// UsernameByID 按 ID 取用户名（事件列表展示用）。
//
// 参数：id — users.id；不存在时返回 "#<id>" 占位字符串。
// 返回：username 或占位；永不返回 error（查询失败时降级为占位）。
// 副作用：只读 users 表一行。
// 并发：可并行调用；只读无锁。
func (s *Store) UsernameByID(id int64) string {
	var name string
	if err := s.db.QueryRow(`SELECT username FROM users WHERE id=?`, id).Scan(&name); err != nil {
		return fmt.Sprintf("#%d", id)
	}
	return name
}
