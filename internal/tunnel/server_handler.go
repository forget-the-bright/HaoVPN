package tunnel

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"haovpn/internal/auth"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
	"haovpn/internal/tun"
	"haovpn/internal/vpnaccount"
)

// ServerHandler 处理服务端每条新 TLS 连接的握手鉴权与 IP 包双向转发。
//
// 字段：
//   Store — SQLite 持久化；读取账号公钥、私钥密文、策略字段。
//   SessMgr — 在线会话管理器；握手成功后 RegisterVPN，断线 RemoveIfConn。
//   ServerKP — 服务端 WireGuard 密钥对；握手应答下发公钥，NewSession 用私钥。
//   TunDev — TUN 设备；入站解密包写入内核，nil 时丢弃并 Warn。
//   AllowedSourceIPs — 允许发起隧道的客户端源 IP/CIDR 白名单；空表示不限制。
//   AllowPlaintextPrivateKeys — true 时兼容库内未加密私钥（生产应 false）。
//   Probe — 可选探针防御；握手拒绝时记 security_events（不含密码失败细节刷爆时可关）。
//   VPN — 账号 IP 分配与 allowed_ips 解析服务。
//   MTU — 下发给客户端的 MTU；≤0 时 ResolveMTU 取平台默认。
//   GatewayIP — TUN 网关 IP；写入 HandshakePolicy 与 DNS 回落。
//   DNSServers — 推送给客户端的 DNS 列表；空时回落 GatewayIP。
//   VPNSubnet — VPN 地址池 CIDR；下发给客户端作 via 出口 SNAT 源。
//   Auth — 隧道账号密码鉴权；nil 时拒绝密码握手。
//   KeyEnc — 账号私钥解密；密码登录成功时解密并下发 client_private_key。
//
// 线程安全：每条 transport.Conn 独立 Attach；字段在 Attach 后只读，依赖下游并发安全。
type ServerHandler struct {
	Store                     *persist.Store
	SessMgr                   *sessionmgr.Manager
	ServerKP                  crypto.KeyPair
	TunDev                    tun.Device
	AllowedSourceIPs          []string
	AllowPlaintextPrivateKeys bool
	Probe                     ProbeRecorder
	VPN                       *vpnaccount.Service
	MTU                       int
	GatewayIP                 string
	DNSServers                []string
	VPNSubnet                 string
	Auth                      *auth.Service
	KeyEnc                    *security.KeyEnc
}

// ProbeRecorder 握手拒绝时的探针记录窄接口（避免 tunnel→probedefense 硬依赖循环）。
type ProbeRecorder interface {
	RecordReject(ip, port, phase, signature, detail string)
}

// Attach 绑定到 transport 连接：首帧握手，之后转发 IP 包。
//
// 参数：conn — 已 StateConnected 的 TLS 连接；首帧 Data/Handshake 触发 doHandshake（once）。
// 副作用：设置 conn.onData；握手成功后切换为数据转发回调。
// 并发：每条连接独立 Attach；ServerHandler 字段只读。
func (h *ServerHandler) Attach(conn *transport.Conn) {
	var once sync.Once
	conn.SetOnData(func(data []byte) {
		once.Do(func() {
			h.doHandshake(conn, data)
		})
	})
}

// rejectHandshake 向客户端发送 handshake_err 并关闭连接。
//
// 参数：err — 失败原因；优先用 errors.Is 映射探针 signature，Error() 写入 JSON 与日志。
// 副作用：打 Warn 日志；可选记探针事件；SendRawSync 握手失败帧；conn.Close。
// 并发：仅在 doHandshake 所在 goroutine 调用。
func (h *ServerHandler) rejectHandshake(conn *transport.Conn, err error) {
	if err == nil {
		err = errors.New("握手失败")
	}
	msg := err.Error()
	remote := conn.RemoteAddr()
	logger.Warn("握手拒绝: remote=%s %s", remote, msg)
	if h.Probe != nil {
		ip, port := netutil.SplitRemoteAddr(remote)
		sig := classifyHandshakeReject(err)
		// 密码失败不参与自动封禁计数（由 Guard ignore auth_failed）；仍记流水便于排查
		h.Probe.RecordReject(ip, port, "handshake", sig, msg)
	}
	errBytes, encErr := EncodeHandshakeErr(msg)
	if encErr != nil {
		logger.Warn("编码握手错误帧失败: %v", encErr)
	} else if sendErr := conn.SendRawSync(transport.FrameTypeHandshake, errBytes); sendErr != nil {
		logger.Warn("发送握手错误帧失败 remote=%s: %v", remote, sendErr)
	}
	conn.Close()
}

