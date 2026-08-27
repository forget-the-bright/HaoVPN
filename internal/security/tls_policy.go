// Package security provides TLS policy, bind checks, and redaction.
package security

import (
	"crypto/tls"
	"fmt"
	"net"
	"regexp"
	"strings"

	"haovpn/internal/logger"
)

// TLSConfig returns a secure TLS config for server or client.
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

// ClientTLSConfig returns client TLS config with optional CA.
func ClientTLSConfig(caPool *tls.Config, insecureSkipVerify bool) *tls.Config {
	cfg := TLSConfig(tls.Certificate{}, false)
	cfg.InsecureSkipVerify = insecureSkipVerify
	if caPool != nil {
		cfg.RootCAs = caPool.RootCAs
	}
	return cfg
}

// BindCheck validates management API listen hosts against allow_public_bind policy.
func BindCheck(listenHosts []string, allowPublic bool) error {
	hasWildcard := false
	for _, h := range listenHosts {
		h = strings.TrimSpace(h)
		if h == "0.0.0.0" || h == "::" {
			hasWildcard = true
		}
	}
	if hasWildcard && !allowPublic {
		return fmt.Errorf("listen_hosts contains 0.0.0.0/:: but api.allow_public_bind is false; set allow_public_bind: true if you accept the risk")
	}
	if hasWildcard && allowPublic {
		logger.Warn("PUBLIC BIND ENABLED: management API is exposed on all interfaces. You assume all risks.")
	}
	return nil
}

// ResolveListenAddrs builds net listen addresses from hosts and port.
func ResolveListenAddrs(hosts []string, port int) ([]string, error) {
	if len(hosts) == 0 {
		hosts = []string{"127.0.0.1"}
	}
	var addrs []string
	for _, h := range hosts {
		addrs = append(addrs, net.JoinHostPort(strings.TrimSpace(h), fmt.Sprintf("%d", port)))
	}
	return addrs, nil
}

var (
	rePassword = regexp.MustCompile(`(?i)(password|passwd|secret|token|private_key)\s*[:=]\s*\S+`)
	reKey      = regexp.MustCompile(`(?i)[A-Za-z0-9+/]{40,}={0,2}`)
)

// Redact removes sensitive data from log/API strings.
func Redact(s string) string {
	s = rePassword.ReplaceAllString(s, "$1=[REDACTED]")
	if len(s) > 200 {
		s = reKey.ReplaceAllString(s, "[REDACTED_KEY]")
	}
	return s
}

// SecurityHeaders returns recommended HTTP security headers.
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
