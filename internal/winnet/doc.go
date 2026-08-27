// Package winnet 封装 Windows 网卡/LUID/netsh/PowerShell 公共能力。
//
// 职责边界：
//   - 解析 Wintun 配置名 → 系统 ifIndex / netsh 别名
//   - 统一 Get-NetAdapter 回退脚本、netsh 薄封装
//   - 不依赖 tun/netstack（tun 打开设备后调用 RegisterFromLUID 写入缓存）
//
// 上游：internal/tun（Wintun 生命周期）、internal/netstack（路由/DNS/NAT）。
// 下游：internal/platform（无窗口子进程）。
package winnet
