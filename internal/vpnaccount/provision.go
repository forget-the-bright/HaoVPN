package vpnaccount

import (
	"fmt"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/security"
)

// ValidateAllowedIPs 校验 Web 创建/更新账号时的 allowed_ips 列表。
//
// 参数：cidrs — 用户提交的分流网段；禁止 0.0.0.0/0 全隧道。
// 返回：err 为 netutil.ValidateNoFullTunnel 的错误。
// 说明：本函数是 vpnaccount 领域别名，委托 netutil；api/Web 开户与 PATCH 须经本入口，
// 禁止在 api 层直接调用 netutil.ValidateNoFullTunnel，以免账号策略校验散落、语义漂移。
func ValidateAllowedIPs(cidrs []string) error {
	return netutil.ValidateNoFullTunnel(cidrs)
}

// ValidateManualIP 校验手动指定的 fixed 模式 VPN IP 是否合法且可分配。
//
// 参数：ip — 请求的 IPv4；allowUserID — 若 IP 已被占，允许属该 user_id 时通过（更新场景）。
// 返回：err 为不在子网、是网关、池未初始化或已被他人占用。
func (s *Service) ValidateManualIP(ip string, allowUserID int64) error {
	ip = strings.TrimSpace(ip)
	gw := strings.TrimSpace(s.Cfg.VPN.GatewayIP)
	if err := netutil.ValidateIPInSubnet(ip, s.Cfg.VPN.Subnet, gw); err != nil {
		return err
	}
	ipNorm, err := netutil.NormalizeIPv4(ip)
	if err != nil {
		return err
	}
	ip = ipNorm
	if s.Pool == nil {
		return fmt.Errorf("IP 池未初始化")
	}
	if s.Pool.IsAllocated(ip) {
		if allowUserID > 0 {
			idx, _ := s.Store.GetUserVPNIPIndex()
			if owner, ok := idx[ip]; ok && owner == allowUserID {
				return nil
			}
		}
		return fmt.Errorf("VPN IP %s 已被占用", ip)
	}
	return nil
}

// ProvisionInput Web/API 创建 VPN 账号时的入参。
//
// 字段：
//   Username — 登录名；须唯一。
//   PasswordHash — 已哈希密码；不由本包哈希。
//   IPMode — fixed / dynamic_session / dynamic_lease；空则 fixed。
//   IPLeaseSec — 租约秒数；dynamic_lease 用；≤0 时默认 persist.DefaultIPLeaseSec。
//   AllowedIPs — 分流 CIDR；nil 存空数组；须通过 ValidateAllowedIPs。
//   RequestedIP — fixed 模式可选指定 IP；动态模式须为空。
//   KeyEnc — 非 nil 时用 AES 密封私钥再入库。
type ProvisionInput struct {
	Username     string
	PasswordHash string
	IPMode       string
	IPLeaseSec   int
	AllowedIPs   []string
	RequestedIP  string
	KeyEnc       *security.KeyEnc
}

// ProvisionResult Web 创建 VPN 账号的返回结果。
//
// 字段：
//   UserID — 新账号 users.id。
//   VPNIP — fixed 模式下的分配 IP；动态模式为空，握手时再分配。
type ProvisionResult struct {
	UserID int64
	VPNIP  string
}

