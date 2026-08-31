package persist

import "errors"

// 互访白名单（peer_access）领域错误；API 经 writeDomainError 映射 HTTP 状态。

var (
	// ErrInvalidPeerAccessPair user_id/peer_user_id 非法或相同。
	ErrInvalidPeerAccessPair = errors.New("无效的互访账号对")

	// ErrPeerAccessUserNotFound 互访方或对端账号不存在。
	ErrPeerAccessUserNotFound = errors.New("互访账号不存在")

	// ErrPeerAccessNotVPN 账号存在但未开通 VPN（无密钥/vpn_ip）。
	ErrPeerAccessNotVPN = errors.New("须为 VPN 账号")
)
