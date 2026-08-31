package clientapp

import (
	"errors"
	"fmt"

	"haovpn/internal/autherr"
	"haovpn/internal/transport"
)

// 客户端展示用文案（与 docs/security-hardening.md 封禁提示对齐）。
const (
	userMsgIPBanned           = "您的 IP 已被服务端封禁，无法连接。请联系管理员在管理台「探针」页解封或加入豁免名单。"
	userMsgSourceDenied       = "您的公网 IP 不在服务端 tunnel_allowed_source_ips 白名单内，无法连接。请联系管理员。"
	userMsgPlaintextBeforeTLS = "服务器在 TLS 前返回了明文（常见原因：您的 IP 已被封禁，或服务器地址/端口不是 HaoVPN 隧道口）。请到管理台「探针」页检查封禁，并核对连接地址。"
	userMsgClosedBeforeTLS    = "服务器在 TLS 握手前关闭了连接，请稍后重试；若持续失败请联系管理员检查封禁与源 IP 白名单。"
)

// FormatDialError 将拨号/TLS 错误转为用户可读中文（探针封禁、源拒绝等）。
func FormatDialError(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case autherr.IsIPBanned(err):
		return userMsgIPBanned
	case autherr.IsSourceDenied(err):
		return userMsgSourceDenied
	case errors.Is(err, transport.ErrPlaintextBeforeTLS):
		return userMsgPlaintextBeforeTLS
	case errors.Is(err, transport.ErrClosedBeforeTLS):
		return userMsgClosedBeforeTLS
	default:
		return fmt.Sprintf("无法连接服务器: %v", err)
	}
}

// IsIPBannedDialError 是否为服务端封禁导致的连接失败。
func IsIPBannedDialError(err error) bool {
	return autherr.IsIPBanned(err)
}

// IsFatalDialError 拨号阶段应停止重连并提示用户的错误。
func IsFatalDialError(err error) bool {
	if err == nil {
		return false
	}
	if transport.IsFatalDialError(err) {
		return true
	}
	return autherr.IsSourceDenied(err) || IsFatalHandshakeError(err)
}
