package tunnel

import (
	"encoding/json"

	"haovpn/internal/logger"
)

// lanRegistryMaxPayloadBytes post-auth lan_registry 帧最大载荷（防刷库/解析放大）。
const lanRegistryMaxPayloadBytes = 16 * 1024

// handlePostAuthHandshake 会话建立后仍可能收到 Handshake 帧（旧客户端 lan_registry 纠正）。
//
// 新客户端（对齐 2cdc5e6）不再发送 post-auth lan_registry；本路径仅兼容旧版/误发。
// 非 lan_registry 类型忽略（防误用握手重放）；失败仅打日志，不断开隧道。
func (h *ServerHandler) handlePostAuthHandshake(userID int64, vpnIP string, data []byte) {
	var peek struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &peek); err != nil {
		logger.Debug("post_auth_handshake ignore parse_err user_id=%d: %v", userID, err)
		return
	}
	switch peek.Type {
	case "lan_registry":
		h.applyLANRegistrySync(userID, vpnIP, data)
	default:
		logger.Debug("post_auth_handshake ignore type=%s user_id=%d", peek.Type, userID)
	}
}

// applyLANRegistrySync 用客户端 ICS Active 列表整表替换注册表，并刷新活会话路由。
//
// 兼容路径：新客户端不发此帧（注册表仅握手写入）；旧客户端仍可能在 ICS 后纠正 Active。
// 步骤：载荷上限 → 解析 → 会话限速 → 写注册表 → ReloadExitLANs → PruneViaRoutes → Kick 受影响成员。
// 为何 Kick：成员 AllowedIPs/客户端路由仍可能含已跳过 CIDR，仅剪服务端 viaIndex 不够。
// 禁止 Kick via 自己：会关当前隧道，客户端随即灌 Data → decrypt/replay。
func (h *ServerHandler) applyLANRegistrySync(userID int64, vpnIP string, data []byte) {
	if len(data) > lanRegistryMaxPayloadBytes {
		logger.Warn("lan_registry_sync rejected oversized user_id=%d len=%d max=%d",
			userID, len(data), lanRegistryMaxPayloadBytes)
		return
	}
	msg, err := ParseLANRegistryUpdate(data)
	if err != nil {
		logger.Warn("lan_registry_sync parse fail user_id=%d: %v", userID, err)
		return
	}
	if h.SessMgr != nil && !h.SessMgr.AllowLANRegistrySync(userID) {
		logger.Warn("lan_registry_sync rate_limited user_id=%d", userID)
		return
	}
	// 复用握手校验路径：构造伪 HandshakeRequest
	req := HandshakeRequest{Type: "handshake", LocalLANs: msg.LocalLANs, HostID: msg.HostID}
	h.applyLANRegistry(userID, vpnIP, req)
	if h.SessMgr != nil {
		h.SessMgr.ReloadExitLANs(userID)
		affected := h.SessMgr.PruneViaRoutesAfterRegistry(userID)
		for _, id := range affected {
			if id == userID {
				// 禁止踢正在 sync 的 via 自己（会关当前隧道，客户端随即灌 Data → decrypt/replay）
				logger.Info("lan_registry_sync skip_kick_self user_id=%d reason=via_route_pruned", id)
				continue
			}
			logger.Info("lan_registry_sync kick_member user_id=%d via=%d reason=via_route_pruned", id, userID)
			h.SessMgr.KickUser(id)
		}
	}
	logger.Info("lan_registry_sync applied user_id=%d vpn_ip=%s count=%d", userID, vpnIP, len(msg.LocalLANs))
}
