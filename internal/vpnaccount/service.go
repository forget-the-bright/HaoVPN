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

// Service 服务端 VPN 账号握手、IP 分配与租约清理逻辑（tunnel 与 Web API 共用）。
//
// 字段：
//   Store — SQLite 持久化；读写 users、ip_allocations 等。
//   Pool — 虚拟网段 IP 池；Allocate/Release 与 Store 同步。
//   Cfg — 服务端 YAML（子网、NAT、分流策略）；DefaultAllowedIPs 来源。
//   OnRegisterIP — 分配 IP 后回调（如注册 TUN 路由/NAT）；可为 nil。
//   OnUnregisterIP — 释放 IP 后回调；可为 nil。
//   OnKickUser — 删除/禁用时踢线；可为 nil。
//
// 线程安全：无内部锁；调用方（serverapp/sessionmgr）应在单 goroutine 或自行串行化同一账号操作。
type Service struct {
	Store  *persist.Store
	Pool   *ippool.Pool
	Cfg    *config.ServerConfig
	OnRegisterIP func(vpnIP string, userID int64)
	OnUnregisterIP func(vpnIP string)
	OnKickUser func(userID int64)
}

// DefaultAllowedIPs 返回服务端默认分流 CIDR 模板（NAT 工控网段 + 可选 VPN 子网）。
//
// 返回：新切片副本；EnforceSplitTunnel 为 true 时追加 Cfg.VPN.Subnet。
// 副作用：无；只读配置。
func (s *Service) DefaultAllowedIPs() []string {
	allowed := append([]string{}, s.Cfg.NAT.AllowedLANCIDRs...)
	if s.Cfg.Security.EnforceSplitTunnel {
		allowed = append(allowed, s.Cfg.VPN.Subnet)
	}
	return allowed
}

// ResolveAllowedIPs 解析账号在握手下发的有效 AllowedIPs 列表。
//
// 参数：u — 账号；其 AllowedIPs 为空时使用 DefaultAllowedIPs。
// 返回：合并后的 CIDR 切片；禁止全隧道规则在 Web 创建时已校验。
func (s *Service) ResolveAllowedIPs(u *persist.User) []string {
	return persist.ResolveAllowedIPs(u.AllowedIPs, s.DefaultAllowedIPs())
}

// EnsureVPNIP 在隧道握手前按 ip_mode 确保账号拥有可用 vpn_ip。
//
// 参数：u — 已认证用户；须非 nil 且 Enabled。
// 返回：分配或复用的 IPv4 字符串；err 为固定模式未配 IP、池耗尽或未知 ip_mode。
// 副作用：可能写 Store.UpdateUserVPNIP、RecordIPAllocation、Pool.Allocate；触发 OnRegisterIP。
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

// ReleaseOnDisconnect 客户端断线时按 ip_mode 回收或保留 IP 占用。
//
// 参数：userID — 账号 ID；vpnIP — 本次会话使用的 IP；ipMode — fixed 时无操作。
// 副作用：dynamic_session 释放池与 allocation；dynamic_lease 设置租约到期时间并保留占用。
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

// StartLeaseCleaner 启动后台 goroutine，定期清理已过期的 dynamic_lease IP。
//
// 参数：stop — 关闭此 channel 时停止清理循环（通常与服务 shutdown 绑定）。
// 副作用：每 60s 扫描过期租约，释放 Pool 并调用 OnUnregisterIP；通过 safeutil.GoSafe 启动。
// 并发：与 EnsureVPNIP/ReleaseOnDisconnect 并行；依赖 Store 与 Pool 自身一致性。
func (s *Service) StartLeaseCleaner(stop <-chan struct{}) {
	safeutil.GoSafe("ip-lease-cleaner", func() {
		safeutil.RunTickerStop(stop, 60*time.Second, func() {
			ips, err := s.Store.ListExpiredLeasedIPs()
			if err != nil {
				logger.Warn("租约 IP 清理失败: %v", err)
				return
			}
			n, err := s.Store.ExpireLeasedIPs()
			if err != nil {
				logger.Warn("租约 IP 清理失败: %v", err)
				return
			}
			if n > 0 {
				logger.Info("已清理 %d 个过期租约 IP", n)
				for _, ip := range ips {
					s.Pool.Release(ip)
					if s.OnUnregisterIP != nil {
						s.OnUnregisterIP(ip)
					}
				}
			}
		})
	})
}
