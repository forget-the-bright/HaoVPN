package autherr

import (
	"errors"
	"strings"

	"haovpn/internal/auth"
	"haovpn/internal/dialerr"
)

// ErrSourceDenied 隧道来源不在 tunnel_allowed_source_ips；与 dialerr 同一枚哨兵（全仓库唯一）。
var ErrSourceDenied = dialerr.ErrSourceDenied

// 握手失败线上稳定 code（JSON handshake_err.code）；新旧客户端兼容：无 code 时回退文案/Classify。
const (
	CodeBadCredentials      = "bad_credentials"
	CodeAccountDisabled     = "account_disabled"
	CodeLoginLocked         = "login_locked"
	CodeMustChangePassword  = "must_change_password"
	CodeNoVPN               = "no_vpn"
	CodePasswordRequired    = "password_required"
	CodeUsePasswordLogin    = "use_password_login"
	CodeInvalidHandshake    = "invalid_handshake"
	CodeAccountOnline       = "account_online"
	CodeSourceDenied        = "source_denied"
	CodeIPBanned            = "ip_banned"
	CodePlaintextBeforeTLS  = "plaintext_before_tls"
	CodeHandshakeReject     = "handshake_reject"
)

// Category 握手/鉴权错误分类（供 clientapp 与 probedefense 映射不同下游行为）。
type Category int

const (
	// CategoryUnknown 无法归类（极少；通常不会进入业务分支）。
	CategoryUnknown Category = iota
	// CategoryIPBanned 服务端 TLS 前封禁（HAOVPN:IP_BANNED）。
	CategoryIPBanned
	// CategoryAccountOnline 同账号已在线（有限重试，非立即 fatal）。
	CategoryAccountOnline
	// CategoryAuthFailed 凭据错误或登录锁定（探针记 auth_failed）。
	CategoryAuthFailed
	// CategorySourceDenied 源 IP 不在 tunnel_allowed_source_ips。
	CategorySourceDenied
	// CategoryFatalAuth 其它应停重连的鉴权失败（禁用、须改密、无 VPN 等）。
	CategoryFatalAuth
	// CategoryHandshakeReject 通用握手拒绝（探针记 handshake_reject）。
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

// fatalAuthLegacySubstrs 历史/截断文案兜底（无 code 的旧服务端）。
var fatalAuthLegacySubstrs = []string{
	"须修改密码",
	"缺少账号密码",
	"登录已锁定",
	"登录失败次数过多",
	"IP 已锁定",
	"用户名或密码",
}

// sourceDeniedLegacySubstrs 源白名单拒绝的中文/配置关键词（无 code 时兜底；与 IsSourceDenied 共用）。
var sourceDeniedLegacySubstrs = []string{"白名单", "tunnel_allowed"}

// accountOnlineLegacySubstrs 「已在线」文案兜底（与 IsAccountAlreadyOnline 共用，避免双份维护）。
var accountOnlineLegacySubstrs = []string{"已在其他设备在线"}

// containsAnySubstr 判断 msg 是否包含任一子串。
func containsAnySubstr(msg string, substrs []string) bool {
	for _, s := range substrs {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
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
	if IsSourceDenied(err) {
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
	// Is* 已覆盖哨兵与子串；此处仅兜底「未走 Is* 的截断文案」——实际上 Is* 已查过，
	// 再扫 fatalAuthLegacySubstrs 处理须改密等无独立 Is* 的历史路径。
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
	return err != nil && errors.Is(err, dialerr.ErrIPBanned)
}

// IsSourceDenied 是否为源 IP 白名单拒绝（TLS 前 banner 或隧道握手层）。
//
// 优先 errors.Is(ErrSourceDenied)；无 code 旧路径用 sourceDeniedLegacySubstrs。
func IsSourceDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrSourceDenied) {
		return true
	}
	return containsAnySubstr(err.Error(), sourceDeniedLegacySubstrs)
}

// IsAccountAlreadyOnline 是否为「同账号已在其他设备在线」类错误。
//
// 优先 errors.Is；再匹配完整 Error() 与 accountOnlineLegacySubstrs。
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
	return containsAnySubstr(msg, accountOnlineLegacySubstrs)
}

