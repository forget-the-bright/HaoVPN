package tunnel

import (
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
//   GatewayIP — TUN 网关 IP；写入 HandshakePolicy；托管 DNS 无命中时作 dns 回落。
//   VPNSubnet — VPN 地址池 CIDR；下发给客户端作 via 出口 SNAT 源。
//   Auth — 隧道账号密码鉴权；nil 时拒绝密码握手。
//   KeyEnc — 账号私钥解密；密码登录成功时解密并下发 client_private_key。
//
// 线程安全：每条 transport.Conn 独立 Attach；字段在 Attach 后只读，依赖下游并发安全。
// 握手编排文件簇：
//   server_handshake_auth.go — 阶段 1～3（源 IP / 鉴权 / 私钥）；
//   server_handshake_session.go — 阶段 4～7（IP / 策略 / 注册 / OK / 转发）；
//   handshake_reject.go — 拒绝 + ProbeRecorder。
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
	VPNSubnet                 string
	Auth                      *auth.Service
	KeyEnc                    *security.KeyEnc
}

// ProbeRecorder 握手拒绝时的探针记录窄接口（避免 tunnel→probedefense 硬依赖）。
//
// 实现方（如 probedefense.Guard）内部做 SplitRemoteAddr + ClassifyHandshakeReject + RecordReject。
type ProbeRecorder interface {
	OnHandshakeReject(remoteAddr string, err error)
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

// doHandshake 编排握手全流程：鉴权阶段成功后再进会话阶段。
//
// 参数：conn — 当前 TLS 连接；data — 首帧握手 JSON 载荷。
// 副作用：可能写 sessionmgr、发 handshake_ok、切换 onData；失败时 rejectHandshake。
// 并发：每条连接 once 调用；与 Attach 同 goroutine（transport readLoop 回调）。
// 为何拆阶段：降低单函数耦合，阅读时按 auth → session 文件定位。
func (h *ServerHandler) doHandshake(conn *transport.Conn, data []byte) {
	authOK, ok := h.handshakeAuth(conn, data)
	if !ok {
		return
	}
	h.handshakeSession(conn, authOK)
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
