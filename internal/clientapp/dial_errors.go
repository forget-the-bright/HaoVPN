package clientapp

import (
	"fmt"

	"haovpn/internal/autherr"
)

// userMsgIPBanned 客户端展示用：IP 被服务端封禁。
const userMsgIPBanned = "您的 IP 已被服务端封禁，无法连接。请联系管理员在管理台「探针」页解封或加入豁免名单。"

// FormatDialError 将拨号/TLS 错误转为用户可读中文（探针封禁等）。
func FormatDialError(err error) string {
	if err == nil {
		return ""
	}
	if autherr.IsIPBanned(err) {
		return userMsgIPBanned
	}
	return fmt.Sprintf("无法连接服务器: %v", err)
}

// IsIPBannedDialError 是否为服务端封禁导致的连接失败。
func IsIPBannedDialError(err error) bool {
	return autherr.IsIPBanned(err)
}