// IsFatalAuth 是否为应停止自动重连的鉴权类失败（不含 account_online）。
func IsFatalAuth(err error) bool {
	if err == nil || IsAccountAlreadyOnline(err) {
		return false
	}
	if errors.Is(err, dialerr.ErrPlaintextBeforeTLS) {
		return true
	}
	c := Classify(err)
	switch c {
	case CategoryIPBanned, CategoryFatalAuth, CategoryAuthFailed, CategorySourceDenied:
		return true
	default:
		return false
	}
}

// HandshakeCode 将 in-process 错误映射为线上稳定 code；无法识别时返回 CodeHandshakeReject。
//
// 关联：tunnel.rejectHandshake → EncodeHandshakeErrCode；客户端 FromHandshakeCode 还原哨兵。
func HandshakeCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, dialerr.ErrIPBanned):
		return CodeIPBanned
	case errors.Is(err, ErrSourceDenied):
		return CodeSourceDenied
	case errors.Is(err, dialerr.ErrPlaintextBeforeTLS):
		return CodePlaintextBeforeTLS
	case errors.Is(err, auth.ErrBadCredentials):
		return CodeBadCredentials
	case errors.Is(err, auth.ErrAccountDisabled):
		return CodeAccountDisabled
	case errors.Is(err, auth.ErrLoginLocked):
		return CodeLoginLocked
	case errors.Is(err, auth.ErrMustChangePassword):
		return CodeMustChangePassword
	case errors.Is(err, auth.ErrNoVPN):
		return CodeNoVPN
	case errors.Is(err, auth.ErrPasswordRequired):
		return CodePasswordRequired
	case errors.Is(err, auth.ErrUsePasswordLogin):
		return CodeUsePasswordLogin
	case errors.Is(err, auth.ErrInvalidHandshake):
		return CodeInvalidHandshake
	case errors.Is(err, auth.ErrAccountAlreadyOnline):
		return CodeAccountOnline
	}
	// 无哨兵时按分类兜底，避免旧路径只有中文文案时 code 全空。
	switch Classify(err) {
	case CategoryIPBanned:
		return CodeIPBanned
	case CategorySourceDenied:
		return CodeSourceDenied
	case CategoryAccountOnline:
		return CodeAccountOnline
	case CategoryAuthFailed:
		return CodeBadCredentials
	case CategoryFatalAuth:
		return CodeHandshakeReject
	default:
		return CodeHandshakeReject
	}
}

// FromHandshakeCode 将线上 code 还原为可 errors.Is 的哨兵；未知/空 code 返回 nil。
//
// 调用方：无 code 时应用文案构造 error 再走 Classify；有 code 时优先本函数。
func FromHandshakeCode(code string) error {
	switch code {
	case CodeBadCredentials:
		return auth.ErrBadCredentials
	case CodeAccountDisabled:
		return auth.ErrAccountDisabled
	case CodeLoginLocked:
		return auth.ErrLoginLocked
	case CodeMustChangePassword:
		return auth.ErrMustChangePassword
	case CodeNoVPN:
		return auth.ErrNoVPN
	case CodePasswordRequired:
		return auth.ErrPasswordRequired
	case CodeUsePasswordLogin:
		return auth.ErrUsePasswordLogin
	case CodeInvalidHandshake:
		return auth.ErrInvalidHandshake
	case CodeAccountOnline:
		return auth.ErrAccountAlreadyOnline
	case CodeSourceDenied:
		return ErrSourceDenied
	case CodeIPBanned:
		return dialerr.ErrIPBanned
	case CodePlaintextBeforeTLS:
		return dialerr.ErrPlaintextBeforeTLS
	default:
		return nil
	}
}
