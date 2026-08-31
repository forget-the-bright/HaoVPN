// Package netutil 提供与平台无关的网络纯函数工具。
//
// 职责边界：
//   - CIDR / 监听地址 / MTU 默认值 / 远端地址解析 / IPv4 规范化等校验与格式化
//   - 不依赖 config、api、tun、netstack（避免循环引用）
//
// 关键文件：
//   validate_ip.go — ValidateIPOrCIDR（单 IP 或 CIDR 校验，API/配置/guard 共用）
//   cidr.go — ValidateCIDRList、SplitCIDR、ParseCIDRToV4Mask、ValidateIPInSubnet、ValidateNoFullTunnel
//   addr.go — HostFromAddr、NormalizeIPv4、MergeDedupTrimNonEmpty、DedupTrimNonEmpty…
//   source_ip.go — CheckSourceIPAllowed（wrap dialerr.ErrSourceDenied；tunnel 握手与 probedefense 直接调用，无薄包装）
//   hostport.go — SplitHostPortLoose、SplitRemoteAddr（探针/握手共用远端拆分）
//   gateway.go — InferGatewayFromVPNIP、InferVPNSubnetHint、ResolveGateway、IsLoopbackHost
//   strings.go — TrimLower（单串 Trim+小写；列表见 DedupTrimNonEmpty）
//   listen.go — 管理口监听地址合并与校验
//   ipmatch.go — IPMatchesRules、ParseCIDROrHost、NormalizeCIDROrHost、NormalizeCIDRList、
//                AppendCIDRUnique、ForbidDefaultRoute、CIDRListContainsIP
//   constants.go — MTU/心跳/重连等传输默认值（保留天数在 config.DefaultRetentionDays）
//
// 上游：config、security、tunnel、netstack、vpnaccount、clientapp、serverapp、api、paginate（via api）。
// 下游：标准库 net；dialerr（源拒哨兵）；不依赖 config/tun/netstack/persist。
// 并发：纯函数无状态，可并行调用。
// 不变量：CIDR/地址纯函数仅在本包；广告 LAN 校验不经 persist。
package netutil