// classifyHandshakeReject 将握手失败映射为探针 signature（errors.Is 优先，避免中文子串漂移）。
func classifyHandshakeReject(err error) string {
	switch {
	case errors.Is(err, auth.ErrAccountAlreadyOnline):
		return "account_online"
	case errors.Is(err, auth.ErrBadCredentials):
		return "auth_failed"
	case errors.Is(err, ErrSourceDenied):
		return "source_deny"
	case errors.Is(err, auth.ErrLoginLocked):
		return "auth_failed" // 锁定不参与自动封（与 auth_failed 同 ignore）
	default:
		// 兼容包装文案「注册会话失败: …已在其他设备在线」
		msg := err.Error()
		if strings.Contains(msg, "已在其他设备在线") {
			return "account_online"
		}
		if strings.Contains(msg, "用户名或密码") {
			return "auth_failed"
		}
		if strings.Contains(msg, "白名单") || strings.Contains(msg, "tunnel_allowed") {
			return "source_deny"
		}
		return "handshake_reject"
	}
}

// doHandshake 校验身份、分配 IP、下发策略与（密码登录时）客户端私钥。
//
// 参数：conn — 当前 TLS 连接；data — 首帧握手 JSON 载荷。
// 副作用：可能写 sessionmgr、发 handshake_ok、切换 onData 为数据转发；失败时 rejectHandshake。
// 并发：每条连接 once 调用；与 Attach 同 goroutine（transport readLoop 回调）。
func (h *ServerHandler) doHandshake(conn *transport.Conn, data []byte) {
	// --- 阶段 1：来源 IP 白名单与请求解析 ---
	if err := CheckTunnelSourceIP(conn.RemoteAddr(), h.AllowedSourceIPs); err != nil {
		h.rejectHandshake(conn, err)
		return
	}
	req, err := ParseHandshakeRequest(data)
	if err != nil {
		h.rejectHandshake(conn, err)
		return
	}

	remoteIP := netutil.HostFromAddr(conn.RemoteAddr())

	// --- 阶段 2：账号密码鉴权（拒绝废弃公钥模式） ---
	if strings.TrimSpace(req.Username) == "" {
		if strings.TrimSpace(req.PublicKey) != "" {
			logger.Warn("握手拒绝已废弃的公钥模式 remote=%s", remoteIP)
			h.rejectHandshake(conn, auth.ErrUsePasswordLogin)
			return
		}
		h.rejectHandshake(conn, auth.ErrInvalidHandshake)
		return
	}
	if h.Auth == nil {
		h.rejectHandshake(conn, errors.New("服务端未启用账号密码鉴权"))
		return
	}
	if req.Password == "" {
		h.rejectHandshake(conn, auth.ErrPasswordRequired)
		return
	}
	user, err := h.Auth.VerifyTunnelLogin(req.Username, req.Password, remoteIP)
	if err != nil {
		h.rejectHandshake(conn, err)
		return
	}

	// --- 阶段 3：解密客户端私钥（默认拒绝库内明文，防 DB 泄露即得线密钥） ---
	var clientPriv string
	if h.KeyEnc != nil && user.PrivateKeyEnc != "" && security.IsEncryptedPrivateKey(user.PrivateKeyEnc) {
		plain, err := h.KeyEnc.OpenPrivateKey(user.PrivateKeyEnc)
		if err != nil {
			h.rejectHandshake(conn, errors.New("解密账号密钥失败"))
			return
		}
		clientPriv = plain
	} else if user.PrivateKeyEnc != "" && !security.IsEncryptedPrivateKey(user.PrivateKeyEnc) {
		if !h.AllowPlaintextPrivateKeys {
			logger.Warn("拒绝明文私钥账号 user=%s（设 security.allow_plaintext_private_keys=true 仅作兼容）", user.Username)
			h.rejectHandshake(conn, errors.New("账号密钥须加密存储"))
			return
		}
		logger.Warn("兼容模式使用明文私钥 user=%s（生产应关闭 allow_plaintext_private_keys）", user.Username)
		clientPriv = user.PrivateKeyEnc
	} else {
		h.rejectHandshake(conn, errors.New("账号密钥不可用"))
		return
	}

	// --- 阶段 4：会话准入校验与 VPN IP 分配 ---
	if err := h.SessMgr.ValidateVPNAccess(user); err != nil {
		h.rejectHandshake(conn, err)
		return
	}

	vpnIP, err := h.VPN.EnsureVPNIP(user)
	if err != nil {
		h.rejectHandshake(conn, fmt.Errorf("VPN IP 分配失败: %w", err))
		return
	}
	user.VPNIP = vpnIP
	// 先写本地网段注册表，再解析策略（托管路由有效性依赖 registry）
	h.applyLANRegistry(user.ID, vpnIP, req)

	clientPol, err := h.VPN.ResolveClientPolicy(user)
	if err != nil {
		h.rejectHandshake(conn, fmt.Errorf("解析客户端策略失败: %w", err))
		return
	}
	allowed := clientPol.AllowedIPs
	peerReg := sessionmgr.PeerReg{
		PeerAccessIDs: clientPol.PeerAccessIDs,
		ViaUserIDs:    clientPol.ViaUserIDs,
	}
	for _, mr := range clientPol.ManagedRoutes {
		if mr.Stale {
			continue // 会话 ViaRoutes 仅含有效托管路由；hub 不向失效 via 转发
		}
		peerReg.ViaRoutes = append(peerReg.ViaRoutes, sessionmgr.ViaRouteSpec{
			DestCIDR: mr.DestCIDR, ViaUserID: mr.ViaUserID,
		})
	}

	// --- 阶段 5：建立加密会话并注册在线状态 ---
	cryptoSess, err := crypto.NewSession(h.ServerKP.PrivateKey, user.PublicKey)
	if err != nil {
		h.rejectHandshake(conn, errors.New("加密会话建立失败"))
		return
	}

	userID := user.ID
	if err := h.SessMgr.RegisterVPN(user, allowed, conn, cryptoSess, conn.RemoteAddr(), peerReg); err != nil {
		// 保留底层哨兵（如 ErrAccountAlreadyOnline）供 classifyHandshakeReject / errors.Is
		h.rejectHandshake(conn, fmt.Errorf("注册会话失败: %w", err))
		return
	}
	// Register 之后再绑 OnClose；若注册瞬间连接已死则立刻摘掉，避免僵尸会话。
	conn.SetOnClose(func(error) {
		h.SessMgr.RemoveIfConn(userID, conn)
	})
	if conn.State() != transport.StateConnected {
		h.SessMgr.RemoveIfConn(userID, conn)
		logger.Warn("注册后连接已断开，放弃会话 user_id=%d", userID)
		return
	}

	// --- 阶段 6：下发握手成功应答与策略 ---
	mtu := h.MTU
	if mtu <= 0 {
		mtu = netutil.ResolveMTU(h.MTU)
	}
	policy := HandshakePolicy{
		VPNIP:      vpnIP,
		GatewayIP:  h.GatewayIP,
		AllowedIPs: allowed,
		DNSServers: h.resolveDNSServers(),
		MTU:        mtu,
		IPMode:     user.IPMode,
		PolicyVer:  user.PolicyVer,
		VPNSubnet:  strings.TrimSpace(h.VPNSubnet),
	}
	for _, mr := range clientPol.ManagedRoutes {
		policy.ManagedRoutes = append(policy.ManagedRoutes, ManagedRoute{
			Dest: mr.DestCIDR, ViaIP: mr.ViaIP, ViaUserID: mr.ViaUserID,
			ViaUsername: mr.ViaUsername, Stale: mr.Stale,
		})
	}
	okBytes, encErr := EncodeHandshakeOKWithKey(h.ServerKP.PublicKey, clientPriv, policy)
	if encErr != nil {
		h.SessMgr.RemoveIfConn(userID, conn)
		logger.Warn("编码握手成功帧失败，已回滚会话 user_id=%d: %v", userID, encErr)
		h.rejectHandshake(conn, errors.New("握手应答编码失败"))
		return
	}
	if sendErr := conn.SendRaw(transport.FrameTypeHandshake, okBytes); sendErr != nil {
		h.SessMgr.RemoveIfConn(userID, conn)
		logger.Warn("发送握手成功帧失败，已回滚会话 user_id=%d: %v", userID, sendErr)
		conn.Close()
		return
	}

	// --- 阶段 7：切换为 IP 包双向转发 ---
	conn.SetOnData(func(payload []byte) {
		_ = h.SessMgr.HandleInbound(userID, payload, func(pkt []byte) error {
			if h.TunDev == nil {
				logger.Warn("TUN 未就绪，丢弃入站包 user_id=%d len=%d", userID, len(pkt))
				return nil
			}
			_, err := h.TunDev.Write(pkt)
			return err
		})
	})
	logger.Info("隧道握手完成 user=%s id=%d vpn_ip=%s policy_ver=%d", user.Username, user.ID, vpnIP, user.PolicyVer)
}

