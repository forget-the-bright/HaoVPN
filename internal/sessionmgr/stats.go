package sessionmgr

import (
	"haovpn/internal/persist"
)

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
