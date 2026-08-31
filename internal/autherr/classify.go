package autherr

import (
	"errors"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/transport"
)

// ErrSourceDenied 隧道来源不在 tunnel_allowed_source_ips；tunnel 与 probedefense 共用。
var ErrSourceDenied = errors.New("隧道来源 IP 不在 tunnel_allowed_source_ips 白名单内")

// Category 握手/鉴权错误分类（供 clientapp 与 probedefense 映射不同下游行为）。
type Category int

const (
	// CategoryUnknown 无法识别或非错误。
	CategoryUnknown Category = iota
	// CategoryIPBanned 源 IP 被服务端封禁（TLS 前 banner）。
	CategoryIPBanned
	// CategoryAccountOnline 同账号已在其他设备在线（clientapp 有限重试，非立即 fatal）。
	CategoryAccountOnline
	// CategoryAuthFailed 凭据/锁定类（探针 ignore 自动封）。
	CategoryAuthFailed
	// CategorySourceDenied 隧道源 IP 白名单拒绝。
	CategorySourceDenied
	// CategoryFatalAuth 其它不可重试鉴权失败（禁用、须改密、无 VPN 等）。
	CategoryFatalAuth
	// CategoryHandshakeReject 通用握手拒绝（探针默认 signature）。
	CategoryHandshakeReject
)

// fatalAuthSentinels 与 auth 导出哨兵对齐（不含 account_online，由有限重试单独处理）。
var fatalAuthSentinels = []error{
	auth.ErrBadCredentials,
	auth.ErrAccountDisabled,
	auth.ErrLoginLocked,
	auth.ErrMustChangePassword,
	auth.ErrNoVPN,
	auth.ErrPasswordRequired,
	auth.ErrUsePasswordLogin,
	auth.ErrInvalidHandshake,
}

// fatalAuthLegacySubstrs 历史/截断文案兜底。
var fatalAuthLegacySubstrs = []string{
	"须修改密码",
	"缺少账号密码",
	"登录已锁定",
	"登录失败次数过多",
	"IP 已锁定",
	"用户名或密码",
}

// Classify 将握手/拨号错误映射为 Category；errors.Is 优先，中文子串兜底。
func Classify(err error) Category {
	if err == nil {
		return CategoryUnknown
	}
	if IsIPBanned(err) {
		return CategoryIPBanned
	}
	if IsAccountAlreadyOnline(err) {
		return CategoryAccountOnline
	}
	if errors.Is(err, ErrSourceDenied) {
		return CategorySourceDenied
	}
	if errors.Is(err, auth.ErrBadCredentials) || errors.Is(err, auth.ErrLoginLocked) {
		return CategoryAuthFailed
	}
	for _, target := range fatalAuthSentinels {
		if errors.Is(err, target) {
			if errors.Is(err, auth.ErrBadCredentials) || errors.Is(err, auth.ErrLoginLocked) {
				return CategoryAuthFailed
			}
			return CategoryFatalAuth
		}
	}
	msg := err.Error()
	if strings.Contains(msg, "已在其他设备在线") || strings.Contains(msg, auth.ErrAccountAlreadyOnline.Error()) {
		return CategoryAccountOnline
	}
	if strings.Contains(msg, "白名单") || strings.Contains(msg, "tunnel_allowed") {
		return CategorySourceDenied
	}
	for _, s := range fatalAuthLegacySubstrs {
		if strings.Contains(msg, s) {
			if strings.Contains(s, "用户名或密码") || strings.Contains(s, "登录") {
				return CategoryAuthFailed
			}
			return CategoryFatalAuth
		}
	}
	return CategoryHandshakeReject
}

// IsIPBanned 是否为服务端 IP 封禁（TLS 前 HAOVPN:IP_BANNED）。
func IsIPBanned(err error) bool {
	return err != nil && errors.Is(err, transport.ErrIPBanned)
}

// IsAccountAlreadyOnline 是否为「同账号已在其他设备在线」类错误。
func IsAccountAlreadyOnline(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, auth.ErrAccountAlreadyOnline) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, auth.ErrAccountAlreadyOnline.Error()) {
		return true
	}
	return strings.Contains(msg, "已在其他设备在线")
}

// IsFatalAuth 是否为应停止自动重连的鉴权类失败（不含 account_online、IP 封禁由调用方单独处理）。
func IsFatalAuth(err error) bool {
	if err == nil || IsAccountAlreadyOnline(err) {
		return false
	}
	c := Classify(err)
	switch c {
	case CategoryIPBanned, CategoryFatalAuth, CategoryAuthFailed, CategorySourceDenied:
		return true
	default:
		return false
	}
}
