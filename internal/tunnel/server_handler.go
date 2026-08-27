package tunnel

import (
	"net"
	"strings"
	"sync"

	"haovpn/internal/auth"
	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
	"haovpn/internal/tun"
	"haovpn/internal/vpnaccount"
)

// ServerHandler 处理服务端新 TLS 连接的握手与数据转发。
type ServerHandler struct {
	Store            *persist.Store
	SessMgr          *sessionmgr.Manager
	ServerKP         crypto.KeyPair
	TunDev           tun.Device
	AllowedSourceIPs []string
	VPN              *vpnaccount.Service
	MTU              int
	GatewayIP        string
	DNSServers       []string
	Auth             *auth.Service
	KeyEnc           *security.KeyEnc
}

// Attach 绑定到 transport 连接：首帧握手，之后转发 IP 包。
func (h *ServerHandler) Attach(conn *transport.Conn) {
	var once sync.Once
	conn.SetOnData(func(data []byte) {
		once.Do(func() {
			h.doHandshake(conn, data)
		})
	})
}

func (h *ServerHandler) rejectHandshake(conn *transport.Conn, msg string) {
	logger.Warn("握手拒绝: %s", msg)
	errBytes, _ := EncodeHandshakeErr(msg)
	_ = conn.SendRaw(transport.FrameTypeHandshake, errBytes)
	conn.Close()
}

func clientIPFromRemote(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

// doHandshake 校验身份、分配 IP、下发策略与（密码登录时）客户端私钥。
func (h *ServerHandler) doHandshake(conn *transport.Conn, data []byte) {
	if err := CheckTunnelSourceIP(conn.RemoteAddr(), h.AllowedSourceIPs); err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}
	req, err := ParseHandshakeRequest(data)
	if err != nil {
		h.rejectHandshake(conn, err.Error())
		return
	}

	remoteIP := clientIPFromRemote(conn.RemoteAddr())

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

	mtu := h.MTU
	if mtu <= 0 {
		mtu = 1420
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
