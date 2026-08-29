package tunnel

import (
	"net"
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
//   Probe — 可选探针防御；握手拒绝时记 security_events（不含密码失败细节刷爆时可关）。
//   VPN — 账号 IP 分配与 allowed_ips 解析服务。
//   MTU — 下发给客户端的 MTU；≤0 时 ResolveMTU 取平台默认。
//   GatewayIP — TUN 网关 IP；写入 HandshakePolicy 与 DNS 回落。
//   DNSServers — 推送给客户端的 DNS 列表；空时回落 GatewayIP。
//   Auth — 隧道账号密码鉴权；nil 时拒绝密码握手。
//   KeyEnc — 账号私钥解密；密码登录成功时解密并下发 client_private_key。
//
// 线程安全：每条 transport.Conn 独立 Attach；字段在 Attach 后只读，依赖下游并发安全。
type ServerHandler struct {
	Store            *persist.Store
	SessMgr          *sessionmgr.Manager
	ServerKP         crypto.KeyPair
	TunDev           tun.Device
	AllowedSourceIPs []string
	Probe            ProbeRecorder
	VPN              *vpnaccount.Service
	MTU              int
	GatewayIP        string
	DNSServers       []string
	Auth             *auth.Service
	KeyEnc           *security.KeyEnc
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
// 参数：msg — 可读错误信息，写入 JSON error 字段。
// 副作用：打 Warn 日志；可选记探针事件；SendRaw 握手失败帧；conn.Close。
// 并发：仅在 doHandshake 所在 goroutine 调用。
func (h *ServerHandler) rejectHandshake(conn *transport.Conn, msg string) {
	remote := conn.RemoteAddr()
	logger.Warn("握手拒绝: remote=%s %s", remote, msg)
	if h.Probe != nil {
		ip := netutil.HostFromAddr(remote)
		port := ""
		if _, p, err := splitPort(remote); err == nil {
			port = p
		}
		sig := "handshake_reject"
		if strings.Contains(msg, "已在其他设备在线") {
			sig = "account_online"
		} else if strings.Contains(msg, "用户名或密码") {
			sig = "auth_failed"
		} else if strings.Contains(msg, "白名单") || strings.Contains(msg, "tunnel_allowed") {
			sig = "source_deny"
		}
		// 密码失败不参与自动封禁计数（由 Guard ignore 或单独 signature）；仍记流水便于排查
		h.Probe.RecordReject(ip, port, "handshake", sig, msg)
	}
	errBytes, _ := EncodeHandshakeErr(msg)
	// 须同步写出：若仅入队后立刻 Close，writeLoop 可能先收到 closed 而丢弃错误帧，客户端会握手超时
	_ = conn.SendRawSync(transport.FrameTypeHandshake, errBytes)
	conn.Close()
}

func splitPort(addr string) (host, port string, err error) {
	return net.SplitHostPort(addr)
}

// doHandshake 校验身份、分配 IP、下发策略与（密码登录时）客户端私钥。
//
// 参数：conn — 当前 TLS 连接；data — 首帧握手 JSON 载荷。
// 副作用：可能写 sessionmgr、发 handshake_ok、切换 onData 为数据转发；失败时 rejectHandshake。
// 并发：每条连接 once 调用；与 Attach 同 goroutine（transport readLoop 回调）。
func (h *ServerHandler) doHandshake(conn *transport.Conn, data []byte) {
	// --- 阶段 1：来源 IP 白名单与请求解析 ---
	if err := CheckTunnelSourceIP(conn.RemoteAddr(), h.AllowedSourceIPs); err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}
	req, err := ParseHandshakeRequest(data)
	if err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}

	remoteIP := netutil.HostFromAddr(conn.RemoteAddr())

	// --- 阶段 2：账号密码鉴权（拒绝废弃公钥模式） ---
	if strings.TrimSpace(req.Username) == "" {
		if strings.TrimSpace(req.PublicKey) != "" {
			logger.Warn("握手拒绝已废弃的公钥模式 remote=%s", remoteIP)
			h.rejectHandshake(conn, "请使用账号密码登录")
			return
		}
		h.rejectHandshake(conn, "无效握手请求")
		return
	}
	if h.Auth == nil {
		h.rejectHandshake(conn, "服务端未启用账号密码鉴权")
		return
	}
	if req.Password == "" {
		h.rejectHandshake(conn, "请提供密码")
		return
	}
	user, err := h.Auth.VerifyTunnelLogin(req.Username, req.Password, remoteIP)
	if err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}

	// --- 阶段 3：解密客户端私钥 ---
	var clientPriv string
	if h.KeyEnc != nil && user.PrivateKeyEnc != "" {
		plain, err := h.KeyEnc.OpenPrivateKey(user.PrivateKeyEnc)
		if err != nil {
			h.rejectHandshake(conn, "解密账号密钥失败")
			return
		}
		clientPriv = plain
	} else if user.PrivateKeyEnc != "" && !security.IsEncryptedPrivateKey(user.PrivateKeyEnc) {
		clientPriv = user.PrivateKeyEnc
	} else {
		h.rejectHandshake(conn, "账号密钥不可用")
		return
	}

	// --- 阶段 4：会话准入校验与 VPN IP 分配 ---
	if err := h.SessMgr.ValidateVPNAccess(user); err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}

	vpnIP, err := h.VPN.EnsureVPNIP(user)
	if err != nil {
		h.rejectHandshake(conn, "VPN IP 分配失败: "+err.Error())
		return
	}
	user.VPNIP = vpnIP
	allowed := h.VPN.ResolveAllowedIPs(user)

	// --- 阶段 5：建立加密会话并注册在线状态 ---
	cryptoSess, err := crypto.NewSession(h.ServerKP.PrivateKey, user.PublicKey)
	if err != nil {
		h.rejectHandshake(conn, "加密会话建立失败")
		return
	}

	userID := user.ID
	if err := h.SessMgr.RegisterVPN(user, allowed, conn, cryptoSess, conn.RemoteAddr()); err != nil {
		h.rejectHandshake(conn, "注册会话失败: "+err.Error())
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
	}
	okBytes, _ := EncodeHandshakeOKWithKey(h.ServerKP.PublicKey, clientPriv, policy)
	_ = conn.SendRaw(transport.FrameTypeHandshake, okBytes)

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

func (h *ServerHandler) resolveDNSServers() []string {
	if len(h.DNSServers) > 0 {
		return append([]string{}, h.DNSServers...)
	}
	if h.GatewayIP != "" {
		return []string{h.GatewayIP}
	}
	return nil
}
