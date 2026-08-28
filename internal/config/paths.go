package config

// DefaultServerCertPath 为服务端 TLS 证书默认相对路径（与 serverYAMLTemplate 一致）。
const DefaultServerCertPath = "./certs/server.crt"

// ResolveServerCertPath 返回用于客户端导出的 CA/证书路径。
//
// 参数：cfg — 服务端配置；cfg.Server.TLS.CertFile 非空时优先使用，否则 DefaultServerCertPath。
func ResolveServerCertPath(cfg *ServerConfig) string {
	if cfg != nil && cfg.Server.TLS.CertFile != "" {
		return cfg.Server.TLS.CertFile
	}
	return DefaultServerCertPath
}
