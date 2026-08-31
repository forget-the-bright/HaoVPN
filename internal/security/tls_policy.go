package security

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"strings"

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

// Redact 从日志或 API 响应字符串中脱敏密码、token 与疑似密钥材料。
//
// 为何保留此薄委托（禁止「顺手删掉」）：
//   - 脱敏实现落在 logger（写日志路径天然需要）；
//   - security 检查清单/测试/部分调用方习惯 security.Redact；
//   - 若把实现放进 security 再让 logger 引用 → logger↔security 循环依赖。
// 新业务代码优先直接调 logger.RedactSensitive；本函数仅为防循环的兼容入口。
// 见 docs/architecture.md「依赖规则」与 logger/redact.go 注释。
func Redact(s string) string {
	return logger.RedactSensitive(s)
}

// SecurityHeaders 返回 Web 管理端推荐的 HTTP 安全响应头。
//
// 参数：无。
// 返回：header 名 → 值的 map。
// 副作用：无。
//
// CSP 说明（诚实、可演进）：
//   - script-src 仅 'self'：管理页脚本已外置到 web/static/*.js，禁止内联脚本；
//   - style-src 仍含 'unsafe-inline'：templates 内大量内联 <style>/style= 尚未迁出；
//     去掉 style 的 unsafe-inline 前须先外置 CSS，否则布局会坏。
// 关联：docs/security-hardening.md；web/static/*.js。
func SecurityHeaders() map[string]string {
	return map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":           "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'",
	}
}

// SecurityHeadersForRequest 按请求上下文追加 HSTS 等动态头。
//
// 参数 secureCookies — 配置 api.secure_cookies；trustedProxyCIDRs — 反代信任列表。
// 当直连 TLS、secure_cookies 或可信反代 X-Forwarded-Proto: https 时设置 HSTS。
func SecurityHeadersForRequest(r *http.Request, secureCookies bool, trustedProxyCIDRs []string) map[string]string {
	h := SecurityHeaders()
	if RequestIsHTTPS(r, secureCookies, trustedProxyCIDRs) {
		h["Strict-Transport-Security"] = "max-age=31536000"
	}
	return h
}

// RequestIsHTTPS 判断管理口请求是否应视为 HTTPS（Cookie Secure / HSTS）。
func RequestIsHTTPS(r *http.Request, secureCookies bool, trustedProxyCIDRs []string) bool {
	if secureCookies {
		return true
	}
	if r == nil {
		return false
	}
	if r.TLS != nil {
		return true
	}
	if len(trustedProxyCIDRs) == 0 {
		return false
	}
	remoteIP := netutil.HostFromAddr(r.RemoteAddr)
	parsed, err := netutil.ParseHostIP(remoteIP)
	if err == nil && netutil.IPMatchesRules(parsed, trustedProxyCIDRs) {
		proto := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")))
		return proto == "https"
	}
	return false
}
