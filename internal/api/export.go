package api

import (
	"haovpn/internal/config"
	"haovpn/internal/persist"
)

// buildClientExportYAML 薄封装：导出逻辑在 config.BuildClientExportYAML，避免与模板漂移。
//
// plainPrivateKey/serverPubKey 保留参数以兼容旧调用签名；策略与密钥均由握手下发，不写入 YAML。
func buildClientExportYAML(cfg *config.ServerConfig, u *persist.User, plainPrivateKey, serverPubKey, caFile string) string {
	_ = plainPrivateKey
	_ = serverPubKey
	if cfg == nil || u == nil {
		return ""
	}
	ca := caFile
	if ca == "" {
		ca = cfg.Server.TLS.CertFile
	}
	return config.BuildClientExportYAML(cfg.Server.Listen, u.Username, ca, cfg.VPN.MTU)
}
