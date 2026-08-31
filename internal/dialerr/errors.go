package dialerr

import "errors"

// TLS 前明文拒绝码（须以 \r\n 结尾；客户端 Peek 见首字节 'H' 后整行解析）。
//
// 为何固定前缀：客户端与服务端共用同一常量，避免文案漂移导致误判可重试。
const (
	BannerIPBanned     = "HAOVPN:IP_BANNED\r\n"
	BannerSourceDenied = "HAOVPN:SOURCE_DENIED\r\n"
)

// banner 前缀（TrimSpace 后匹配；ClassifyRejectBanner* 共用，禁止两处各写一遍）。
const (
	bannerPrefixIPBanned     = "HAOVPN:IP_BANNED"
	bannerPrefixSourceDenied = "HAOVPN:SOURCE_DENIED"
)

// 拨号阶段哨兵（供 ReconnectClient / autherr / FormatDialError / netutil 源白名单识别）。
//
// 全仓库仅此一套；禁止在 transport/autherr 再 new 同义哨兵。
// Error() 统一中文主句：日志与兜底展示一致；对外判定以 errors.Is 为准，勿靠英文子串。
var (
	// ErrIPBanned 对端因 ip_blocks 在 TLS 前拒绝（明确读到 HAOVPN:IP_BANNED）。
	ErrIPBanned = errors.New("服务端已封禁本机 IP（HAOVPN:IP_BANNED）")
	// ErrSourceDenied 源 IP 不在 tunnel_allowed_source_ips（TLS 前 banner 或握手层白名单）。
	// Error() 含「白名单」「tunnel_allowed」供无 code 旧路径子串兜底；对外以 errors.Is 为准。
	ErrSourceDenied = errors.New("隧道来源 IP 不在 tunnel_allowed_source_ips 白名单内")
	// ErrPlaintextBeforeTLS TLS 层读到非握手明文（晚到的封禁 banner、或连错非 HaoVPN 端口）。
	ErrPlaintextBeforeTLS = errors.New("服务器在 TLS 握手前返回了明文")
	// ErrClosedBeforeTLS 对端在 TLS 前关闭且未发可识别 banner（网络闪断或旧版仅 Close）。
	ErrClosedBeforeTLS = errors.New("连接在 TLS 握手前被关闭")
)
