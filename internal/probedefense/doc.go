// Package probedefense 识别公网对隧道口的扫描探针，记录安全事件并可自动/手动封禁源 IP。
//
// 上下游：
//   - 由 serverapp 注入 transport.Config.Probe（有 Guard 即挂载，不依赖 Enabled）与 tunnel.ServerHandler.Probe；
//   - 持久化 security_events / ip_blocks（persist）；
//   - 管理面经 api handler_security_* 查询与手动封/解封。
//
// 文件：
//   guard.go — Guard、Accept 门禁、RecordReject/ManualBan；
//   classify_tls.go / classify_handshake.go — TLS/帧/握手特征分类；
//   signatures.go — 特征码常量（与 labels.go map key 同源）；
//   auto_ban.go — maybeAutoBan 窗口计数；
//   manual_ban.go — ManualBanStore（豁免检查 + upsert）；
//   errors.go — ErrBanExempt、ErrInvalidBanIP、ErrProbeGuardNotReady；
//   exempt.go — 封禁豁免；ignorable.go — 读超时等不记探针；
//   labels.go — 中文标签；config_from.go — 从 config 映射。
//
// 不变量：
//   - record_events 控制 security_events 写库；enabled 控制 RecordReject 是否参与 auto-ban；
//   - IsBlocked/手动封禁/Accept 封禁检查不依赖 enabled；
//   - 源白名单与 tunnel.CheckTunnelSourceIP 共用 netutil.CheckSourceIPAllowed；
//   - classify_handshake 委托 autherr.Classify 映射 signature。
package probedefense
