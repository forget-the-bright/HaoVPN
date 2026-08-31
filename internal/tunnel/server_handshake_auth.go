package tunnel

import (
	"errors"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/persist"
	"haovpn/internal/security"
	"haovpn/internal/transport"
)

// handshakeAuthOK 握手鉴权阶段成功产出（阶段 1～3）。
//
// 字段：
//   user — 已通过 VerifyTunnelLogin 的账号行；
//   clientPriv — 解密后的客户端私钥明文（下发 handshake_ok 用，勿写日志）；
//   req — 已解析的握手请求（后续 LAN 注册依赖 LocalLANs）。
type handshakeAuthOK struct {
	user       *persist.User
	clientPriv string
	req        HandshakeRequest
}

// handshakeAuth 阶段 1～3：源 IP 白名单、解析请求、账号密码鉴权、解密私钥。
//
// 参数：conn — 当前 TLS 连接；data — 首帧握手 JSON。
// 返回：成功产出；失败时已调用 rejectHandshake，ok=false（调用方勿再 reject）。
// 关联：netutil.CheckSourceIPAllowed、auth.VerifyTunnelLogin、security.KeyEnc。
func (h *ServerHandler) handshakeAuth(conn *transport.Conn, data []byte) (ok handshakeAuthOK, success bool) {
	// --- 阶段 1：来源 IP 白名单与请求解析 ---
	// 直接调 netutil（禁止本包薄包装 CheckTunnelSourceIP）。
	if err := netutil.CheckSourceIPAllowed(conn.RemoteAddr(), h.AllowedSourceIPs); err != nil {
		h.rejectHandshake(conn, err)
		return ok, false
	}
	req, err := ParseHandshakeRequest(data)
	if err != nil {
		h.rejectHandshake(conn, err)
		return ok, false
	}

	remoteIP := netutil.HostFromAddr(conn.RemoteAddr())

	// --- 阶段 2：账号密码鉴权（拒绝废弃公钥模式） ---
	if strings.TrimSpace(req.Username) == "" {
		if strings.TrimSpace(req.PublicKey) != "" {
			logger.Warn("握手拒绝已废弃的公钥模式 remote=%s", remoteIP)
			h.rejectHandshake(conn, auth.ErrUsePasswordLogin)
			return ok, false
		}
		h.rejectHandshake(conn, auth.ErrInvalidHandshake)
		return ok, false
	}
	if h.Auth == nil {
		h.rejectHandshake(conn, errors.New("服务端未启用账号密码鉴权"))
		return ok, false
	}
	if req.Password == "" {
		h.rejectHandshake(conn, auth.ErrPasswordRequired)
		return ok, false
	}
	user, err := h.Auth.VerifyTunnelLogin(req.Username, req.Password, remoteIP)
	if err != nil {
		h.rejectHandshake(conn, err)
		return ok, false
	}

	// --- 阶段 3：解密客户端私钥（默认拒绝库内明文，防 DB 泄露即得线密钥） ---
	clientPriv, err := h.decryptClientPrivateKey(user)
	if err != nil {
		h.rejectHandshake(conn, err)
		return ok, false
	}

	return handshakeAuthOK{user: user, clientPriv: clientPriv, req: req}, true
}

// decryptClientPrivateKey 按策略解密或兼容读取账号私钥。
//
// 参数：user — 已鉴权账号。
// 返回：明文私钥；不可用时返回中文 error（由调用方 rejectHandshake）。
// 为何默认拒明文：库被拖走时明文私钥可直接冒充客户端。
func (h *ServerHandler) decryptClientPrivateKey(user *persist.User) (string, error) {
	if h.KeyEnc != nil && user.PrivateKeyEnc != "" && security.IsEncryptedPrivateKey(user.PrivateKeyEnc) {
		plain, err := h.KeyEnc.OpenPrivateKey(user.PrivateKeyEnc)
		if err != nil {
			return "", errors.New("解密账号密钥失败")
		}
		return plain, nil
	}
	if user.PrivateKeyEnc != "" && !security.IsEncryptedPrivateKey(user.PrivateKeyEnc) {
		if !h.AllowPlaintextPrivateKeys {
			logger.Warn("拒绝明文私钥账号 user=%s（设 security.allow_plaintext_private_keys=true 仅作兼容）", user.Username)
			return "", errors.New("账号密钥须加密存储")
		}
		logger.Warn("兼容模式使用明文私钥 user=%s（生产应关闭 allow_plaintext_private_keys）", user.Username)
		return user.PrivateKeyEnc, nil
	}
	return "", errors.New("账号密钥不可用")
}
