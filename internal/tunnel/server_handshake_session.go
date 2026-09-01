package tunnel

import (
	"errors"
	"fmt"
	"strings"

	"haovpn/internal/crypto"
	"haovpn/internal/logger"
	"haovpn/internal/netutil"
	"haovpn/internal/sessionmgr"
	"haovpn/internal/transport"
)

// handshakeSession 阶段 4～7：准入、IP 分配、策略、注册会话、下发 OK、切换转发。
//
// 参数：conn — 当前 TLS 连接；authOK — 鉴权阶段产出。
// 副作用：写 sessionmgr / LAN 注册表；成功后切换 onData；失败时 rejectHandshake 或回滚会话。
// 关联：vpnaccount.EnsureVPNIP / ResolveClientPolicy、sessionmgr.RegisterVPN、ProbeRecorder（经 reject）。
func (h *ServerHandler) handshakeSession(conn *transport.Conn, authOK handshakeAuthOK) {
	user := authOK.user
	req := authOK.req
	clientPriv := authOK.clientPriv

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
	dnsServers := h.resolveDNSServers()
	// TUN DNS 须进隧道：把 DNS /32 并入 AllowedIPs（会话 dstAllowed + 客户端路由/上送）
	allowed = netutil.MergeDNSIntoAllowedIPs(allowed, dnsServers)
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
		// 保留底层哨兵（如 ErrAccountAlreadyOnline）供 autherr.Classify / probedefense.ClassifyHandshakeReject / errors.Is
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
		DNSServers: dnsServers,
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

	// --- 阶段 7：切换为 IP 包双向转发；Handshake 回调兼容旧客户端 post-auth lan_registry ---
	conn.SetOnHandshake(func(payload []byte) {
		h.handlePostAuthHandshake(userID, vpnIP, payload)
	})
	// 闭包捕获本连接：HandleInbound 用 conn 身份拒绝顶替后旧 readLoop 的迟到包，
	// 避免同钥密文灌进新 Crypto 防重放窗口（local_lans/ICS 软重连现场）。
	conn.SetOnData(func(payload []byte) {
		_ = h.SessMgr.HandleInbound(userID, conn, payload, func(pkt []byte) error {
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
