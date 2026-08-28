package vpnaccount

import "haovpn/internal/persist"

// DeleteAccount 删除 VPN/Web 账号：踢线、按 ip_mode 释放 IP 占用、删 users 行。
//
// 参数：userID — users.id。
// 返回：GetUserByID 或 DeleteUser 的错误；用户不存在时返回 sql.ErrNoRows。
// 副作用：OnKickUser、Pool/Store 释放、OnUnregisterIP；须由 serverapp 注入 OnKickUser。
func (s *Service) DeleteAccount(userID int64) error {
	u, err := s.Store.GetUserByID(userID)
	if err != nil {
		return err
	}
	if s.OnKickUser != nil {
		s.OnKickUser(userID)
	}
	if u != nil && u.HasVPN() && u.VPNIP != "" {
		switch u.IPMode {
		case persist.IPModeFixed, "":
			s.ReleaseFixedVPNIP(userID, u.VPNIP)
		default:
			s.releaseDynamicIP(u.VPNIP)
		}
	}
	return s.Store.DeleteUser(userID)
}