// applyLANRegistry 按握手 local_lans 写入或清空临时注册表。
//
// 非空有效列表：ReplaceClientLANRegistry；空列表：Clear（配置关闭 via 广告，避免残留）。
// 过宽/非 RFC1918/与 vpn.subnet 重叠的 CIDR 逐条 Warn（lan_cidr_reject），不写入 ExitLAN。
// 为何禁 VPN 池重叠：ExitLAN 可旁路 peer_access，广告 VPN 网段等于允许伪造他户 VPN 源。
// 断线时仍会再 Clear（幂等）。
func (h *ServerHandler) applyLANRegistry(userID int64, vpnIP string, req HandshakeRequest) {
	if h.Store == nil {
		return
	}
	forbid := []string{strings.TrimSpace(h.VPNSubnet)}
	// 逐条校验以便埋点；ValidLANCIDRsNotForbidden 仅返回通过项
	for _, raw := range req.LocalLANs {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		if _, err := netutil.ValidateAdvertisedLANNotForbidden(raw, forbid...); err != nil {
			logger.Warn("lan_cidr_reject user_id=%d cidr=%q reason=%v", userID, raw, err)
		}
	}
	lans := netutil.ValidLANCIDRsNotForbidden(req.LocalLANs, forbid...)
	if len(lans) == 0 {
		if len(req.LocalLANs) > 0 {
			logger.Warn("lan_registry_skip user_id=%d reason=no_valid_cidrs raw=%v", userID, req.LocalLANs)
		}
		if err := h.Store.ClearClientLANRegistry(userID); err != nil {
			logger.Warn("lan_registry_clear_on_empty_fail user_id=%d: %v", userID, err)
		}
		return
	}
	if err := h.Store.ReplaceClientLANRegistry(userID, vpnIP, req.HostID, lans); err != nil {
		logger.Error("lan_registry_report_failed user_id=%d: %v", userID, err)
		return
	}
	logger.Info("lan_registry_reported user_id=%d vpn_ip=%s count=%d", userID, vpnIP, len(lans))
}

func (h *ServerHandler) resolveDNSServers() []string {
	if len(h.DNSServers) > 0 {
		return append([]string{}, h.DNSServers...)
	}
	if h.GatewayIP != "" {
		return []string{h.GatewayIP}
	}
	return nil
}
