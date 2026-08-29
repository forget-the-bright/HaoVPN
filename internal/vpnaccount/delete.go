package vpnaccount

import (
	"errors"

	"haovpn/internal/persist"
)

// ErrLastAdmin 试图删除或禁用最后一个启用的管理员时返回（防 Web 管理面锁死）。
var ErrLastAdmin = errors.New("不能删除或禁用最后一个启用的管理员账号")

// guardLastAdmin 若目标为启用中的管理员且当前仅一名启用管理员，则返回 ErrLastAdmin。
func (s *Service) guardLastAdmin(u *persist.User) error {
	if u == nil || !u.IsAdmin || !u.Enabled {
		return nil
	}
	n, err := s.Store.CountEnabledAdmins()
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// DeleteAccount 删除 VPN/Web 账号：踢线、按 ip_mode 释放 IP 占用、删 users 行。
//
// 参数：userID — users.id。
// 返回：GetUserByID 或 DeleteUser 的错误；用户不存在时返回 sql.ErrNoRows；
// 若为最后一个启用管理员则 ErrLastAdmin。
// 副作用：OnKickUser、Pool/Store 释放、OnUnregisterIP；须由 serverapp 注入 OnKickUser。
func (s *Service) DeleteAccount(userID int64) error {
	u, err := s.Store.GetUserByID(userID)
	if err != nil {
		return err
	}
	if err := s.guardLastAdmin(u); err != nil {
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
