package security

import (
	"crypto/tls"
	"crypto/x509"

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
// CSP 残留风险（诚实说明，不假装已消除）：
//   - 仍含 script-src/style-src 'unsafe-inline'，因多数 templates/*.html 仍有内联 <script>/<style>；
//   - 登录页已外置到 static/login.js，其它管理页外置属后续增量；
//   - 若去掉 unsafe-inline 而未迁完脚本，浏览器会拦截 → 白屏/按钮无反应。
// 关联：docs/security-hardening.md；web/static/login.js。
func SecurityHeaders() map[string]string {
	return map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Content-Security-Policy": "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'",
	}
}
