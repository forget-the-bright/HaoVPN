// Package vpnaccount VPN 账号 IP 分配与策略解析（握手/断线共用，避免 api↔tunnel 循环依赖）。
package vpnaccount

import (
	"fmt"
	"time"

	"haovpn/internal/config"
	"haovpn/internal/ippool"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/safeutil"
)

// Service 握手与 IP 模式逻辑。
type Service struct {
	Store  *persist.Store
	Pool   *ippool.Pool
	Cfg    *config.ServerConfig
	OnRegisterIP func(vpnIP string, userID int64)
	OnUnregisterIP func(vpnIP string)
}

// DefaultAllowedIPs 服务端默认分流模板。
func (s *Service) DefaultAllowedIPs() []string {
	allowed := append([]string{}, s.Cfg.NAT.AllowedLANCIDRs...)
	if s.Cfg.Security.EnforceSplitTunnel {
		allowed = append(allowed, s.Cfg.VPN.Subnet)
	}
	return allowed
}

// ResolveAllowedIPs 解析账号有效 AllowedIPs。
func (s *Service) ResolveAllowedIPs(u *persist.User) []string {
	return persist.ResolveAllowedIPs(u.AllowedIPs, s.DefaultAllowedIPs())
}

// EnsureVPNIP 握手前确保账号有可用 vpn_ip（按 ip_mode）。
func (s *Service) EnsureVPNIP(u *persist.User) (string, error) {
	switch u.IPMode {
	case persist.IPModeFixed, "":
		if u.VPNIP == "" {
			return "", fmt.Errorf("固定 IP 模式未分配 vpn_ip")
		}
		return u.VPNIP, nil
	case persist.IPModeDynamicLease:
		if ip, err := s.Store.GetLeasedIPForUser(u.ID); err == nil && ip != "" {
			if err := s.Pool.AllocateSpecific(ip, u.ID); err == nil {
				_ = s.Store.RecordIPAllocation(ip, u.ID)
				if s.OnRegisterIP != nil {
					s.OnRegisterIP(ip, u.ID)
				}
				return ip, nil
			}
		}
		fallthrough
	case persist.IPModeDynamicSession:
		ip, err := s.Pool.Allocate(u.ID)
		if err != nil {
			return "", err
		}
		if err := s.Store.UpdateUserVPNIP(u.ID, ip); err != nil {
			s.Pool.Release(ip)
			return "", err
		}
		_ = s.Store.RecordIPAllocation(ip, u.ID)
		if s.OnRegisterIP != nil {
			s.OnRegisterIP(ip, u.ID)
		}
		u.VPNIP = ip
		return ip, nil
	default:
		return "", fmt.Errorf("未知 ip_mode: %s", u.IPMode)
	}
}

// ReleaseOnDisconnect 断线按 ip_mode 回收 IP。
func (s *Service) ReleaseOnDisconnect(userID int64, vpnIP, ipMode string) {
	if vpnIP == "" {
		return
	}
	switch ipMode {
	case persist.IPModeDynamicSession:
		s.Pool.Release(vpnIP)
		_ = s.Store.ReleaseIPAllocation(vpnIP)
		if s.OnUnregisterIP != nil {
			s.OnUnregisterIP(vpnIP)
		}
	case persist.IPModeDynamicLease:
		u, _ := s.Store.GetUserByID(userID)
		leaseSec := 86400
		if u != nil && u.IPLeaseSec > 0 {
			leaseSec = u.IPLeaseSec
		}
		until := time.Now().Add(time.Duration(leaseSec) * time.Second)
		_ = s.Store.SetIPLeaseUntil(vpnIP, until)
		// 租约期内保持池内占用，避免其他用户分配到同一 IP。
		logger.Info("租约 IP 保留 user_id=%d ip=%s until=%s", userID, vpnIP, until.Format("2006-01-02 15:04:05"))
	}
}

// StartLeaseCleaner 定期清理过期租约 IP。
func (s *Service) StartLeaseCleaner(stop <-chan struct{}) {
	safeutil.GoSafe("ip-lease-cleaner", func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				ips, err := s.Store.ListExpiredLeasedIPs()
				if err != nil {
					logger.Warn("租约 IP 清理失败: %v", err)
					continue
				}
				n, err := s.Store.ExpireLeasedIPs()
				if err != nil {
					logger.Warn("租约 IP 清理失败: %v", err)
				} else if n > 0 {
					logger.Info("已清理 %d 个过期租约 IP", n)
					for _, ip := range ips {
						s.Pool.Release(ip)
						if s.OnUnregisterIP != nil {
							s.OnUnregisterIP(ip)
						}
					}
				}
			}
		}
	})
}
