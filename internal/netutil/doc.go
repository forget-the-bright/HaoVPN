// Package netutil 提供与平台无关的网络纯函数工具。
//
// 职责边界：
//   - CIDR / 监听地址 / MTU 默认值 / 远端地址解析 / IPv4 规范化等校验与格式化
//   - 不依赖 config、api、tun、netstack（避免循环引用）
//
// 关键文件：
//   cidr.go — ValidateCIDRList、SplitCIDR、ParseCIDRToV4Mask、ValidateIPInSubnet
//   addr.go — HostFromAddr、ParseHostIP、NormalizeIPv4、DedupTrimNonEmpty
//   gateway.go — InferGatewayFromVPNIP、ResolveGateway、IsLoopbackHost
//   listen.go — 管理口监听地址合并与校验
//   ipmatch.go — IPMatchesRules、ParseCIDROrHost
//   constants.go — MTU/心跳/重连等传输默认值（保留天数在 config.DefaultRetentionDays）
//
// 上游调用方：config（加载校验）、security（TLS ServerName）、tunnel（来源 IP）、
// netstack（路由/WFP 掩码）、vpnaccount（IP 规范化）、serverapp、api。
// 与 winnet 区别：本包无 Windows shell/netsh/PowerShell；winnet 专管 Windows 网卡/LUID。
package netutil
