package sessionmgr

import (
	"time"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/safeutil"
)

// stats.go：在线会话查询与 session_stats 节流刷新。

// OnlineCount 返回当前内存中在线 VPN 账号数。
func (m *Manager) OnlineCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GetSession 按 userID 查询在线会话。
//
// 返回：会话指针与是否存在；指针在会话存活期间有效，调用方不应长期持有。
func (m *Manager) GetSession(userID int64) (*AccountSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ps, ok := m.sessions[userID]
	return ps, ok
}

// ListOnline 返回所有在线账号 userID 切片（顺序不定，仅供遍历）。
func (m *Manager) ListOnline() []int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ids := make([]int64, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// ValidateVPNAccess 校验账号是否允许建立 VPN 隧道连接。
//
// 返回：无 VPN 身份或账号已禁用时 *AccessError。
func (m *Manager) ValidateVPNAccess(user *persist.User) error {
	if user == nil || !user.HasVPN() {
		return ErrNoVPNIdentity
	}
	if !user.Enabled {
		return ErrUserDisabled
	}
	return nil
}

var (
	ErrNoVPNIdentity = &AccessError{Msg: "账号未配置 VPN 身份"}
	ErrUserDisabled  = &AccessError{Msg: "账号已禁用"}
)

// AccessError 表示隧道接入被拒绝（无 VPN 身份或账号已禁用）。
type AccessError struct{ Msg string }

func (e *AccessError) Error() string { return e.Msg }

// noteInboundRx 累加入站字节并节流刷新 session_stats（≥5s 一次，经 safeutil.AllowEvery）。
func (m *Manager) noteInboundRx(ps *AccountSession, userID int64, n int) {
	if ps == nil || n <= 0 {
		return
	}
	ps.RxBytes.Add(int64(n))
	now := time.Now()
	// 与 WARN 限频同一 CAS 语义；此处驱动 DB 写而非日志。
	if !safeutil.AllowEvery(&ps.lastStatFlush, 5*time.Second) || m.store == nil {
		return
	}
	m.flushSessionStat(userID, &ps.ConnectedAt, &now, ps.RxBytes.Load(), ps.TxBytes.Load(), ps.RemoteAddr)
}

// flushSessionStat 写入/更新 session_stats，保留既有 reconnect_count。
//
// route 节流刷新与 kick 断线共用，避免两处字段漂移。
func (m *Manager) flushSessionStat(userID int64, connectedAt, lastHB *time.Time, rx, tx int64, remoteAddr string) {
	if m.store == nil {
		return
	}
	rc := 0
	if st, err := m.store.GetSessionStat(userID); err == nil && st != nil {
		rc = st.ReconnectCount
	}
	if err := m.store.UpsertSessionStat(persist.SessionStat{
		UserID:         userID,
		ConnectedAt:    connectedAt,
		LastHeartbeat:  lastHB,
		RxBytes:        rx,
		TxBytes:        tx,
		ReconnectCount: rc,
		RemoteAddr:     remoteAddr,
	}); err != nil {
		logger.Warn("更新 session_stats 失败 user_id=%d: %v", userID, err)
	}
}
