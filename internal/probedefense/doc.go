// Package probedefense 识别公网对隧道口的扫描探针，记录安全事件并可自动/手动封禁源 IP。
//
// 上下游：
//   - 由 serverapp 注入 transport.Config.Probe（有 Guard 即挂载，不依赖 Enabled）与 tunnel.ServerHandler.Probe；
//   - 持久化 security_events / ip_blocks（persist）；
//   - 管理面经 api handler_security 查询与手动封/解封。
//
// 文件：
//   guard.go — Guard、特征分类、Accept 门禁；远端地址用 netutil.SplitRemoteAddr；
//   ignorable.go — 读超时等不记探针；
//   labels.go — 特征/阶段/动作中文（与 docs/security-hardening.md 对照表同源）；
//   config_from.go — 从 config.SecuritySection 映射。
//
// 不变量：
//   - Enabled 只管自动记录/自动封；IsBlocked/手动封禁/Accept 封禁检查不依赖 Enabled；
//   - 源白名单匹配与 tunnel.CheckTunnelSourceIP 共用 netutil.IPMatchesRules。
package probedefense
