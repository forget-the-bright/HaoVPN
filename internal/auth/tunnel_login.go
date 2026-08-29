package auth

import (
	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// VerifyTunnelLogin 隧道握手阶段的账号密码校验，不创建 Web 会话。
//
// 参数：username/password/clientIP 同 Login；须已开通 VPN 密钥且非「须改密」状态。
// 返回：通过时 *persist.User；须改密、无密钥、锁定或凭据错误时返回 errors.go 哨兵。
// 副作用：失败时可能累加 tunnelLockouts；不写 sessions；与 Web 锁定表隔离。
func (s *Service) VerifyTunnelLogin(username, password, clientIP string) (*persist.User, error) {
	u, err := s.verifyCredentials(lockoutTunnel, username, password, clientIP)
	if err != nil {
		return nil, err
	}
	if u.MustChangePassword {
		logger.Warn("隧道登录拒绝: 须先修改密码 user=%s ip=%s", username, clientIP)
		return nil, ErrMustChangePassword
	}
	if u.PublicKey == "" || u.PrivateKeyEnc == "" {
		return nil, ErrNoVPN
	}
	return u, nil
}
