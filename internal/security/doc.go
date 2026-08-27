// Package security TLS、证书、密钥加密与管理口安全策略。
//
// 关键文件：
//   tls_policy.go — TLSConfig、ClientTLSConfigWithRootCAs、BindCheck、SecurityHeaders
//   tls_client.go — BuildClientTLS、BuildClientTLSFromOptions
//   cert.go — 自签证书 EnsureServerCert
//   keyenc.go / datakey.go — 账号私钥与 DB 字段 AES 密封
//
// 上游：clientapp/serverapp、api、vpnaccount、tunnel。
// 下游：netutil（BindCheck）、fileutil（证书/密钥目录）。
// 不变量：默认禁 insecure_skip_verify；Redact 用于日志脱敏。
package security
