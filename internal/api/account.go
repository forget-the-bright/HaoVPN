package api

import (
	"fmt"
	"net"
	"strings"

	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/security"
)

// validateAllowedIPs 禁止全隧道 0.0.0.0/0。
func validateAllowedIPs(cidrs []string) error {
	for _, c := range cidrs {
		c = strings.TrimSpace(c)
		if c == "0.0.0.0/0" || c == "::/0" {
			return fmt.Errorf("禁止 0.0.0.0/0 全隧道")
		}
		if !strings.Contains(c, "/") {
			c += "/32"
		}
		if _, _, err := net.ParseCIDR(c); err != nil {
			return fmt.Errorf("无效 CIDR %q: %w", c, err)
		}
	}
	return nil
}

// validateVPNIPInPool 校验手动指定的 VPN IP：合法、落在子网、非网关、池内空闲（或已属本账号）。
func (s *Server) validateVPNIPInPool(ip string, allowUserID int64) error {
	ip = strings.TrimSpace(ip)
	parsed := net.ParseIP(ip)
	if parsed == nil || parsed.To4() == nil {
		return fmt.Errorf("无效 VPN IP: %s", ip)
	}
	ip = parsed.To4().String()
	gw := strings.TrimSpace(s.cfg.VPN.GatewayIP)
	if gw != "" && ip == gw {
		return fmt.Errorf("不能占用网关 IP %s", gw)
	}
	_, subnet, err := net.ParseCIDR(s.cfg.VPN.Subnet)
	if err != nil {
		return fmt.Errorf("服务端 VPN 子网配置无效")
	}
	if !subnet.Contains(parsed) {
		return fmt.Errorf("VPN IP %s 不在子网 %s 内", ip, s.cfg.VPN.Subnet)
	}
	if s.ipPool == nil {
		return fmt.Errorf("IP 池未初始化")
	}
	if s.ipPool.IsAllocated(ip) {
		// 允许「本账号已占用该 IP」的幂等更新
		if allowUserID > 0 {
			idx, _ := s.store.GetUserVPNIPIndex()
			if owner, ok := idx[ip]; ok && owner == allowUserID {
				return nil
			}
		}
		return fmt.Errorf("VPN IP %s 已被占用", ip)
	}
	return nil
}

// provisionVPNAccount 为新建 Web 账号同时创建隧道身份。
// requestedIP：仅 fixed 可填；空=池自动分配；dynamic 模式禁止非空。
func (s *Server) provisionVPNAccount(username, passwordHash string, ipMode string, ipLeaseSec int, allowedIPs []string, requestedIP string) (int64, string, error) {
	if ipMode == "" {
		ipMode = persist.IPModeFixed
	}
	if ipLeaseSec <= 0 {
		ipLeaseSec = 86400
	}
	if allowedIPs == nil {
		allowedIPs = []string{}
	}
	if err := validateAllowedIPs(allowedIPs); err != nil {
		return 0, "", err
	}
	requestedIP = strings.TrimSpace(requestedIP)

	switch ipMode {
	case persist.IPModeFixed:
		// ok
	case persist.IPModeDynamicSession, persist.IPModeDynamicLease:
		if requestedIP != "" {
			return 0, "", fmt.Errorf("动态 IP 模式不可指定 VPN IP，请留空由握手分配")
		}
	default:
		return 0, "", fmt.Errorf("未知 ip_mode: %s", ipMode)
	}

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		return 0, "", err
	}
	privEnc := kp.PrivateKey
	if s.keyEnc != nil {
		sealed, err := s.keyEnc.SealPrivateKey(kp.PrivateKey)
		if err != nil {
			return 0, "", err
		}
		privEnc = sealed
	}

	var vpnIP string
	if ipMode == persist.IPModeFixed {
		if requestedIP != "" {
			if err := s.validateVPNIPInPool(requestedIP, 0); err != nil {
				return 0, "", err
			}
			vpnIP = net.ParseIP(requestedIP).To4().String()
		} else {
			vpnIP, err = s.ipPool.Allocate(0)
			if err != nil {
				return 0, "", err
			}
		}
	}

	u := persist.User{
		Username:      username,
		PasswordHash:  passwordHash,
		PublicKey:     kp.PublicKey,
		PrivateKeyEnc: privEnc,
		VPNIP:         vpnIP,
		AllowedIPs:    allowedIPs,
		IPMode:        ipMode,
		IPLeaseSec:    ipLeaseSec,
		PolicyVer:     1,
		Enabled:       true,
	}
	id, err := s.store.CreateVPNAccount(u)
	if err != nil {
		if vpnIP != "" {
			s.ipPool.Release(vpnIP)
		}
		return 0, "", err
	}

	if ipMode == persist.IPModeFixed && vpnIP != "" {
		// Allocate(0) 后改归属，或手动 IP 首次占用
		s.ipPool.Release(vpnIP)
		if err := s.ipPool.AllocateSpecific(vpnIP, id); err != nil {
			_ = s.store.DeleteUser(id)
			return 0, "", err
		}
		_ = s.store.RecordIPAllocation(vpnIP, id)
		s.sessions.RegisterVPNIP(vpnIP, id)
	}

	logger.Info("VPN 账号已创建: user=%s id=%d ip_mode=%s vpn_ip=%s", username, id, ipMode, vpnIP)
	return id, vpnIP, nil
}

// rebindFixedVPNIP 更换 fixed 账号的 VPN IP：释放旧占用、占用新 IP。
func (s *Server) rebindFixedVPNIP(userID int64, oldIP, newIP string) error {
	newIP = strings.TrimSpace(newIP)
	if newIP == "" {
		return fmt.Errorf("fixed 模式 VPN IP 不能为空")
	}
	if err := s.validateVPNIPInPool(newIP, userID); err != nil {
		return err
	}
	newIP = net.ParseIP(newIP).To4().String()
	if oldIP == newIP {
		return nil
	}
	if err := s.ipPool.AllocateSpecific(newIP, userID); err != nil {
		return err
	}
	_ = s.store.RecordIPAllocation(newIP, userID)
	s.sessions.RegisterVPNIP(newIP, userID)
	if oldIP != "" {
		s.ipPool.Release(oldIP)
		_ = s.store.ReleaseIPAllocation(oldIP)
		s.sessions.UnregisterVPNIP(oldIP)
	}
	return nil
}

// releaseFixedVPNIP 账号从 fixed 切走或删除时释放池占用。
func (s *Server) releaseFixedVPNIP(userID int64, vpnIP string) {
	if vpnIP == "" {
		return
	}
	s.ipPool.Release(vpnIP)
	_ = s.store.ReleaseIPAllocation(vpnIP)
	s.sessions.UnregisterVPNIP(vpnIP)
}

// openAccountPrivateKey 解密账号私钥。
func (s *Server) openAccountPrivateKey(u *persist.User) (string, error) {
	if u.PrivateKeyEnc == "" {
		return "", fmt.Errorf("无私钥")
	}
	if s.keyEnc != nil && security.IsEncryptedPrivateKey(u.PrivateKeyEnc) {
		return s.keyEnc.OpenPrivateKey(u.PrivateKeyEnc)
	}
	return u.PrivateKeyEnc, nil
}
