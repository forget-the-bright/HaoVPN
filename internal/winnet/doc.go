// Package winnet 封装 Windows 网卡/LUID/netsh/PowerShell 公共能力。
//
// 职责边界：
//   - 解析 Wintun 配置名 → 系统 ifIndex / netsh 别名
//   - 统一 PowerShell：RunPS（Bypass）/ RunPSBestEffort（失败 Warn）；禁止业务包 raw powershell
//   - netsh 薄封装；ParseDNSShowOutput（netsh DNS 输出解析，跨平台纯函数）
//   - ICS：HasICSResidue（便宜探测 137）、CleanupICSResidue（一次 PS 关共享+清地址）、
//     DisableAllICS / PreferVPNSourceWithICS / RemoveICSAddressesKeepVPN
//   - 不依赖 tun/netstack（tun 打开设备后调用 RegisterFromLUID 写入缓存）
//
// 关键文件（Windows 实现见 *_windows.go；非 Windows 见 stub.go）：
//   escape.go — EscapeSingleQuoted
//   dns_parse.go — ParseDNSShowOutput
//   ps_windows.go — RunPS / RunPSBestEffort / RunNetsh
//   address_windows.go — SetInterfaceIPv4、AssignIPv4PowerShell、PreferVPNSource、RemoveICSAddresses
//   dns_netsh_windows.go — Set/Add/Restore/Show DNS（netsh）
//   ics_windows.go — HasICSResidue / CleanupICSResidue / DisableAllICS
//   resolver_windows.go — ifIndex / LUID / 别名
//
// 上游：internal/tun（Wintun 生命周期）、internal/netstack（路由/DNS/NAT）、
// clientapp/via_exit（空 local_lans 智能清理）。
// 下游：internal/platform（无窗口子进程）、internal/logger。
package winnet
