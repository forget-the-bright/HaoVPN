package auth

import (
	"errors"

	"haovpn/internal/logger"
	"haovpn/internal/persist"
)

// VerifyTunnelLogin 隧道握手阶段的账号密码校验，不创建 Web 会话。
//
// 参数：username/password/clientIP 同 Login；须已开通 VPN 密钥且非「须改密」状态。
// 返回：通过时 *persist.User；须改密、无密钥、锁定或凭据错误时 err 非 nil。
// 副作用：失败时可能累加 lockouts；不写 sessions。
func (s *Service) VerifyTunnelLogin(username, password, clientIP string) (*persist.User, error) {
	u, err := s.verifyCredentials(username, password, clientIP)
	if err != nil {
		return nil, err
	}
	if u.MustChangePassword {
		logger.Warn("隧道登录拒绝: 须先修改密码 user=%s ip=%s", username, clientIP)
		return nil, errors.New("须先修改密码后再连接 VPN（请用 Web 管理端或联系管理员）")
	}
	if u.PublicKey == "" || u.PrivateKeyEnc == "" {
		return nil, errors.New("账号未开通 VPN（无密钥）")
	}
	return u, nil
}
