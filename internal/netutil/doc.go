// Package netutil 提供与平台无关的网络纯函数工具。
//
// 职责边界：
//   - CIDR / 监听地址 / MTU 默认值 / 远端地址解析 / IPv4 规范化等校验与格式化
//   - 不依赖 config、api、tun、netstack（避免循环引用）
//
// 关键文件：
//   cidr.go — ValidateCIDRList、SplitCIDR、ParseCIDRToV4Mask、ValidateIPInSubnet
//   addr.go — HostFromAddr、ParseHostIP、NormalizeIPv4、DedupTrimNonEmpty
//   hostport.go — SplitHostPortLoose、SplitRemoteAddr（探针/握手共用远端拆分）
//   gateway.go — InferGatewayFromVPNIP、ResolveGateway、IsLoopbackHost
//   listen.go — 管理口监听地址合并与校验
//   ipmatch.go — IPMatchesRules、ParseCIDROrHost
//   constants.go — MTU/心跳/重连等传输默认值（保留天数在 config.DefaultRetentionDays）
//
// 上游：config、security、tunnel、netstack、vpnaccount、serverapp、api、paginate（via api）。
// 下游：标准库 net；不依赖 config/tun/netstack。
// 并发：纯函数无状态，可并行调用。
// 不变量：CIDR/地址纯函数仅在本包；保留天数在 config.DefaultRetentionDays。
package netutil
