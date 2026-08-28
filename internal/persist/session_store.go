package persist

import (
	"database/sql"
	"time"

	"haovpn/internal/timeutil"
)

// InsertConnectionEvent 写入隧道连接/断线/认证失败等事件。
//
// 参数：userID — 关联 users.id；eventType — 如 connect、disconnect；detail 为 JSON 或空。
// 返回：err 为 INSERT 失败。
// 副作用：写 connection_events；供仪表盘与 ListRecentConnectionEvents 展示。
func (s *Store) InsertConnectionEvent(userID int64, eventType, remoteAddr, detail string) error {
	_, err := s.db.Exec(`INSERT INTO connection_events(user_id, event_type, remote_addr, detail_json) VALUES(?,?,?,?)`,
		userID, eventType, remoteAddr, detail)
	return err
}

// ListRecentConnectionEvents 返回最近连接事件。
func (s *Store) ListRecentConnectionEvents(limit int) ([]ConnectionEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, user_id, event_type, remote_addr, detail_json, created_at
		FROM connection_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConnectionEventRows(rows)
}

// UpsertSessionStat 插入或更新在线会话统计（每 user_id 一行）。
//
// 参数：st — UserID 必填；ConnectedAt/LastHeartbeat 可为 nil 表示未知。
// 返回：err 为 UPSERT 失败。
// 副作用：写 session_stats；sessionmgr 心跳与断线时调用。
func (s *Store) UpsertSessionStat(st SessionStat) error {
	_, err := s.db.Exec(`INSERT INTO session_stats(user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr)
		VALUES(?,?,?,?,?,?,?)
		ON CONFLICT(user_id) DO UPDATE SET
		connected_at=excluded.connected_at,
		last_heartbeat=excluded.last_heartbeat,
		rx_bytes=excluded.rx_bytes,
		tx_bytes=excluded.tx_bytes,
		reconnect_count=excluded.reconnect_count,
		remote_addr=excluded.remote_addr`,
		st.UserID, timePtrStr(st.ConnectedAt), timePtrStr(st.LastHeartbeat),
		st.RxBytes, st.TxBytes, st.ReconnectCount, st.RemoteAddr)
	return err
}

// GetSessionStat 读取指定用户的会话统计行。
//
// 参数：userID — users.id。
// 返回：*SessionStat；无记录时 err 为 sql.ErrNoRows。
// 副作用：只读；管理 API 展示在线状态与流量时使用。
func (s *Store) GetSessionStat(userID int64) (*SessionStat, error) {
	row := s.db.QueryRow(`SELECT user_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count, remote_addr FROM session_stats WHERE user_id=?`, userID)
	var st SessionStat
	var connAt, hb, remote sql.NullString
	if err := row.Scan(&st.UserID, &connAt, &hb, &st.RxBytes, &st.TxBytes, &st.ReconnectCount, &remote); err != nil {
		return nil, err
	}
	st.ConnectedAt = timeutil.ParseUTCPtr(connAt)
	st.LastHeartbeat = timeutil.ParseUTCPtr(hb)
	if remote.Valid {
		st.RemoteAddr = remote.String
	}
	return &st, nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanUser(row scannable) (*User, error) {
	var u User
	var enabled, mustChange, isAdmin int
	var pubKey, privEnc, vpnIP, ipsJSON, ipMode sql.NullString
	var ipLease, policyVer int
	var created, updated string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &enabled, &mustChange, &isAdmin,
		&pubKey, &privEnc, &vpnIP, &ipsJSON, &ipMode, &ipLease, &policyVer, &created, &updated); err != nil {
		return nil, err
	}
	u.Enabled = enabled == 1
	u.MustChangePassword = mustChange == 1
	u.IsAdmin = isAdmin == 1
	u.PublicKey = pubKey.String
	u.PrivateKeyEnc = privEnc.String
	u.VPNIP = vpnIP.String
	unmarshalAllowedIPs(ipsJSON, &u.AllowedIPs)
	u.IPMode = ipMode.String
	if u.IPMode == "" {
		u.IPMode = IPModeFixed
	}
	u.IPLeaseSec = ipLease
	if u.IPLeaseSec <= 0 {
		u.IPLeaseSec = DefaultIPLeaseSec
	}
	u.PolicyVer = policyVer
	if u.PolicyVer <= 0 {
		u.PolicyVer = 1
	}
	u.CreatedAt = timeutil.ParseUTC(created)
	u.UpdatedAt = timeutil.ParseUTC(updated)
	return &u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func timePtrStr(t *time.Time) interface{} {
	if t == nil {
		return nil
	}
	return timeutil.FormatUTC(*t)
}

func nullStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
