package persist

import (
	"database/sql"
	"strings"

	"haovpn/internal/readmodel"
	"haovpn/internal/timeutil"
)

// ListMonitorAccountRows 一次 JOIN 拉取监控所需字段，避免 N+1。
//
// 返回：已配置公钥的 VPN 账号 + 左连接 session_stats；可选 f.NameQuery 用户名子串筛选。
// err 为 JOIN 查询或 Scan 失败。
// 副作用：只读 users 与 session_stats 表。
// 并发：可并行调用；管理监控页刷新时使用。
func (s *Store) ListMonitorAccountRows(f readmodel.MonitorAccountFilter) ([]readmodel.MonitorAccountRow, error) {
	where := []string{"u.public_key IS NOT NULL", "u.public_key != ''"}
	args := []any{}
	if q := strings.TrimSpace(f.NameQuery); q != "" {
		where = append(where, "INSTR(LOWER(u.username), LOWER(?)) > 0")
		args = append(args, q)
	}
	wsql := strings.Join(where, " AND ")
	rows, err := s.db.Query(`SELECT u.id, u.username, u.vpn_ip, u.ip_mode, u.policy_ver, u.allowed_ips,
		ss.connected_at, ss.last_heartbeat, ss.rx_bytes, ss.tx_bytes, ss.reconnect_count, ss.remote_addr
		FROM users u
		LEFT JOIN session_stats ss ON ss.user_id = u.id
		WHERE `+wsql+` ORDER BY u.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []readmodel.MonitorAccountRow
	for rows.Next() {
		var r readmodel.MonitorAccountRow
		var vpnIP, ipMode, ipsJSON sql.NullString
		var policyVer int
		var connAt, hb, remote sql.NullString
		var rx, tx sql.NullInt64
		var recon sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Username, &vpnIP, &ipMode, &policyVer, &ipsJSON,
			&connAt, &hb, &rx, &tx, &recon, &remote); err != nil {
			return nil, err
		}
		r.VPNIP = vpnIP.String
		r.IPMode = ipMode.String
		r.PolicyVer = policyVer
		unmarshalAllowedIPs(ipsJSON, &r.AllowedIPs)
		r.ConnectedAt = timeutil.ParseUTCPtr(connAt)
		r.LastHeartbeat = timeutil.ParseUTCPtr(hb)
		if rx.Valid {
			r.RxBytes = rx.Int64
		}
		if tx.Valid {
			r.TxBytes = tx.Int64
		}
		if recon.Valid {
			r.ReconnectCount = int(recon.Int64)
		}
		r.RemoteAddr = remote.String
		out = append(out, r)
	}
	return out, rows.Err()
}
