package security

import (
	"crypto/tls"
	"crypto/x509"
	"regexp"

	"haovpn/internal/logger"
	"haovpn/internal/netutil"
)

// TLSConfig 构造服务端或客户端共用的安全 TLS 配置（最低 TLS 1.2、现代 cipher）。
//
// 参数：cert 服务端时写入 Certificates；isServer true 为服务端模式，false 为客户端基础模板。
// 返回：已设 MinVersion 与 CipherSuites 的 *tls.Config；客户端须再设 RootCAs/InsecureSkipVerify。
// 副作用：无；返回新 struct，可安全修改后交给 tls.Listen/Dial。
func TLSConfig(cert tls.Certificate, isServer bool) *tls.Config {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		PreferServerCipherSuites: true,
	}
	if isServer {
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg
}

// ClientTLSConfigWithRootCAs 基于 TLSConfig 生成客户端 TLS，挂载自定义 CA 池。
//
// 参数：rootCAs — 可为 nil（仅 insecure 模式）；insecureSkipVerify 仅开发/内网慎用。
func ClientTLSConfigWithRootCAs(rootCAs *x509.CertPool, insecureSkipVerify bool) *tls.Config {
	cfg := TLSConfig(tls.Certificate{}, false)
	cfg.InsecureSkipVerify = insecureSkipVerify
	cfg.RootCAs = rootCAs
	return cfg
}

// ClientTLSConfig 基于 TLSConfig 生成客户端 TLS，可选挂载 CA 与跳过校验开关。
//
// 参数：caPool 非 nil 时将其 RootCAs 复制到 cfg；insecureSkipVerify 仅开发/内网慎用。
// 返回：可直接用于 tls.Dial 的 *tls.Config。
// 副作用：无。
func ClientTLSConfig(caPool *tls.Config, insecureSkipVerify bool) *tls.Config {
	cfg := TLSConfig(tls.Certificate{}, false)
	cfg.InsecureSkipVerify = insecureSkipVerify
	if caPool != nil {
		cfg.RootCAs = caPool.RootCAs
	}
	return cfg
}

// BindCheck 校验管理 API 监听地址是否符合 allow_public_bind 策略。
//
// 参数：listenHosts 为配置中的 host 列表；allowPublic false 时禁止公网/0.0.0.0 绑定。
// 返回：策略违规时 err；允许但 wildcard 且 allowPublic 时打 Warn 仍返回 nil。
// 副作用：可能写 Warn 日志提醒公网暴露风险。
func BindCheck(listenHosts []string, allowPublic bool) error {
	if err := netutil.ValidatePublicBindPolicy(listenHosts, allowPublic); err != nil {
		return err
	}
	if netutil.HasWildcardListenHost(listenHosts) && allowPublic {
		logger.Warn("PUBLIC BIND ENABLED: management API is exposed on all interfaces. You assume all risks.")
	}
	return nil
}

var (
	rePassword = regexp.MustCompile(`(?i)(password|passwd|secret|token|private_key)\s*[:=]\s*\S+`)
	reKey      = regexp.MustCompile(`(?i)[A-Za-z0-9+/]{40,}={0,2}`)
)

// Redact 从日志或 API 响应字符串中脱敏密码、token 与疑似密钥材料。
//
// 参数：s 原始文本；过长（>200）时额外匹配 base64 样长串。
// 返回：替换为 [REDACTED] / [REDACTED_KEY] 后的副本；不修改原串。
// 副作用：无；纯正则替换。
func Redact(s string) string {
	s = rePassword.ReplaceAllString(s, "$1=[REDACTED]")
	if len(s) > 200 {
		s = reKey.ReplaceAllString(s, "[REDACTED_KEY]")
	}
	return s
}

// SecurityHeaders 返回 Web 管理端推荐的 HTTP 安全响应头。
//
// 参数：无。
// 返回：header 名 → 值的 map；CSP 允许同源内联 script/style（模板无外链 CDN）。
// 副作用：无。
//
// WebUI 模板使用内联 <script>/<style>（无外链 CDN）。若仅设 default-src 'self'，
// 浏览器会拦截内联脚本，表现为「白屏 / 登录按钮无反应」。因此显式允许同源内联。
func SecurityHeaders() map[string]string {
	return map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'",
	}
}