// ProvisionWebAccount 创建 Web 账号并生成隧道密钥、按 ip_mode 占用 IP。
//
// 参数：in — 用户名、密码哈希、IP 策略等；KeyEnc 可选加密私钥。
// 返回：ProvisionResult 含 user_id 与 vpn_ip；任一步失败则回滚池占用并可能 DeleteUser。
// 副作用：写 users 表、Pool.Allocate/RecordIPAllocation、OnRegisterIP；日志记录创建。
func (s *Service) ProvisionWebAccount(in ProvisionInput) (ProvisionResult, error) {
	username := strings.TrimSpace(in.Username)
	if err := auth.ValidateUsername(username); err != nil {
		return ProvisionResult{}, err
	}
	in.Username = username
	ipMode := in.IPMode
	if ipMode == "" {
		ipMode = persist.IPModeFixed
	}
	leaseSec := in.IPLeaseSec
	if leaseSec <= 0 {
		leaseSec = persist.DefaultIPLeaseSec
	}
	allowedIPs := in.AllowedIPs
	if allowedIPs == nil {
		allowedIPs = []string{}
	}
	if err := ValidateAllowedIPs(allowedIPs); err != nil {
		return ProvisionResult{}, err
	}
	requestedIP := strings.TrimSpace(in.RequestedIP)

	switch ipMode {
	case persist.IPModeFixed:
	case persist.IPModeDynamicSession, persist.IPModeDynamicLease:
		if requestedIP != "" {
			return ProvisionResult{}, fmt.Errorf("动态 IP 模式不可指定 VPN IP，请留空由握手分配")
		}
	default:
		return ProvisionResult{}, fmt.Errorf("未知 ip_mode: %s", ipMode)
	}

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		return ProvisionResult{}, err
	}
	privEnc := kp.PrivateKey
	if in.KeyEnc != nil {
		sealed, err := in.KeyEnc.SealPrivateKey(kp.PrivateKey)
		if err != nil {
			return ProvisionResult{}, err
		}
		privEnc = sealed
	}

	var vpnIP string
	if ipMode == persist.IPModeFixed {
		if requestedIP != "" {
			if err := s.ValidateManualIP(requestedIP, 0); err != nil {
				return ProvisionResult{}, err
			}
			vpnIP, err = netutil.NormalizeIPv4(requestedIP)
			if err != nil {
				return ProvisionResult{}, err
			}
		} else {
			vpnIP, err = s.Pool.Allocate(0)
			if err != nil {
				return ProvisionResult{}, err
			}
		}
	}

	u := persist.User{
		Username:      in.Username,
		PasswordHash:  in.PasswordHash,
		PublicKey:     kp.PublicKey,
		PrivateKeyEnc: privEnc,
		VPNIP:         vpnIP,
		AllowedIPs:    allowedIPs,
		IPMode:        ipMode,
		IPLeaseSec:    leaseSec,
		PolicyVer:     1,
		Enabled:       true,
	}
	id, err := s.Store.CreateVPNAccount(u)
	if err != nil {
		if vpnIP != "" {
			s.Pool.Release(vpnIP)
		}
		return ProvisionResult{}, err
	}

	if ipMode == persist.IPModeFixed && vpnIP != "" {
		s.Pool.Release(vpnIP)
		if err := s.Pool.AllocateSpecific(vpnIP, id); err != nil {
			_ = s.Store.DeleteUser(id)
			return ProvisionResult{}, err
		}
		_ = s.Store.RecordIPAllocation(vpnIP, id)
		if s.OnRegisterIP != nil {
			s.OnRegisterIP(vpnIP, id)
		}
	}

	logger.Info("VPN 账号已创建: user=%s id=%d ip_mode=%s vpn_ip=%s", in.Username, id, ipMode, vpnIP)
	return ProvisionResult{UserID: id, VPNIP: vpnIP}, nil
}

// RebindFixedVPNIP 更换 fixed 模式账号的 VPN IP（先占新 IP 再释旧 IP）。
//
// 参数：userID — 账号 ID；oldIP — 当前 IP（可为空）；newIP — 目标 IP，须通过 ValidateManualIP。
// 返回：err 为校验失败或 AllocateSpecific 失败；newIP 与 oldIP 相同时无操作。
// 副作用：更新 Pool 与 ip_allocations；触发 OnRegisterIP/OnUnregisterIP。
func (s *Service) RebindFixedVPNIP(userID int64, oldIP, newIP string) error {
	newIP = strings.TrimSpace(newIP)
	if newIP == "" {
		return fmt.Errorf("fixed 模式 VPN IP 不能为空")
	}
	if err := s.ValidateManualIP(newIP, userID); err != nil {
		return err
	}
	norm, err := netutil.NormalizeIPv4(newIP)
	if err != nil {
		return err
	}
	newIP = norm
	if oldIP == newIP {
		return nil
	}
	if err := s.Pool.AllocateSpecific(newIP, userID); err != nil {
		return err
	}
	_ = s.Store.RecordIPAllocation(newIP, userID)
	if s.OnRegisterIP != nil {
		s.OnRegisterIP(newIP, userID)
	}
	if oldIP != "" {
		s.Pool.Release(oldIP)
		_ = s.Store.ReleaseIPAllocation(oldIP)
		if s.OnUnregisterIP != nil {
			s.OnUnregisterIP(oldIP)
		}
	}
	return nil
}

// ReleaseFixedVPNIP 账号从 fixed 切走或删除时释放 IP 池占用。
//
// 参数：userID — 保留供审计扩展，当前未使用；vpnIP — 要释放的地址，空则跳过。
// 副作用：Pool.Release、ReleaseIPAllocation、OnUnregisterIP。
func (s *Service) ReleaseFixedVPNIP(userID int64, vpnIP string) {
	if vpnIP == "" {
		return
	}
	s.Pool.Release(vpnIP)
	_ = s.Store.ReleaseIPAllocation(vpnIP)
	if s.OnUnregisterIP != nil {
		s.OnUnregisterIP(vpnIP)
	}
	_ = userID
}

// OpenAccountPrivateKey 解密并返回账号隧道私钥（明文 Base64）。
//
// 参数：u — 须含 PrivateKeyEnc；keyEnc — 密文时用于 OpenPrivateKey，明文时可为 nil。
// 返回：私钥字符串；err 为无私钥或解密失败。
// 副作用：无；不修改 u。
func OpenAccountPrivateKey(u *persist.User, keyEnc *security.KeyEnc) (string, error) {
	if u.PrivateKeyEnc == "" {
		return "", fmt.Errorf("无私钥")
	}
	if keyEnc != nil && security.IsEncryptedPrivateKey(u.PrivateKeyEnc) {
		return keyEnc.OpenPrivateKey(u.PrivateKeyEnc)
	}
	return u.PrivateKeyEnc, nil
}
